// Package tox is a tiny, pure Go subset of c-toxcore.
//
// It supports: bootstrapping from DHT nodes, connecting to TCP relays,
// announcing ourselves and finding friends over the onion, sending friend
// requests, detecting when a friend accepted (comes online), sending and
// receiving messages over net_crypto, and sending and receiving files (see
// file.go). All traffic goes through TCP relays
// (like toxcore with UDP disabled), so it works behind NAT. Every key lives
// only in memory and a new identity is generated for each instance.
package tox

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Options configures a Tox instance.
type Options struct {
	Name string                        // nickname sent to friends (optional)
	Logf func(format string, a ...any) // debug logger (optional)

	// SavePath, if set, is a file holding the long-term identity (so the Tox
	// ID is stable across restarts) and the friend list. It is created on
	// first run and rewritten when the friend list changes.
	SavePath string

	// AutoAccept, if true, automatically accepts every incoming friend request.
	AutoAccept bool
}

type friend struct {
	num         uint32
	realPK      Key
	nospam      [4]byte
	requestMsg  string
	sendRequest bool
	lastReqSent time.Time

	search       *onionTarget
	dhtPK        Key
	hasDHT       bool
	lastNoReplay uint64
	lastDHTPK    time.Time

	conn       *cryptoConn
	online     bool
	everOnline bool
	removed    bool
	name       string
	localName  string // nickname we assigned locally
	note       string
	addedByUs  bool      // we sent the friend request (AddFriend), not auto-accepted
	lastSeen   time.Time // last time we observed them online

	// File transfers (c-toxcore style, see file.go), guarded by Tox.mu.
	// Keyed by the sender-assigned slot (0..255). fileSend holds transfers we
	// are sending; fileRecv holds transfers we are receiving.
	fileSend   map[uint8]*fileTransfer
	fileRecv   map[uint8]*fileTransfer
	numSending int // count of fileSend entries actively transferring (pump gate)
}

type relayInfo struct {
	nodes       []Node // same relay, possibly several ports
	next        int
	r           *relay
	dialing     bool
	lastAttempt time.Time
}

// Message is a text message received from a friend.
type Message struct {
	Friend uint32
	Name   string // friend's nickname when the message arrived
	Text   string
	Time   time.Time
}

// ReceivedMessages returns the messages received since the last call and
// clears the queue. Poll it as often as you like; it never blocks.
func (t *Tox) ReceivedMessages() []Message {
	t.mu.Lock()
	defer t.mu.Unlock()
	msgs := t.inbox
	t.inbox = nil
	return msgs
}

// FriendInfo is a snapshot of one friend.
type FriendInfo struct {
	Num       uint32 `json:"num"`
	PublicKey string `json:"publicKey"`
	Name      string `json:"name"`      // nickname the friend broadcasts
	LocalName string `json:"localName"` // nickname we assigned locally
	Note      string `json:"note"`
	AddedByUs bool   `json:"addedByUs"` // we added them via Add Friend, vs auto-accepted
	Online    bool   `json:"online"`
	LastSeen  int64  `json:"lastSeen"` // unix seconds, 0 if never seen online
}

// Friends returns a snapshot of the current friend list.
func (t *Tox) Friends() []FriendInfo {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := []FriendInfo{}
	for _, f := range t.friends {
		if f.removed {
			continue
		}
		var seen int64
		if !f.lastSeen.IsZero() {
			seen = f.lastSeen.Unix()
		}
		out = append(out, FriendInfo{Num: f.num, PublicKey: keyHex(f.realPK),
			Name: f.name, LocalName: f.localName, Note: f.note, AddedByUs: f.addedByUs, Online: f.online, LastSeen: seen})
	}
	return out
}

// SetFriendNote sets a free-text note on a friend and persists it.
func (t *Tox) SetFriendNote(friend uint32, note string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	f := t.friendAt(friend)
	if f == nil {
		return errors.New("tox: no such friend")
	}
	if len(note) > 4096 {
		note = note[:4096]
	}
	f.note = note
	t.save()
	return nil
}

