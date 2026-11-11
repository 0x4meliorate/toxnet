package tox

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// NodeInfo describes a public bootstrap node / TCP relay.
type NodeInfo struct {
	Host      string
	Port      uint16   // UDP DHT port
	TCPPorts  []uint16 // TCP relay ports
	PublicKey string
}

// DefaultNodes is a small built-in list used when the live list can't be fetched.
var DefaultNodes = []NodeInfo{
	{"tox.abilinski.com", 33445, []uint16{33445}, "10C00EB250C3233E343E2AEBA07115A5C28920E9C8D29492F6D00B29049EDC7E"},
	{"144.217.167.73", 33445, []uint16{33445, 3389}, "7E5668E0EE09E19F320AD47902419331FFEE147BB3606769CFBE921A2A2FD34C"},
	{"tox.novg.net", 33445, []uint16{33445}, "D527E5847F8330D628DAB1814F0A422F6DC9D0A300E6C357634EE2DA88C35463"},
	{"205.185.115.131", 53, []uint16{53, 443, 33445, 3389}, "3091C6BEB2A993F1C6300C16549FABA67098FF3D62C6D253828B531470B53D68"},
	{"tox.kurnevsky.net", 33445, []uint16{33445}, "82EF82BA33445A1F91A7DB27189ECFC0C013E06E3DA71F588ED692BED625EC23"},
	{"tox.plastiras.org", 33445, []uint16{33445, 443}, "8E8B63299B3D520FB377FE5100E65E3322F7AE5B20A0ACED2981769FC5B43725"},
	{"188.225.9.167", 33445, []uint16{33445, 3389}, "1911341A83E02503AB1FD6561BD64AF3A9D6C3F12B5FBB656976B2E678644A67"},
	{"tox.initramfs.io", 33445, []uint16{33445}, "3F0A45A268367C1BEA652F258C85F4A66DA76BCAA667A49E770BCC4917AB6A25"},
}

// FetchNodes downloads the current node list from nodes.tox.chat, keeping only
// nodes that are online over both UDP and TCP.
func FetchNodes(ctx context.Context) ([]NodeInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://nodes.tox.chat/json", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Nodes []struct {
			IPv4      string   `json:"ipv4"`
			Port      uint16   `json:"port"`
			TCPPorts  []uint16 `json:"tcp_ports"`
			PublicKey string   `json:"public_key"`
			StatusUDP bool     `json:"status_udp"`
			StatusTCP bool     `json:"status_tcp"`
		} `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	var out []NodeInfo
	for _, n := range body.Nodes {
		if n.IPv4 == "" || n.IPv4 == "-" || !n.StatusUDP || !n.StatusTCP || len(n.TCPPorts) == 0 {
			continue
		}
		out = append(out, NodeInfo{n.IPv4, n.Port, n.TCPPorts, n.PublicKey})
	}
	return out, nil
}

// BootstrapAll bootstraps from and adds TCP relays for every node in the list.
func (t *Tox) BootstrapAll(nodes []NodeInfo) {
	for _, n := range nodes {
		if err := t.Bootstrap(n.Host, n.Port, n.PublicKey); err != nil {
			t.logf("bootstrap %s: %v", n.Host, err)
			continue
		}
		for _, p := range n.TCPPorts {
			t.AddTCPRelay(n.Host, p, n.PublicKey)
		}
	}
}
