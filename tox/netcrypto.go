package tox

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"time"
)

const (
	pktCookieRequest  = 0x18
	pktCookieResponse = 0x19
	pktCryptoHS       = 0x1a
	pktCryptoData     = 0x1b

	cookieLen         = 24 + 8 + 64 + macSize            // 112
	cookieRequestLen  = 1 + 32 + 24 + 72 + macSize       // 145
	cookieResponseLen = 1 + 24 + cookieLen + 8 + macSize // 161
	handshakeLen      = 1 + cookieLen + 24 + 24 + 32 + 64 + cookieLen + macSize

	pidPadding = 0
	pidRequest = 1
	pidKill    = 2

	pktAlive       = 16 // friend_connection
	pktShareRelays = 17
	pktOnline      = 24 // Messenger
	pktOffline     = 25
	pktUserStatus  = 50 // 0 = online, 1 = away, 2 = busy
	pktNickname    = 48
	pktMessage     = 64
	pktAction      = 65

	pktFileSendReq = 80 // PACKET_ID_FILE_SENDREQUEST
	pktFileControl = 81 // PACKET_ID_FILE_CONTROL
	pktFileData    = 82 // PACKET_ID_FILE_DATA

	maxCryptoData = 1400 - (1 + 2 + 4 + 4 + macSize) // 1373
	maxMessageLen = maxCryptoData - 1
)

const (
	csCookieRequesting = iota
	csHandshakeSent
	csNotConfirmed
	csEstablished
)

type sentPacket struct {
	data []byte
	sent time.Time
	retx bool // retransmitted at least once (its RTT sample is unreliable)
}

type cryptoConn struct {
	f         *friend
	peerDHT   Key
	status    int
	dhtShared Key
	cookieNum [8]byte

	sessPK, sessSK Key
	sentNonce      Nonce
	recvNonce      Nonce
	shared         Key

	temp      []byte
	tempSent  time.Time
	tempTries int

	sendStart, sendEnd uint32
	sendBuf            map[uint32]*sentPacket
	recvStart, recvEnd uint32
	recvBuf            map[uint32][]byte
	ackedUpto          uint32 // recvStart at the last ack we sent the peer

	lastAlive, lastAliveSent, lastReqSent time.Time
	lastShare, lastRouted                 time.Time
	lastDebug                             time.Time

	// Congestion control for file transfers (see congestion.go).
	cwnd       float64   // congestion window, in packets (paces the file pump)
	ssthresh   float64   // slow-start threshold
	srtt       time.Duration
	rttvar     time.Duration
	lastShrink time.Time // last cwnd reduction, to back off once per round trip
}

var connStatusNames = [...]string{"cookie-requesting", "handshake-sent", "not-confirmed", "established"}

// debugStatus logs a one-line summary of a connection every 10 seconds.
func (t *Tox) debugStatus(f *friend, c *cryptoConn, now time.Time) {
	if t.opts.Logf == nil || now.Sub(c.lastDebug) < 10*time.Second {
		return
	}
	c.lastDebug = now
	routes := 0
	for _, r := range t.aliveRelays() {
		if r.isOnline(c.peerDHT) {
			routes++
		}
	}
	alive := "never"
	if !c.lastAlive.IsZero() {
		alive = now.Sub(c.lastAlive).Round(time.Second).String() + " ago"
	}
	t.logf("friend %s: conn=%s online=%v routed-relays=%d sent=%d acked=%d received=%d last-alive=%s",
		keyHex(f.realPK)[:8], connStatusNames[c.status], f.online, routes, c.sendEnd, c.sendStart, c.recvStart, alive)
}

