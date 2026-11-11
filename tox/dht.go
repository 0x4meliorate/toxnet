package tox

import (
	"net"
)

const (
	pktGetNodes  = 0x02
	pktSendNodes = 0x04
)

// sendGetNodes asks a DHT node (over UDP) for the nodes it knows closest to search.
func (t *Tox) sendGetNodes(n Node, search Key) {
	if t.udp == nil {
		return
	}
	plain := append(append([]byte{}, search[:]...), randomBytes(8)...)
	nonce := randomNonce()
	shared := precompute(&n.PK, &t.dhtSK)
	pkt := []byte{pktGetNodes}
	pkt = append(pkt, t.dhtPK[:]...)
	pkt = append(pkt, nonce[:]...)
	pkt = append(pkt, seal(&shared, &nonce, plain)...)
	t.udp.WriteToUDP(pkt, &net.UDPAddr{IP: n.IP, Port: int(n.Port)})
}

func (t *Tox) udpLoop() {
	buf := make([]byte, 2048)
	for {
		n, _, err := t.udp.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-t.closed:
				return
			default:
				continue
			}
		}
		p := buf[:n]
		if len(p) < 1+32+24+macSize+1+8 || p[0] != pktSendNodes {
			continue
		}
		var pk Key
		var nonce Nonce
		copy(pk[:], p[1:33])
		copy(nonce[:], p[33:57])
		shared := precompute(&pk, &t.dhtSK)
		plain, ok := open(&shared, &nonce, p[57:])
		if !ok || len(plain) < 9 {
			continue
		}
		nodes := unpackNodes(plain[1 : len(plain)-8])
		t.mu.Lock()
		for _, nd := range nodes {
			t.learnNode(nd)
		}
		t.mu.Unlock()
	}
}
