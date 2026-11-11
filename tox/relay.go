package tox

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

// TCP relay packet IDs (TCP_common.h).
const (
	tcpRoutingRequest  = 0
	tcpRoutingResponse = 1
	tcpConnNotify      = 2
	tcpDisconnNotify   = 3
	tcpPing            = 4
	tcpPong            = 5
	tcpOOBSend         = 6
	tcpOOBRecv         = 7
	tcpOnionRequest    = 8
	tcpOnionResponse   = 9
	tcpReservedIDs     = 16
)

type route struct {
	pk     Key
	id     byte // 0 until the relay assigned a connection id
	online bool
}

// relay is a client connection to a toxcore TCP relay server.
type relay struct {
	t    *Tox
	node Node
	conn net.Conn

	wmu       sync.Mutex
	shared    Key
	sendNonce Nonce
	recvNonce Nonce
	out       chan []byte

	// guarded by t.mu
	alive  bool
	routes map[Key]*route
	byID   map[byte]*route
}

// dialRelay performs the TCP relay handshake using our DHT keypair.
func dialRelay(t *Tox, n Node) (*relay, error) {
	c, err := net.DialTimeout("tcp", n.addr(), 8*time.Second)
	if err != nil {
		return nil, err
	}
	r := &relay{t: t, node: n, conn: c, routes: map[Key]*route{}, byID: map[byte]*route{}}

	tempPK, tempSK := newKeyPair()
	r.sendNonce = randomNonce()
	r.shared = precompute(&n.PK, &t.dhtSK)

	// [our pk][nonce][enc(temp pk, base nonce)]
	plain := append(append([]byte{}, tempPK[:]...), r.sendNonce[:]...)
	nonce := randomNonce()
	hs := append(append(append([]byte{}, t.dhtPK[:]...), nonce[:]...), seal(&r.shared, &nonce, plain)...)

	c.SetDeadline(time.Now().Add(8 * time.Second))
	if _, err := c.Write(hs); err != nil {
		c.Close()
		return nil, err
	}
	resp := make([]byte, 24+56+macSize)
	if _, err := io.ReadFull(c, resp); err != nil {
		c.Close()
		return nil, err
	}
	var rn Nonce
	copy(rn[:], resp[:24])
	pl, ok := open(&r.shared, &rn, resp[24:])
	if !ok || len(pl) != 56 {
		c.Close()
		return nil, errors.New("relay handshake failed")
	}
	var serverTemp Key
	copy(serverTemp[:], pl[:32])
	copy(r.recvNonce[:], pl[32:56])
	r.shared = precompute(&serverTemp, &tempSK)
	c.SetDeadline(time.Time{})
	r.out = make(chan []byte, 2048) // hold a full send window without dropping
	go r.writeLoop()
	return r, nil
}

// queue sends a payload asynchronously, preserving order. Safe to call with t.mu held.
func (r *relay) queue(payload []byte) {
	select {
	case r.out <- payload:
	default: // relay is backed up; drop like toxcore does
	}
}

func (r *relay) writeLoop() {
	for p := range r.out {
		if p == nil || r.send(p) != nil {
			return
		}
	}
}

func (r *relay) send(payload []byte) error {
	r.wmu.Lock()
	defer r.wmu.Unlock()
	enc := seal(&r.shared, &r.sendNonce, payload)
	incrementNonce(&r.sendNonce, 1)
	pkt := binary.BigEndian.AppendUint16(nil, uint16(len(enc)))
	pkt = append(pkt, enc...)
	r.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_, err := r.conn.Write(pkt)
	if err != nil {
		r.conn.Close()
	}
	return err
}

func (r *relay) readLoop() {
	defer r.conn.Close()
	hdr := make([]byte, 2)
	for {
		r.conn.SetReadDeadline(time.Now().Add(3 * time.Minute))
		if _, err := io.ReadFull(r.conn, hdr); err != nil {
			break
		}
		n := binary.BigEndian.Uint16(hdr)
		if n <= macSize || n > 4096 {
			break
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r.conn, buf); err != nil {
			break
		}
		plain, ok := open(&r.shared, &r.recvNonce, buf)
		incrementNonce(&r.recvNonce, 1)
		if !ok || len(plain) == 0 {
			break
		}
		r.t.handleRelayPacket(r, plain)
	}
	r.t.relayDown(r)
}

// The methods below are called with t.mu held.

func (r *relay) requestRoute(pk Key) {
	if _, ok := r.routes[pk]; ok {
		return
	}
	r.routes[pk] = &route{pk: pk}
	r.queue(append([]byte{tcpRoutingRequest}, pk[:]...))
}

func (r *relay) isOnline(pk Key) bool {
	rt := r.routes[pk]
	return rt != nil && rt.online
}

// sendTo sends a packet to a peer through this relay, routed if possible, else out-of-band.
func (r *relay) sendTo(pk Key, data []byte) {
	if rt := r.routes[pk]; rt != nil && rt.online {
		r.queue(append([]byte{rt.id}, data...))
		return
	}
	pkt := append([]byte{tcpOOBSend}, pk[:]...)
	r.queue(append(pkt, data...))
}

func (t *Tox) handleRelayPacket(r *relay, p []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch p[0] {
	case tcpRoutingResponse:
		if len(p) != 34 || p[1] == 0 {
			return
		}
		var pk Key
		copy(pk[:], p[2:])
		rt := r.routes[pk]
		if rt == nil {
			rt = &route{pk: pk}
			r.routes[pk] = rt
		}
		rt.id = p[1]
		r.byID[p[1]] = rt
	case tcpConnNotify:
		if len(p) == 2 {
			if rt := r.byID[p[1]]; rt != nil {
				rt.online = true
				t.logf("relay %s: peer %s online", r.node, keyHex(rt.pk)[:8])
				t.peerRouteOnline(rt.pk)
			}
		}
	case tcpDisconnNotify:
		if len(p) == 2 {
			if rt := r.byID[p[1]]; rt != nil {
				rt.online = false
				t.logf("relay %s: peer %s offline", r.node, keyHex(rt.pk)[:8])
			}
		}
	case tcpPing:
		if len(p) == 9 {
			pong := append([]byte{tcpPong}, p[1:]...)
			r.queue(pong)
		}
	case tcpOOBRecv:
		if len(p) > 33 {
			var pk Key
			copy(pk[:], p[1:33])
			t.handleCryptoPacket(r, pk, p[33:])
		}
	case tcpOnionResponse:
		if len(p) > 1 {
			t.handleOnionResponse(p[1:])
		}
	default:
		if p[0] >= tcpReservedIDs && len(p) > 1 {
			if rt := r.byID[p[0]]; rt != nil {
				t.handleCryptoPacket(r, rt.pk, p[1:])
			}
		}
	}
}