// SetFriendLocalName sets a local nickname for a friend (our own label, not
// the name they broadcast) and persists it.
func (t *Tox) SetFriendLocalName(friend uint32, name string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	f := t.friendAt(friend)
	if f == nil {
		return errors.New("tox: no such friend")
	}
	if len(name) > 255 {
		name = name[:255]
	}
	f.localName = name
	t.save()
	return nil
}

// Tox is one in-memory Tox identity.
type Tox struct {
	mu   sync.Mutex
	opts Options

	realPK, realSK   Key // long-term identity (Tox ID)
	dhtPK, dhtSK     Key // DHT / TCP relay identity
	onionPK, onionSK Key // data key announced on the onion
	nospam           [4]byte
	cookieKey        Key
	noReplay         uint64

	udp        *net.UDPConn
	bootstraps []Node
	dhtNodes   []Node
	relays     map[Key]*relayInfo

	self    *onionTarget
	paths   []*onionPath
	pending map[uint64]*pendingAnnounce

	friends []*friend
	inbox   []Message

	autoAccept bool

	cbAccepted func(friend uint32)
	cbConn     func(friend uint32, online bool)
	cbMessage  func(friend uint32, msg string)
	cbRequest  func(publicKey string, msg string)

	// File-transfer callbacks (see file.go). All run on the event goroutine.
	cbFileRecv      func(friend, fileNum uint32, kind uint32, size uint64, name string)
	cbFileRecvChunk func(friend, fileNum uint32, position uint64, data []byte)
	cbFileChunkReq  func(friend, fileNum uint32, position uint64, length int)
	cbFileControl   func(friend, fileNum uint32, control int)

	evMu  sync.Mutex
	evQ   []func()
	evSig chan struct{}

	closed    chan struct{}
	closeOnce sync.Once
}

// New creates a new identity with fresh in-memory keys and starts its event loop.
func New(opts Options) (*Tox, error) {
	t := &Tox{
		opts:    opts,
		relays:  map[Key]*relayInfo{},
		pending: map[uint64]*pendingAnnounce{},
		evSig:   make(chan struct{}, 1),
		closed:  make(chan struct{}),
	}
	t.autoAccept = opts.AutoAccept
	t.realPK, t.realSK = newKeyPair()
	t.dhtPK, t.dhtSK = newKeyPair()
	t.onionPK, t.onionSK = newKeyPair()
	copy(t.nospam[:], randomBytes(4))
	copy(t.cookieKey[:], randomBytes(32))

	// Load a saved identity and friend list, if any (keeps the Tox ID stable).
	savedFriends, err := t.load()
	if err != nil {
		return nil, err
	}
	t.self = &onionTarget{pk: t.realPK, sendPK: t.realPK, sendSK: t.realSK, isSelf: true}
	for _, sf := range savedFriends {
		if f, err := t.addFriendLocked(sf.pk); err == nil {
			f.name = sf.name
			f.localName = sf.localName
			f.note = sf.note
			f.addedByUs = sf.addedByUs
			if sf.lastSeen > 0 {
				f.lastSeen = time.Unix(sf.lastSeen, 0)
			}
		}
	}
	if opts.SavePath != "" {
		if err := t.save(); err != nil {
			return nil, err
		}
	}

	udp, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err == nil {
		t.udp = udp
		go t.udpLoop()
	} else {
		t.logf("UDP disabled: %v", err)
	}
	go t.eventLoop()
	go t.mainLoop()
	return t, nil
}

// Address returns our Tox ID (76 hex characters).
func (t *Tox) Address() string { return makeAddress(t.realPK, t.nospam) }

// PublicKey returns our long-term public key as hex.
func (t *Tox) PublicKey() string { return keyHex(t.realPK) }

// Close shuts everything down. Keys are discarded with the instance.
func (t *Tox) Close() {
	t.closeOnce.Do(func() {
		close(t.closed)
		t.mu.Lock()
		defer t.mu.Unlock()
		if t.udp != nil {
			t.udp.Close()
		}
		for _, ri := range t.relays {
			if ri.r != nil {
				ri.r.conn.Close()
			}
		}
		t.realSK, t.dhtSK, t.onionSK = Key{}, Key{}, Key{}
	})
}

