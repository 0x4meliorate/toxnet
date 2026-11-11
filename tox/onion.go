package tox

import (
	"encoding/binary"
	"math/rand"
	"sort"
	"time"
)

const (
	pktAnnounceRequest  = 0x83 // NET_PACKET_ANNOUNCE_REQUEST_OLD
	pktAnnounceResponse = 0x84 // NET_PACKET_ANNOUNCE_RESPONSE_OLD
	pktOnionDataRequest = 0x85
	pktOnionDataResp    = 0x86

	onionDataFriendReq = 32  // CRYPTO_PACKET_FRIEND_REQ
	onionDataDHTPK     = 156 // CRYPTO_PACKET_DHTPK

	maxOnionNodes = 8
	maxPaths      = 4
)

// onionPath: TCP relay (hop 1) -> n2 -> n3 -> destination.
type onionPath struct {
	relay    *relay
	n2, n3   Node
	pk2, pk3 Key // our per-hop temporary public keys
	k2, k3   Key // shared keys with n2 and n3
	created  time.Time
	fails    int
}

type onionNode struct {
	n          Node
	pingID     [32]byte
	stored     byte // last is_stored value: 0 none, 1 target found (friend), 2 we are announced
	dataPK     Key
	path       *onionPath
	lastSent   time.Time
	lastRecv   time.Time
	unanswered int
	fast       int
}

// onionTarget is either ourselves (announcing) or a friend (searching).
type onionTarget struct {
	pk             Key
	sendPK, sendSK Key
	isSelf         bool
	friend         *friend
	nodes          []*onionNode
	bad            map[Key]time.Time
	announced      bool
}

type pendingAnnounce struct {
	target *onionTarget
	node   *onionNode
	path   *onionPath
	sent   time.Time
}

func (ot *onionTarget) storedCount() int {
	c := 0
	for _, n := range ot.nodes {
		if (ot.isSelf && n.stored == 2) || (!ot.isSelf && n.stored == 1) {
			c++
		}
	}
	return c
}

func (ot *onionTarget) add(n Node) {
	if !n.usable() || n.IP.To4() == nil || n.TCP || n.IP.IsPrivate() || n.IP.IsLinkLocalUnicast() {
		return
	}
	for _, e := range ot.nodes {
		if e.n.PK == n.PK {
			return
		}
	}
	if t, ok := ot.bad[n.PK]; ok && time.Since(t) < 2*time.Minute {
		return
	}
	if len(ot.nodes) >= maxOnionNodes {
		far := ot.nodes[len(ot.nodes)-1]
		if !closer(&ot.pk, &n.PK, &far.n.PK) {
			return
		}
		ot.nodes = ot.nodes[:len(ot.nodes)-1]
	}
	ot.nodes = append(ot.nodes, &onionNode{n: n})
	sort.Slice(ot.nodes, func(i, j int) bool { return closer(&ot.pk, &ot.nodes[i].n.PK, &ot.nodes[j].n.PK) })
}

func (ot *onionTarget) remove(on *onionNode) {
	for i, e := range ot.nodes {
		if e == on {
			ot.nodes = append(ot.nodes[:i], ot.nodes[i+1:]...)
			break
		}
	}
	if ot.bad == nil {
		ot.bad = map[Key]time.Time{}
	}
	ot.bad[on.n.PK] = time.Now()
}

func (t *Tox) learnNode(n Node) {
	if !n.usable() || n.IP.To4() == nil || n.IP.IsPrivate() {
		return
	}
	for _, e := range t.dhtNodes {
		if e.PK == n.PK {
			return
		}
	}
	if len(t.dhtNodes) >= 512 {
		t.dhtNodes = t.dhtNodes[1:]
	}
	t.dhtNodes = append(t.dhtNodes, n)
}

func (t *Tox) pathValid(p *onionPath) bool {
	if p == nil || !p.relay.alive || p.fails >= 3 || time.Since(p.created) > 15*time.Minute {
		return false
	}
	for _, e := range t.paths {
		if e == p {
			return true
		}
	}
	return false
}

// pickPath returns a working path whose hops differ from the destination.
func (t *Tox) pickPath(dest Key) *onionPath {
	var ok []*onionPath
	kept := t.paths[:0]
	for _, p := range t.paths {
		if t.pathValid(p) {
			kept = append(kept, p)
			if p.n2.PK != dest && p.n3.PK != dest && p.relay.node.PK != dest {
				ok = append(ok, p)
			}
		}
	}
	t.paths = kept
	if len(ok) > 0 && (len(t.paths) >= maxPaths || rand.Intn(4) != 0) {
		return ok[rand.Intn(len(ok))]
	}
	if p := t.newPath(dest); p != nil {
		if len(t.paths) >= maxPaths {
			t.paths = t.paths[1:]
		}
		t.paths = append(t.paths, p)
		return p
	}
	if len(ok) > 0 {
		return ok[rand.Intn(len(ok))]
	}
	return nil
}