func (t *Tox) newCryptoConn(f *friend, peerDHT Key, sendCookie bool) *cryptoConn {
	c := &cryptoConn{f: f, peerDHT: peerDHT, sendBuf: map[uint32]*sentPacket{}, recvBuf: map[uint32][]byte{}}
	c.sessPK, c.sessSK = newKeyPair()
	c.sentNonce = randomNonce()
	c.dhtShared = precompute(&peerDHT, &t.dhtSK)
	copy(c.cookieNum[:], randomBytes(8))
	c.status = csCookieRequesting
	c.cwnd = cwndInit
	c.ssthresh = maxCwnd()

	plain := append(append([]byte{}, t.realPK[:]...), make([]byte, 32)...)
	plain = append(plain, c.cookieNum[:]...)
	nonce := randomNonce()
	pkt := []byte{pktCookieRequest}
	pkt = append(pkt, t.dhtPK[:]...)
	pkt = append(pkt, nonce[:]...)
	c.temp = append(pkt, seal(&c.dhtShared, &nonce, plain)...)
	if sendCookie {
		t.sendTemp(c)
	}
	return c
}

func (t *Tox) sendTemp(c *cryptoConn) {
	if c.temp == nil {
		return
	}
	c.tempSent = time.Now()
	c.tempTries++
	t.sendToPeer(c.peerDHT, c.temp)
}

// sendToPeer delivers a raw net_crypto packet over a routed relay connection if
// one is online. Otherwise cookie and handshake packets go out-of-band through
// every relay we're connected to; data packets are dropped, because toxcore
// ignores out-of-band data (it only accepts cookie requests and handshakes).
func (t *Tox) sendToPeer(dht Key, pkt []byte) bool {
	relays := t.aliveRelays()
	for _, r := range relays {
		if r.isOnline(dht) {
			r.sendTo(dht, pkt)
			return true
		}
	}
	if pkt[0] == pktCryptoData {
		return false
	}
	for _, r := range relays {
		r.sendTo(dht, pkt)
	}
	return len(relays) > 0
}

func (t *Tox) hasRoute(dht Key) bool {
	for _, r := range t.aliveRelays() {
		if r.isOnline(dht) {
			return true
		}
	}
	return false
}

// shareRelays sends PACKET_ID_SHARE_RELAYS with up to 3 relays we're connected
// to (preferring ones already routed to the friend), like toxcore does every
// 2 minutes. The friend connects to them, which keeps a routed path alive.
func (t *Tox) shareRelays(f *friend, c *cryptoConn) {
	var routed, other []Node
	for _, r := range t.aliveRelays() {
		n := r.node
		n.TCP = true
		if r.isOnline(c.peerDHT) {
			routed = append(routed, n)
		} else {
			other = append(other, n)
		}
	}
	nodes := append(routed, other...)
	if len(nodes) > 3 {
		nodes = nodes[:3]
	}
	c.lastShare = time.Now()
	if len(nodes) > 0 {
		t.sendLossless(c, append([]byte{pktShareRelays}, packNodes(nodes)...))
	}
}

func (t *Tox) setFriendDHT(f *friend, dht Key) {
	if f.hasDHT && f.dhtPK == dht && f.conn != nil {
		return
	}
	t.killConn(f)
	f.dhtPK, f.hasDHT = dht, true
	for _, r := range t.aliveRelays() {
		r.requestRoute(dht)
	}
	f.conn = t.newCryptoConn(f, dht, true)
}

func (t *Tox) peerRouteOnline(dht Key) {
	if f := t.friendByDHT(dht); f != nil && f.conn != nil {
		if f.conn.status != csEstablished {
			t.sendTemp(f.conn)
		} else {
			t.resendUnacked(f.conn)
		}
	}
}

func (t *Tox) killConn(f *friend) {
	c := f.conn
	if c == nil {
		return
	}
	t.logf("friend %s: closing %s connection", keyHex(f.realPK)[:8], connStatusNames[c.status])
	if c.status >= csNotConfirmed {
		t.sendDataPacket(c, c.recvStart, c.sendEnd, []byte{pidKill})
	}
	f.conn = nil
	if f.online {
		f.online = false
		f.lastSeen = time.Now()
		t.save()
		if cb := t.cbConn; cb != nil {
			n := f.num
			t.emit(func() { cb(n, false) })
		}
	}
}

// ---- cookies ----

func (t *Tox) makeCookie(realPK, dhtPK []byte) []byte {
	plain := binary.LittleEndian.AppendUint64(nil, uint64(time.Now().Unix()))
	plain = append(plain, realPK...)
	plain = append(plain, dhtPK...)
	nonce := randomNonce()
	return append(nonce[:], seal(&t.cookieKey, &nonce, plain)...)
}