// OnFriendRequestAccepted is called the first time a friend we sent a request to comes online.
func (t *Tox) OnFriendRequestAccepted(fn func(friend uint32)) {
	t.mu.Lock()
	t.cbAccepted = fn
	t.mu.Unlock()
}

// OnFriendConnectionStatus is called whenever a friend goes online or offline.
func (t *Tox) OnFriendConnectionStatus(fn func(friend uint32, online bool)) {
	t.mu.Lock()
	t.cbConn = fn
	t.mu.Unlock()
}

// OnFriendMessage is called for every message received from a friend.
func (t *Tox) OnFriendMessage(fn func(friend uint32, msg string)) {
	t.mu.Lock()
	t.cbMessage = fn
	t.mu.Unlock()
}

// OnFriendRequest is called when someone sends us a friend request.
// Accept it with AddFriendNoRequest(publicKey).
func (t *Tox) OnFriendRequest(fn func(publicKey string, msg string)) {
	t.mu.Lock()
	t.cbRequest = fn
	t.mu.Unlock()
}

func resolve(host string, port uint16) (net.IP, error) {
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {
			return ip4, nil
		}
	}
	return nil, fmt.Errorf("tox: no IPv4 address for %s", host)
}

// Bootstrap adds a DHT bootstrap node. It is queried over UDP for more nodes
// and used as an onion path/announce node.
func (t *Tox) Bootstrap(host string, port uint16, publicKey string) error {
	pk, err := keyFromHex(publicKey)
	if err != nil {
		return err
	}
	ip, err := resolve(host, port)
	if err != nil {
		return err
	}
	n := Node{PK: pk, IP: ip, Port: port}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, b := range t.bootstraps {
		if b.PK == pk {
			return nil
		}
	}
	t.bootstraps = append(t.bootstraps, n)
	t.sendGetNodes(n, t.dhtPK)
	t.sendGetNodes(n, t.realPK)
	for _, f := range t.friends {
		if !f.removed {
			t.sendGetNodes(n, f.realPK)
		}
	}
	return nil
}

// AddTCPRelay adds a TCP relay; we keep a connection to it open.
func (t *Tox) AddTCPRelay(host string, port uint16, publicKey string) error {
	pk, err := keyFromHex(publicKey)
	if err != nil {
		return err
	}
	ip, err := resolve(host, port)
	if err != nil {
		return err
	}
	t.mu.Lock()
	t.addRelay(Node{PK: pk, IP: ip, Port: port, TCP: true}, maxRelays)
	t.mu.Unlock()
	return nil
}

const (
	maxRelays       = 10 // relays we pick ourselves
	maxFriendRelays = 24 // hard cap including relays our friends ask us to use
)

func (t *Tox) addRelay(n Node, limit int) {
	if !n.usable() {
		return
	}
	ri := t.relays[n.PK]
	if ri == nil {
		if len(t.relays) >= limit {
			return
		}
		ri = &relayInfo{}
		t.relays[n.PK] = ri
	}
	for _, e := range ri.nodes {
		if e.IP.Equal(n.IP) && e.Port == n.Port {
			return
		}
	}
	ri.nodes = append(ri.nodes, n)
	t.maybeDial(ri)
}

func (t *Tox) maybeDial(ri *relayInfo) {
	if ri.r != nil || ri.dialing || time.Since(ri.lastAttempt) < 10*time.Second || len(ri.nodes) == 0 {
		return
	}
	n := ri.nodes[ri.next%len(ri.nodes)]
	ri.next++
	ri.dialing = true
	ri.lastAttempt = time.Now()
	go func() {
		r, err := dialRelay(t, n)
		t.mu.Lock()
		defer t.mu.Unlock()
		ri.dialing = false
		if err != nil {
			t.logf("relay %s: %v", n, err)
			return
		}
		select {
		case <-t.closed:
			r.conn.Close()
			return
		default:
		}
		r.alive = true
		ri.r = r
		t.logf("connected to TCP relay %s", n)
		r.queue(append([]byte{tcpPing}, randomBytes(8)...)) // confirms the connection
		for _, f := range t.friends {
			if f.hasDHT && !f.removed {
				r.requestRoute(f.dhtPK)
			}
		}
		go r.readLoop()
	}()
}