func (t *Tox) newPath(dest Key) *onionPath {
	relays := t.aliveRelays()
	if len(relays) == 0 {
		return nil
	}
	r := relays[rand.Intn(len(relays))]
	pick := func(pool []Node, exclude ...Key) []Node {
		var out []Node
	outer:
		for _, n := range pool {
			for _, x := range exclude {
				if n.PK == x {
					continue outer
				}
			}
			if n.usable() && n.IP.To4() != nil {
				out = append(out, n)
			}
		}
		return out
	}
	cands := pick(t.bootstraps, r.node.PK, dest)
	if len(cands) < 2 {
		cands = append(cands, pick(t.dhtNodes, r.node.PK, dest)...)
	}
	if len(cands) < 2 {
		return nil
	}
	i := rand.Intn(len(cands))
	j := rand.Intn(len(cands) - 1)
	if j >= i {
		j++
	}
	p := &onionPath{relay: r, n2: cands[i], n3: cands[j], created: time.Now()}
	var sk2, sk3 Key
	p.pk2, sk2 = newKeyPair()
	p.pk3, sk3 = newKeyPair()
	p.k2 = precompute(&p.n2.PK, &sk2)
	p.k3 = precompute(&p.n3.PK, &sk3)
	return p
}

// sendOnion wraps data for dest and hands it to the path's TCP relay (create_onion_packet_tcp).
func (t *Tox) sendOnion(p *onionPath, dest Node, data []byte) {
	nonce := randomNonce()
	step1 := append(packIPPort(dest), data...)
	step2 := append(packIPPort(p.n3), p.pk3[:]...)
	step2 = append(step2, seal(&p.k3, &nonce, step1)...)
	pkt := []byte{tcpOnionRequest}
	pkt = append(pkt, nonce[:]...)
	pkt = append(pkt, packIPPort(p.n2)...)
	pkt = append(pkt, p.pk2[:]...)
	pkt = append(pkt, seal(&p.k2, &nonce, step2)...)
	p.relay.queue(pkt)
}

func (t *Tox) sendAnnounce(ot *onionTarget, on *onionNode) {
	if !t.pathValid(on.path) {
		on.path = t.pickPath(on.n.PK)
	}
	if on.path == nil {
		return
	}
	sendback := randomU64()
	t.pending[sendback] = &pendingAnnounce{target: ot, node: on, path: on.path, sent: time.Now()}
	on.lastSent = time.Now()

	plain := make([]byte, 0, 104)
	var dataPK Key
	if ot.isSelf {
		plain = append(plain, on.pingID[:]...)
		dataPK = t.onionPK
	} else {
		plain = append(plain, make([]byte, 32)...)
	}
	plain = append(plain, ot.pk[:]...)
	plain = append(plain, dataPK[:]...)
	plain = binary.LittleEndian.AppendUint64(plain, sendback)

	nonce := randomNonce()
	shared := precompute(&on.n.PK, &ot.sendSK)
	pkt := []byte{pktAnnounceRequest}
	pkt = append(pkt, nonce[:]...)
	pkt = append(pkt, ot.sendPK[:]...)
	pkt = append(pkt, seal(&shared, &nonce, plain)...)
	t.sendOnion(on.path, on.n, pkt)
}

func (t *Tox) onionTick() {
	now := time.Now()
	for id, pa := range t.pending {
		if now.Sub(pa.sent) > 6*time.Second {
			delete(t.pending, id)
			pa.path.fails++
			pa.node.unanswered++
			if pa.node.unanswered >= 3 {
				pa.target.remove(pa.node)
			}
		}
	}
	targets := []*onionTarget{t.self}
	for _, f := range t.friends {
		if !f.removed {
			targets = append(targets, f.search) // keep tracking friends so DHTPK updates reach them
		}
	}
	for _, ot := range targets {
		if len(ot.nodes) < maxOnionNodes {
			for _, n := range t.bootstraps {
				ot.add(n)
			}
			for _, n := range t.dhtNodes {
				ot.add(n)
			}
		}
		for _, on := range ot.nodes {
			interval := 15 * time.Second
			if on.lastRecv.IsZero() || (ot.isSelf && on.stored != 2) {
				interval = 3 * time.Second
			}
			if now.Sub(on.lastSent) >= interval {
				t.sendAnnounce(ot, on)
			}
		}
		if ot.isSelf {
			c := ot.storedCount()
			if c > 0 && !ot.announced {
				t.logf("announced on the onion (%d nodes)", c)
			}
			ot.announced = c > 0
		}
	}
}

func (t *Tox) handleOnionResponse(p []byte) {
	switch p[0] {
	case pktAnnounceResponse:
		t.handleAnnounceResponse(p)
	case pktOnionDataResp:
		t.handleOnionData(p)
	}
}

