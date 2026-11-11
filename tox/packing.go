package tox

import (
	"encoding/binary"
	"fmt"
	"net"
)

// Wire family values used by toxcore.
const (
	famIPv4    = 2
	famIPv6    = 10
	famTCPIPv4 = 130
	famTCPIPv6 = 138
)

// Node is a DHT node or TCP relay: a public key plus an address.
type Node struct {
	PK   Key
	IP   net.IP
	Port uint16
	TCP  bool // true if this describes a TCP relay
}

func (n Node) addr() string { return net.JoinHostPort(n.IP.String(), fmt.Sprint(n.Port)) }

func (n Node) String() string { return n.addr() + "/" + keyHex(n.PK)[:8] }

func (n Node) usable() bool {
	return n.IP != nil && !n.IP.IsUnspecified() && !n.IP.IsLoopback() && n.Port != 0
}

const sizeIPPort = 1 + 16 + 2 // onion ip_port encoding

// packIPPort writes the fixed 19-byte onion ip_port encoding.
func packIPPort(n Node) []byte {
	b := make([]byte, sizeIPPort)
	if ip4 := n.IP.To4(); ip4 != nil {
		b[0] = famIPv4
		copy(b[1:], ip4)
	} else {
		b[0] = famIPv6
		copy(b[1:], n.IP.To16())
	}
	binary.BigEndian.PutUint16(b[17:], n.Port)
	return b
}

// packNodes writes toxcore's variable-length packed node format.
func packNodes(nodes []Node) []byte {
	var out []byte
	for _, n := range nodes {
		if ip4 := n.IP.To4(); ip4 != nil {
			f := byte(famIPv4)
			if n.TCP {
				f = famTCPIPv4
			}
			out = append(out, f)
			out = append(out, ip4...)
		} else {
			f := byte(famIPv6)
			if n.TCP {
				f = famTCPIPv6
			}
			out = append(out, f)
			out = append(out, n.IP.To16()...)
		}
		out = binary.BigEndian.AppendUint16(out, n.Port)
		out = append(out, n.PK[:]...)
	}
	return out
}

func unpackNodes(data []byte) []Node {
	var nodes []Node
	for len(data) > 0 {
		var ipLen int
		var tcp bool
		switch data[0] {
		case famIPv4:
			ipLen = 4
		case famTCPIPv4:
			ipLen, tcp = 4, true
		case famIPv6:
			ipLen = 16
		case famTCPIPv6:
			ipLen, tcp = 16, true
		default:
			return nodes
		}
		need := 1 + ipLen + 2 + 32
		if len(data) < need {
			return nodes
		}
		n := Node{TCP: tcp}
		n.IP = append(net.IP(nil), data[1:1+ipLen]...)
		n.Port = binary.BigEndian.Uint16(data[1+ipLen:])
		copy(n.PK[:], data[3+ipLen:need])
		nodes = append(nodes, n)
		data = data[need:]
	}
	return nodes
}