func (t *Tox) openCookie(c []byte) (realPK, dhtPK Key, ok bool) {
	var nonce Nonce
	copy(nonce[:], c[:24])
	plain, ok := open(&t.cookieKey, &nonce, c[24:])
	if !ok || len(plain) != 72 {
		return realPK, dhtPK, false
	}
	ts := int64(binary.LittleEndian.Uint64(plain[:8]))
	now := time.Now().Unix()
	if ts > now || ts+15 < now {
		return realPK, dhtPK, false
	}
	copy(realPK[:], plain[8:40])
	copy(dhtPK[:], plain[40:72])
	return realPK, dhtPK, true
}

func (t *Tox) makeHandshake(c *cryptoConn, theirCookie []byte) []byte {
	h := sha512.Sum512(theirCookie)
	plain := append([]byte{}, c.sentNonce[:]...)
	plain = append(plain, c.sessPK[:]...)
	plain = append(plain, h[:]...)
	plain = append(plain, t.makeCookie(c.f.realPK[:], c.peerDHT[:])...)
	nonce := randomNonce()
	k := precompute(&c.f.realPK, &t.realSK)
	pkt := append([]byte{pktCryptoHS}, theirCookie...)
	pkt = append(pkt, nonce[:]...)
	return append(pkt, seal(&k, &nonce, plain)...)
}

// handleCryptoPacket processes a net_crypto packet that arrived from DHT key `from`
// through relay r (routed or out-of-band). Called with t.mu held.
func (t *Tox) handleCryptoPacket(r *relay, from Key, p []byte) {
	switch p[0] {
	case pktCookieRequest:
		if len(p) != cookieRequestLen || !bytes.Equal(p[1:33], from[:]) {
			return
		}
		var nonce Nonce
		copy(nonce[:], p[33:57])
		shared := precompute(&from, &t.dhtSK)
		plain, ok := open(&shared, &nonce, p[57:])
		if !ok || len(plain) != 72 {
			return
		}
		resp := append(t.makeCookie(plain[:32], from[:]), plain[64:72]...)
		n2 := randomNonce()
		out := append([]byte{pktCookieResponse}, n2[:]...)
		r.sendTo(from, append(out, seal(&shared, &n2, resp)...))
		t.logf("cookie request from %s answered via relay %s", keyHex(from)[:8], r.node)

	case pktCookieResponse:
		f := t.friendByDHT(from)
		if f == nil || f.conn == nil || f.conn.status != csCookieRequesting || len(p) != cookieResponseLen {
			return
		}
		c := f.conn
		var nonce Nonce
		copy(nonce[:], p[1:25])
		plain, ok := open(&c.dhtShared, &nonce, p[25:])
		if !ok || len(plain) != cookieLen+8 || !bytes.Equal(plain[cookieLen:], c.cookieNum[:]) {
			return
		}
		c.temp = t.makeHandshake(c, plain[:cookieLen])
		c.status = csHandshakeSent
		t.logf("friend %s: cookie received, handshake sent", keyHex(f.realPK)[:8])
		c.tempTries = 0
		t.sendTemp(c)

	case pktCryptoHS:
		if len(p) != handshakeLen {
			return
		}
		realPK, dht, ok := t.openCookie(p[1 : 1+cookieLen])
		if !ok {
			return
		}
		f := t.friendByPK(realPK)
		if f == nil {
			return
		}
		var nonce Nonce
		copy(nonce[:], p[1+cookieLen:1+cookieLen+24])
		k := precompute(&realPK, &t.realSK)
		plain, ok := open(&k, &nonce, p[1+cookieLen+24:])
		if !ok || len(plain) != 24+32+64+cookieLen {
			return
		}
		h := sha512.Sum512(p[1 : 1+cookieLen])
		if !bytes.Equal(h[:], plain[56:120]) {
			return
		}
		c := f.conn
		if c == nil || c.peerDHT != dht {
			if c != nil {
				t.logf("friend %s changed DHT key", keyHex(f.realPK)[:8])
			}
			t.killConn(f)
			f.dhtPK, f.hasDHT = dht, true
			for _, rr := range t.aliveRelays() {
				rr.requestRoute(dht)
			}
			c = t.newCryptoConn(f, dht, false)
			f.conn = c
		}
		if c.status == csEstablished {
			return
		}
		copy(c.recvNonce[:], plain[:24])
		var peerSess Key
		copy(peerSess[:], plain[24:56])
		c.shared = precompute(&peerSess, &c.sessSK)
		theirCookie := plain[120:]
		t.logf("friend %s: handshake received (our state %s)", keyHex(f.realPK)[:8], connStatusNames[c.status])
		wasRequesting := c.status == csCookieRequesting
		c.temp = t.makeHandshake(c, theirCookie)
		c.status = csNotConfirmed
		c.tempTries = 0
		if wasRequesting {
			t.sendTemp(c)
		}

	case pktCryptoData:
		f := t.friendByDHT(from)
		if f == nil || f.conn == nil || f.conn.status < csNotConfirmed {
			return
		}
		t.handleData(f, f.conn, p)
	}
}

