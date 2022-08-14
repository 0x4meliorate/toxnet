package net

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	tox "github.com/TokTok/go-toxcore-c"
)

var Tox_instance *tox.Tox

func ToxStart() {

	opt := tox.NewToxOptions()

	if tox.FileExist(Tox_key) {
		data, err := ioutil.ReadFile(Tox_key)
		if err != nil {
			fmt.Println("[-] Error: Failed to read key -", Tox_key, err)
		} else {
			opt.Savedata_data = data
			opt.Savedata_type = tox.SAVEDATA_TYPE_TOX_SAVE
		}
	}

	opt.Tcp_port = 33445
	for i := 0; i < 5; i++ {
		Tox_instance = tox.NewTox(opt)
		if Tox_instance == nil {
			opt.Tcp_port++
		} else {
			break
		}
	}
}

type BootstrapServers struct {
	LastScan    int `json:"last_scan"`
	LastRefresh int `json:"last_refresh"`
	Nodes       []struct {
		Ipv4       string `json:"ipv4"`
		Ipv6       string `json:"ipv6"`
		Port       int    `json:"port"`
		TCPPorts   []int  `json:"tcp_ports"`
		PublicKey  string `json:"public_key"`
		Maintainer string `json:"maintainer"`
		Location   string `json:"location"`
		StatusUDP  bool   `json:"status_udp"`
		StatusTCP  bool   `json:"status_tcp"`
		Version    string `json:"version"`
		Motd       string `json:"motd"`
		LastPing   int    `json:"last_ping"`
	} `json:"nodes"`
}

func GetBootstraps() BootstrapServers {
	// Tox bootstrap nodes
	url := "https://nodes.tox.chat/json"
	client := http.Client{Timeout: time.Second * 2}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		fmt.Println("[-] Error: Failed to make request context -", err)
	}

	res, err := client.Do(req)
	if err != nil {
		fmt.Println("[-] Error: Failed to send/recieve on request -", err)
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println("[-] Error: Failed to read response body -", err)
	}

	servers := BootstrapServers{}
	err = json.Unmarshal(body, &servers)
	if err != nil {
		fmt.Println("[-] Error: Failed to parse JSON data -", err)
	}

	return servers
}

func Bootstrap() {

	servers := GetBootstraps()

	var server = []interface{}{
		servers.Nodes[0].Ipv4, uint16(servers.Nodes[0].Port), servers.Nodes[0].PublicKey,
	}
	r, err := Tox_instance.Bootstrap(server[0].(string), server[1].(uint16), server[2].(string))
	r1, err1 := Tox_instance.AddTcpRelay(server[0].(string), server[1].(uint16), server[2].(string))
	if err != nil && err1 != nil {
		fmt.Println("[-] Error: Failed to use bootstrap node -", r, r1, err, err1)
	}
}

func ShowC2() {

	colorReset := "\033[0m"
	colorRed := "\033[31m"
	colorYellow := "\033[33m"
	colorBlack := "\033[30m"

	fmt.Println(string(colorYellow), `
	 _____                 __       _   
	/__   \ ___ __  __  /\ \ \ ___ | |_ 
	  / /\// _ \\ \/ / /  \/ // _ \| __|
	 / /  | (_) |>  < / /\  /|  __/| |_ 
	 \/    \___//_/\_\\_\ \/  \___| \__|
	`, string(colorReset))

	fmt.Println("")
	fmt.Println("\t\t\t\t\t", string(colorBlack), strings.Repeat("#", 80), string(colorReset))
	fmt.Println("\t\t\t\t\t", string(colorRed), "TOX-ID:", Tox_instance.SelfGetAddress(), string(colorReset), "Add me!")
	fmt.Println("\t\t\t\t\t", string(colorRed), "PUB-KEY:", Tox_instance.SelfGetPublicKey(), string(colorReset))
	fmt.Println("\t\t\t\t\t", string(colorBlack), strings.Repeat("#", 80), string(colorReset))
	fmt.Println("")
}

func ToxWrite() {
	err := Tox_instance.WriteSavedata(Tox_key)
	if err != nil {
		fmt.Println("[-] Error: Failed to write tox data in 'Tox_key' -", err)
	}
}