func (t *Tox) relayDown(r *relay) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r.alive = false
	select {
	case r.out <- nil:
	default:
	}
	if ri := t.relays[r.node.PK]; ri != nil && ri.r == r {
		ri.r = nil
		t.logf("TCP relay %s disconnected", r.node)
	}
}

func (t *Tox) aliveRelays() []*relay {
	var out []*relay
	for _, ri := range t.relays {
		if ri.r != nil && ri.r.alive {
			out = append(out, ri.r)
		}
	}
	return out
}

// AddFriend sends a friend request to a Tox ID and returns the friend number.
func (t *Tox) AddFriend(toxID, message string) (uint32, error) {
	pk, nospam, err := ParseAddress(toxID)
	if err != nil {
		return 0, err
	}
	if message == "" {
		message = "Hi"
	}
	if len(message) > 900 {
		return 0, errors.New("tox: friend request message too long")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if existing := t.friendByPK(pk); existing != nil {
		// Already a friend (e.g. they added us): claim them as one we added,
		// which moves them into the People group. No duplicate, no new request.
		if !existing.addedByUs {
			existing.addedByUs = true
			t.save()
		}
		return existing.num, nil
	}
	f, err := t.addFriendLocked(pk)
	if err != nil {
		return 0, err
	}
	f.nospam, f.requestMsg, f.sendRequest = nospam, message, true
	f.addedByUs = true
	t.save()
	return f.num, nil
}

// AddFriendNoRequest adds a friend by public key without sending a request
// (used to accept an incoming request).
func (t *Tox) AddFriendNoRequest(publicKey string) (uint32, error) {
	pk, err := keyFromHex(publicKey)
	if err != nil {
		return 0, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	f, err := t.addFriendLocked(pk)
	if err != nil {
		return 0, err
	}
	t.save()
	return f.num, nil
}

func (t *Tox) addFriendLocked(pk Key) (*friend, error) {
	if pk == t.realPK {
		return nil, errors.New("tox: cannot add yourself")
	}
	if t.friendByPK(pk) != nil {
		return nil, errors.New("tox: friend already added")
	}
	tpk, tsk := newKeyPair()
	f := &friend{num: uint32(len(t.friends)), realPK: pk}
	f.search = &onionTarget{pk: pk, sendPK: tpk, sendSK: tsk, friend: f}
	t.friends = append(t.friends, f)
	for _, b := range t.bootstraps {
		t.sendGetNodes(b, pk)
	}
	return f, nil
}

// friendAt returns the live friend with that number, or nil if the number is
// out of range or the friend was removed.
func (t *Tox) friendAt(friend uint32) *friend {
	if int(friend) >= len(t.friends) {
		return nil
	}
	f := t.friends[friend]
	if f.removed {
		return nil
	}
	return f
}

// FriendPublicKey returns a friend's public key as hex.
func (t *Tox) FriendPublicKey(friend uint32) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if f := t.friendAt(friend); f != nil {
		return keyHex(f.realPK)
	}
	return ""
}

// FriendName returns the nickname a friend sent us, if any.
func (t *Tox) FriendName(friend uint32) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if f := t.friendAt(friend); f != nil {
		return f.name
	}
	return ""
}

// IsFriendOnline reports whether the friend is currently connected.
func (t *Tox) IsFriendOnline(friend uint32) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	f := t.friendAt(friend)
	return f != nil && f.online
}

// ErrSendBusy means the reliable-send window is full. Callers sending a stream
// of messages (e.g. a file transfer) should retry shortly; the window drains as
// the peer acknowledges. Control traffic is never throttled this way.
var ErrSendBusy = errors.New("tox: send window full")

// sendWindow caps how many unacknowledged message packets may be in flight.
// It paces a bulk transfer to fill the link without overrunning the relay
// queue, in place of a fixed per-message delay.
const sendWindow = 256