// ---- data packets ----

// sendDataPacket encrypts and sends one data packet. It returns false (and
// sends nothing) when there's no routed relay connection to the peer, since
// toxcore drops data that arrives out-of-band.
func (t *Tox) sendDataPacket(c *cryptoConn, ackStart, num uint32, payload []byte) bool {
	if !t.hasRoute(c.peerDHT) {
		return false
	}
	pad := (maxCryptoData - len(payload)) % 8
	plain := binary.BigEndian.AppendUint32(nil, ackStart)
	plain = binary.BigEndian.AppendUint32(plain, num)
	plain = append(plain, make([]byte, pad)...)
	plain = append(plain, payload...)
	pkt := []byte{pktCryptoData, c.sentNonce[22], c.sentNonce[23]}
	pkt = append(pkt, seal(&c.shared, &c.sentNonce, plain)...)
	incrementNonce(&c.sentNonce, 1)
	return t.sendToPeer(c.peerDHT, pkt)
}

// sendLossless queues a reliable packet and sends it now if a route exists.
// Unsent or unacknowledged packets are retransmitted by resendUnacked.
func (t *Tox) sendLossless(c *cryptoConn, payload []byte) {
	num := c.sendEnd
	c.sendEnd++
	sp := &sentPacket{data: payload}
	c.sendBuf[num] = sp
	if t.sendDataPacket(c, c.recvStart, num, payload) {
		sp.sent = time.Now()
	}
}

// resendUnacked retransmits reliable packets the peer hasn't acknowledged
// within 2 seconds (or that were never sent because no route existed).
// toxcore delivers them strictly in order, so one lost packet would otherwise
// block everything after it, including our ONLINE packet and messages.
func (t *Tox) resendUnacked(c *cryptoConn) {
	now := time.Now()
	rto := c.rto()
	count := 0
	lost := false
	for i := c.sendStart; i != c.sendEnd && count < 32; i++ {
		sp := c.sendBuf[i]
		if sp == nil || now.Sub(sp.sent) < rto {
			continue
		}
		if !t.sendDataPacket(c, c.recvStart, i, sp.data) {
			break
		}
		if !sp.sent.IsZero() {
			lost = true
			t.logf("friend %s: resending packet #%d", keyHex(c.f.realPK)[:8], i)
		}
		sp.sent = now
		sp.retx = true
		count++
	}
	// A retransmission is our loss signal: back the congestion window off so a
	// bulk transfer stops overrunning the link instead of collapsing.
	if lost {
		c.onLoss(now)
	}
}

// sendRequest sends a PACKET_ID_REQUEST listing the lossless packets we are missing.
func (t *Tox) sendRequest(c *cryptoConn) {
	req := []byte{pidRequest}
	n := byte(1)
	for i := c.recvStart; i != c.recvEnd && len(req) < 512; i++ {
		if _, ok := c.recvBuf[i]; !ok {
			req = append(req, n)
			n = 0
		} else if n == 255 {
			req = append(req, 0)
			n = 0
		}
		n++
	}
	c.lastReqSent = time.Now()
	t.sendDataPacket(c, c.recvStart, c.sendEnd, req)
}