func (t *Tox) handleAnnounceResponse(p []byte) {
	if len(p) < 1+8+24+1+32+macSize {
		return
	}
	id := binary.LittleEndian.Uint64(p[1:9])
	pa := t.pending[id]
	if pa == nil {
		return
	}
	var nonce Nonce
	copy(nonce[:], p[9:33])
	shared := precompute(&pa.node.n.PK, &pa.target.sendSK)
	plain, ok := open(&shared, &nonce, p[33:])
	if !ok || len(plain) < 33 {
		return
	}
	delete(t.pending, id)
	ot, on := pa.target, pa.node
	pa.path.fails = 0
	on.unanswered = 0
	on.lastRecv = time.Now()
	isStored := plain[0]

	if ot.isSelf {
		copy(on.pingID[:], plain[1:33])
		on.stored = isStored
		if isStored == 0 && on.fast < 3 {
			on.fast++
			t.sendAnnounce(ot, on) // re-send right away with the ping id we just got
		} else if isStored == 2 {
			on.fast = 0
		}
	} else {
		if isStored == 1 {
			if on.stored != 1 && ot.storedCount() == 0 {
				t.logf("found friend %s on the onion", keyHex(ot.pk)[:8])
			}
			copy(on.dataPK[:], plain[1:33])
			on.stored = 1
		} else {
			on.stored = 0
		}
	}
	for _, n := range unpackNodes(plain[33:]) {
		t.learnNode(n)
		ot.add(n)
	}
}

// sendOnionData sends an onion data packet to a friend through every node that holds their announce.
func (t *Tox) sendOnionData(f *friend, data []byte) int {
	nonce := randomNonce()
	shared := precompute(&f.realPK, &t.realSK)
	inner := append(append([]byte{}, t.realPK[:]...), seal(&shared, &nonce, data)...)
	sent := 0
	for _, on := range f.search.nodes {
		if on.stored != 1 {
			continue
		}
		p := t.pickPath(on.n.PK)
		if p == nil {
			continue
		}
		tpk, tsk := newKeyPair()
		k := precompute(&on.dataPK, &tsk)
		req := []byte{pktOnionDataRequest}
		req = append(req, f.realPK[:]...)
		req = append(req, nonce[:]...)
		req = append(req, tpk[:]...)
		req = append(req, seal(&k, &nonce, inner)...)
		t.sendOnion(p, on.n, req)
		sent++
	}
	return sent
}

// sendDHTPK tells a friend our DHT key and the TCP relays we are connected to.
func (t *Tox) sendDHTPK(f *friend) int {
	now := uint64(time.Now().Unix())
	if now <= t.noReplay {
		now = t.noReplay + 1
	}
	t.noReplay = now
	d := []byte{onionDataDHTPK}
	d = binary.BigEndian.AppendUint64(d, now)
	d = append(d, t.dhtPK[:]...)
	var nodes []Node
	for _, r := range t.aliveRelays() {
		if len(nodes) == 3 {
			break
		}
		n := r.node
		n.TCP = true
		nodes = append(nodes, n)
	}
	d = append(d, packNodes(nodes)...)
	return t.sendOnionData(f, d)
}

func (t *Tox) handleOnionData(p []byte) {
	if len(p) <= 1+24+32+32+2*macSize {
		return
	}
	var nonce Nonce
	var tmpPK Key
	copy(nonce[:], p[1:25])
	copy(tmpPK[:], p[25:57])
	k1 := precompute(&tmpPK, &t.onionSK)
	l1, ok := open(&k1, &nonce, p[57:])
	if !ok || len(l1) <= 32+macSize {
		return
	}
	var sender Key
	copy(sender[:], l1[:32])
	k2 := precompute(&sender, &t.realSK)
	d, ok := open(&k2, &nonce, l1[32:])
	if !ok || len(d) == 0 {
		return
	}
	switch d[0] {
	case onionDataDHTPK:
		if f := t.friendByPK(sender); f != nil {
			t.handleDHTPK(f, d)
		}
	case onionDataFriendReq:
		if len(d) > 5 && [4]byte(d[1:5]) == t.nospam && t.friendByPK(sender) == nil {
			pk, msg := keyHex(sender), string(d[5:])
			if t.autoAccept {
				if _, err := t.addFriendLocked(sender); err == nil {
					t.logf("auto-accepted friend request from %s", pk[:8])
					t.save()
				}
			}
			if cb := t.cbRequest; cb != nil {
				t.emit(func() { cb(pk, msg) })
			}
		}
	}
}

func (t *Tox) handleDHTPK(f *friend, d []byte) {
	if len(d) < 41 {
		return
	}
	nr := binary.BigEndian.Uint64(d[1:9])
	if nr <= f.lastNoReplay {
		return
	}
	f.lastNoReplay = nr
	var dht Key
	copy(dht[:], d[9:41])
	for _, n := range unpackNodes(d[41:]) {
		if n.TCP {
			t.addRelay(n, maxFriendRelays)
		}
	}
	if !f.hasDHT || f.dhtPK != dht {
		t.logf("got DHT key of friend %s", keyHex(f.realPK)[:8])
		t.setFriendDHT(f, dht)
	}
}