// SendMessage sends a normal text message to an online friend. It returns
// ErrSendBusy when the send window is full (retry shortly).
func (t *Tox) SendMessage(friend uint32, msg string) error {
	if len(msg) == 0 || len(msg) > maxMessageLen {
		return fmt.Errorf("tox: message length must be 1..%d bytes", maxMessageLen)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	f := t.friendAt(friend)
	if f == nil {
		return errors.New("tox: no such friend")
	}
	if !f.online || f.conn == nil {
		return errors.New("tox: friend is not online")
	}
	if f.conn.sendEnd-f.conn.sendStart >= sendWindow {
		return ErrSendBusy
	}
	t.sendLossless(f.conn, append([]byte{pktMessage}, msg...))
	return nil
}

// RemoveFriend deletes a friend: it drops any connection and stops announcing
// to or searching for them. With AutoAccept on, the same person can come back
// if they send a new friend request.
func (t *Tox) RemoveFriend(friend uint32) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	f := t.friendAt(friend)
	if f == nil {
		return errors.New("tox: no such friend")
	}
	t.killConn(f)
	f.removed = true
	f.search = nil
	t.save()
	return nil
}

func (t *Tox) friendByPK(pk Key) *friend {
	for _, f := range t.friends {
		if !f.removed && f.realPK == pk {
			return f
		}
	}
	return nil
}

func (t *Tox) friendByDHT(pk Key) *friend {
	for _, f := range t.friends {
		if !f.removed && f.hasDHT && f.dhtPK == pk {
			return f
		}
	}
	return nil
}

func (t *Tox) logf(format string, a ...any) {
	select {
	case <-t.closed: // no logging after Close
		return
	default:
	}
	if t.opts.Logf != nil {
		t.opts.Logf(format, a...)
	}
}

// emit queues a user callback; callbacks run on their own goroutine, in order.
func (t *Tox) emit(fn func()) {
	t.evMu.Lock()
	t.evQ = append(t.evQ, fn)
	t.evMu.Unlock()
	select {
	case t.evSig <- struct{}{}:
	default:
	}
}

func (t *Tox) eventLoop() {
	for {
		select {
		case <-t.closed:
			return
		case <-t.evSig:
		}
		for {
			t.evMu.Lock()
			q := t.evQ
			t.evQ = nil
			t.evMu.Unlock()
			if len(q) == 0 {
				break
			}
			for _, fn := range q {
				fn()
			}
		}
	}
}

func (t *Tox) mainLoop() {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	lastDHT := time.Now()
	for {
		select {
		case <-t.closed:
			return
		case <-tick.C:
		}
		t.mu.Lock()
		for _, ri := range t.relays {
			t.maybeDial(ri)
		}
		if time.Since(lastDHT) > 30*time.Second {
			lastDHT = time.Now()
			for _, b := range t.bootstraps {
				t.sendGetNodes(b, t.realPK)
			}
		}
		t.onionTick()
		t.friendsTick()
		t.mu.Unlock()
	}
}

func (t *Tox) friendsTick() {
	now := time.Now()
	for _, f := range t.friends {
		if f.removed {
			continue
		}
		if f.online {
			f.lastSeen = now
		}
		found := f.search.storedCount() > 0
		if found && f.sendRequest && !f.everOnline && now.Sub(f.lastReqSent) > 20*time.Second {
			pkt := append([]byte{onionDataFriendReq}, f.nospam[:]...)
			pkt = append(pkt, f.requestMsg...)
			if n := t.sendOnionData(f, pkt); n > 0 {
				f.lastReqSent = now
				t.logf("friend request sent to %s via %d onion nodes", keyHex(f.realPK)[:8], n)
			}
		}
		// Tell the friend our DHT key and relays: often while offline, and every
		// minute while online so they keep connecting to relays we're on.
		dhtpkEvery := 10 * time.Second
		if f.online {
			dhtpkEvery = time.Minute
		}
		if found && now.Sub(f.lastDHTPK) > dhtpkEvery {
			if t.sendDHTPK(f) > 0 {
				f.lastDHTPK = now
			}
		}
		if f.hasDHT && f.conn == nil {
			f.conn = t.newCryptoConn(f, f.dhtPK, true)
		}
		if f.conn != nil {
			t.cryptoTick(f, now)
		}
	}
}