func (t *Tox) handleRequest(c *cryptoConn, d []byte) {
	d = d[1:]
	n := byte(1)
	for i := c.sendStart; i != c.sendEnd && len(d) > 0; i++ {
		if n == d[0] {
			if sp := c.sendBuf[i]; sp != nil && time.Since(sp.sent) > time.Second {
				sp.sent = time.Now()
				t.sendDataPacket(c, c.recvStart, i, sp.data)
			}
			d = d[1:]
			n = 0
		} else {
			delete(c.sendBuf, i)
		}
		if n == 255 {
			n = 1
			if len(d) == 0 || d[0] != 0 {
				return
			}
			d = d[1:]
		} else {
			n++
		}
	}
}

func (t *Tox) handleData(f *friend, c *cryptoConn, p []byte) {
	if len(p) <= 1+2+macSize {
		return
	}
	num := binary.BigEndian.Uint16(p[1:3])
	cur := binary.BigEndian.Uint16(c.recvNonce[22:24])
	diff := num - cur
	nonce := c.recvNonce
	incrementNonce(&nonce, uint32(diff))
	plain, ok := open(&c.shared, &nonce, p[3:])
	if !ok || len(plain) <= 8 {
		return
	}
	if diff > 2*21845 {
		incrementNonce(&c.recvNonce, 21845)
	}
	ack := binary.BigEndian.Uint32(plain[0:4])
	pnum := binary.BigEndian.Uint32(plain[4:8])
	if ack-c.sendStart <= c.sendEnd-c.sendStart {
		if ack != c.sendStart {
			// RTT sample from the oldest newly-acked packet, skipping any that
			// were retransmitted (Karn's algorithm), then grow the window.
			if sp := c.sendBuf[c.sendStart]; sp != nil && !sp.retx && !sp.sent.IsZero() {
				c.onRTT(time.Since(sp.sent))
			}
			old := c.sendStart
			for c.sendStart != ack {
				delete(c.sendBuf, c.sendStart)
				c.sendStart++
			}
			c.onAck(int(ack - old))
		}
		// Ack-clock file transfers: as this acknowledgement frees window space,
		// send the next chunks now instead of waiting for the 250ms pump. This
		// paces a bulk transfer to the link so it can't flood the relay queue.
		t.fileTick(f, c)
	}
	d := plain[8:]
	for len(d) > 0 && d[0] == pidPadding {
		d = d[1:]
	}
	if len(d) == 0 {
		return
	}
	if d[0] == pidKill {
		t.logf("friend %s closed the connection", keyHex(f.realPK)[:8])
		t.killConn(f)
		return
	}
	if c.status == csNotConfirmed {
		c.status = csEstablished
		c.temp = nil
		c.lastAlive = time.Now()
		t.logf("secure connection with friend %s established", keyHex(f.realPK)[:8])
		t.sendLossless(c, []byte{pktOnline})
		// qTox only shows a friend as online after receiving their user status.
		t.sendLossless(c, []byte{pktUserStatus, 0})
		if t.opts.Name != "" {
			t.sendLossless(c, append([]byte{pktNickname}, t.opts.Name...))
		}
		t.sendLossless(c, []byte{pktAlive})
		c.lastAliveSent = time.Now()
		c.lastRouted = time.Now()
		t.shareRelays(f, c)
	}
	switch {
	case d[0] == pidRequest:
		t.handleRequest(c, d)
	case d[0] >= 16 && d[0] <= 191:
		if pnum-c.recvStart >= 32768 {
			return // old duplicate or out of window
		}
		c.recvBuf[pnum] = append([]byte(nil), d...)
		if pnum+1-c.recvStart > c.recvEnd-c.recvStart {
			c.recvEnd = pnum + 1
		}
		if pnum != c.recvStart {
			t.logf("friend %s: packet #%d arrived early, waiting for #%d", keyHex(f.realPK)[:8], pnum, c.recvStart)
		}
		for {
			pkt, ok := c.recvBuf[c.recvStart]
			if !ok {
				break
			}
			delete(c.recvBuf, c.recvStart)
			c.recvStart++
			t.handleFriendPacket(f, c, pkt)
			if f.conn != c {
				return
			}
		}
		// Acknowledge promptly during a bulk transfer so the sender's window
		// keeps draining, instead of waiting for the ~1s request timer. The ack
		// is carried in the request packet's ackStart field.
		if c.recvStart-c.ackedUpto >= 16 {
			c.ackedUpto = c.recvStart
			t.sendRequest(c)
		}
	}
}

func (t *Tox) handleFriendPacket(f *friend, c *cryptoConn, d []byte) {
	switch d[0] {
	case pktAlive:
		c.lastAlive = time.Now()
	case pktShareRelays:
		for _, n := range unpackNodes(d[1:]) {
			if n.TCP {
				t.addRelay(n, maxFriendRelays)
				if ri := t.relays[n.PK]; ri != nil && ri.r != nil && ri.r.alive {
					ri.r.requestRoute(c.peerDHT)
				}
			}
		}
	case pktOnline:
		if !f.online {
			f.online = true
			t.logf("friend %s is online", keyHex(f.realPK)[:8])
			n := f.num
			if cb := t.cbConn; cb != nil {
				t.emit(func() { cb(n, true) })
			}
			if !f.everOnline && f.sendRequest {
				if cb := t.cbAccepted; cb != nil {
					t.emit(func() { cb(n) })
				}
			}
			f.everOnline = true
		}
	case pktOffline:
		if f.online {
			f.online = false
			f.lastSeen = time.Now()
			t.save()
			if cb := t.cbConn; cb != nil {
				n := f.num
				t.emit(func() { cb(n, false) })
			}
		}
	case pktNickname:
		if len(d) <= 129 {
			name := string(d[1:])
			if name != f.name {
				f.name = name
				t.save() // keep the saved friend name current
			}
		}
	case pktMessage, pktAction:
		if len(d) > 1 {
			t.logf("friend %s: message received (%d bytes)", keyHex(f.realPK)[:8], len(d)-1)
			msg := Message{Friend: f.num, Name: f.name, Text: string(d[1:]), Time: time.Now()}
			if len(t.inbox) >= 1000 {
				t.inbox = t.inbox[1:]
			}
			t.inbox = append(t.inbox, msg)
			if cb := t.cbMessage; cb != nil {
				t.emit(func() { cb(msg.Friend, msg.Text) })
			}
		}
	case pktFileSendReq:
		t.handleFileSendReq(f, d[1:])
	case pktFileControl:
		t.handleFileControl(f, d[1:])
	case pktFileData:
		t.handleFileData(f, d[1:])
	default:
		t.logf("friend %s: ignoring packet id %d (%d bytes)", keyHex(f.realPK)[:8], d[0], len(d))
	}
}

func (t *Tox) cryptoTick(f *friend, now time.Time) {
	c := f.conn
	t.debugStatus(f, c, now)
	if c.status != csEstablished && now.Sub(c.tempSent) >= time.Second {
		if c.tempTries >= 10 {
			// handshake stalled: start over with a fresh cookie request
			t.killConn(f)
			return
		}
		t.sendTemp(c)
	}
	if c.status >= csNotConfirmed && now.Sub(c.lastReqSent) >= time.Second {
		t.sendRequest(c)
	}
	if c.status == csEstablished {
		t.resendUnacked(c)
	}
	if c.status == csEstablished {
		if t.hasRoute(c.peerDHT) {
			c.lastRouted = now
		} else if now.Sub(c.lastRouted) > 20*time.Second {
			// Without a routed relay connection our data can't reach a toxcore
			// peer (it drops out-of-band data), so it will consider us offline.
			t.logf("friend %s: no routed relay connection for 20s, reconnecting", keyHex(f.realPK)[:8])
			t.killConn(f)
			return
		}
		if now.Sub(c.lastShare) >= 2*time.Minute {
			t.shareRelays(f, c)
		}
		if now.Sub(c.lastAliveSent) >= 8*time.Second {
			c.lastAliveSent = now
			t.sendLossless(c, []byte{pktAlive})
		}
		t.fileTick(f, c) // pull file chunks from the sender as the window drains
		if now.Sub(c.lastAlive) > 32*time.Second {
			t.logf("friend %s timed out", keyHex(f.realPK)[:8])
			t.killConn(f)
		}
	}
}
