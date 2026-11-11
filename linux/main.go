// Command remote is an SSH-like remote shell for your own machine, over Tox,
// built on the pure-Go tox library. It is the Linux counterpart of the
// assembly remote.exe (Windows); the same program also runs on Windows/macOS.
//
// It accepts commands only from one owner Tox ID (ownerID below), which the web
// UI patches into the binary when you download it. You can also pass the owner
// Tox ID as the first argument. Each message from the owner is run with the
// system shell and its combined output is sent back.
//
// The owner key is the one credential, like an SSH private key. There is no
// stealth or persistence - it is a visible program you run on a machine you
// administer.
package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"toxnet/tox"
)

// cwd is the working directory carried across commands (like a shell session).
// Commands run one at a time (see the queue in main), so it needs no locking.
var cwd string

// cwdMarker separates command output from the trailing pwd we append.
const cwdMarker = "__TOXNET_CWD_5b1c9d__"

// ownerID is patched to the web client's Tox ID at download time. It is exactly
// 76 bytes so the replacement is the same length. It can also be overridden by
// passing a Tox ID as the first command-line argument.
var ownerID = "TOXNET_OWNER_ID_PLACEHOLDER_000000000000000000000000000000000000000000000000"

func main() {
	id := ownerID
	if len(os.Args) > 1 {
		id = os.Args[1]
	}

	tx, err := tox.New(tox.Options{Name: "toxnet-" + runtime.GOOS})
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Close()

	nodes, err := tox.FetchNodes(context.Background())
	if err != nil || len(nodes) == 0 {
		nodes = tox.DefaultNodes
	}
	if len(nodes) > 12 {
		nodes = nodes[:12]
	}
	tx.BootstrapAll(nodes)

	friend, err := tx.AddFriend(id, "")
	if err != nil {
		log.Fatalf("owner Tox ID not set or invalid - download this from the web UI, or pass the owner Tox ID as an argument: %v", err)
	}
	cwd, _ = os.Getwd()

	// Run commands serially so the working directory stays consistent.
	cmds := make(chan string, 64)
	go func() {
		for cmdline := range cmds {
			send(tx, friend, run(cmdline))
		}
	}()
	// Serve file sends: when the owner asks (FTSEND<path>), send the file over
	// the Tox file-transfer protocol instead of running a shell command. The
	// library requests each chunk as the send window drains; we read and supply
	// it. All three callbacks and OnFriendMessage run on the same event
	// goroutine, so sendFiles needs no locking.
	tx.OnFileChunkRequest(func(f, fileNum uint32, pos uint64, length int) {
		file := sendFiles[fileNum]
		if file == nil {
			return
		}
		if length == 0 { // transfer complete
			file.Close()
			delete(sendFiles, fileNum)
			return
		}
		buf := make([]byte, length)
		n, err := file.ReadAt(buf, int64(pos))
		if n > 0 {
			tx.FileSendChunk(f, fileNum, pos, buf[:n])
		}
		if n == 0 && err != nil { // unreadable: abort the transfer
			tx.FileControl(f, fileNum, tox.FileControlCancel)
			file.Close()
			delete(sendFiles, fileNum)
		}
	})
	tx.OnFileControl(func(f, fileNum uint32, control int) {
		if control == tox.FileControlCancel {
			if file := sendFiles[fileNum]; file != nil {
				file.Close()
				delete(sendFiles, fileNum)
			}
		}
	})
	tx.OnFriendMessage(func(f uint32, msg string) {
		if f != friend {
			return
		}
		if strings.HasPrefix(msg, ftSendPrefix) {
			startSend(tx, f, msg[len(ftSendPrefix):])
			return
		}
		select {
		case cmds <- msg:
		default:
		}
	})
	select {} // run until the process is closed
}

// ftSendPrefix marks an owner message as a request to send a file (not a shell
// command): the prefix followed by the path. Must match the web server.
const ftSendPrefix = "FTSEND\x00"

// sendFiles holds the open files we are sending, keyed by file number. Accessed
// only from the Tox event goroutine (see the callbacks in main), so no locking.
var sendFiles = map[uint32]*os.File{}

// startSend opens path and offers it to the owner over the file-transfer
// protocol. On any failure it reports the reason back as a normal message.
func startSend(tx *tox.Tox, friend uint32, path string) {
	file, err := os.Open(path)
	if err != nil {
		send(tx, friend, "cannot open "+path+": "+err.Error())
		return
	}
	st, err := file.Stat()
	if err != nil || st.IsDir() {
		file.Close()
		send(tx, friend, "cannot send "+path+": not a regular file")
		return
	}
	num, err := tx.FileSend(friend, tox.FileKindData, uint64(st.Size()), nil, filepath.Base(path))
	if err != nil {
		file.Close()
		send(tx, friend, "cannot start transfer of "+path+": "+err.Error())
		return
	}
	sendFiles[num] = file
}

// run executes one command starting in cwd, then updates cwd from the shell's
// final directory, so "cd foo" (or "cd foo && ..." ) persists to later commands.
func run(cmdline string) string {
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		full := "cd /d \"" + cwd + "\" & " + cmdline + " & echo " + cwdMarker + " & cd"
		c = exec.Command("cmd", "/c", full)
	} else {
		full := "cd '" + strings.ReplaceAll(cwd, "'", `'\''`) + "' 2>/dev/null; " +
			cmdline + "; printf '%s\\n' " + cwdMarker + "; pwd"
		c = exec.Command("/bin/sh", "-c", full)
	}
	out, err := c.CombinedOutput()

	body := out
	if i := bytes.Index(out, []byte(cwdMarker)); i >= 0 {
		body = bytes.TrimRight(out[:i], "\r\n")
		rest := bytes.TrimLeft(out[i+len(cwdMarker):], "\r\n")
		if nl := bytes.IndexAny(rest, "\r\n"); nl >= 0 {
			rest = rest[:nl]
		}
		if nc := string(rest); nc != "" {
			cwd = nc
		}
	}
	if len(body) == 0 {
		if err != nil {
			return err.Error()
		}
		return "(no output)"
	}
	return string(body)
}

// send delivers text back in chunks as fast as the reliable-send window allows:
// it keeps the window full and only waits (briefly) when the peer hasn't
// acknowledged enough yet, which the library reports as ErrSendBusy.
func send(tx *tox.Tox, friend uint32, text string) {
	b := []byte(text)
	for len(b) > 0 {
		n := len(b)
		if n > 1300 {
			n = 1300
		}
		err := tx.SendMessage(friend, string(b[:n]))
		if errors.Is(err, tox.ErrSendBusy) {
			time.Sleep(time.Millisecond)
			continue
		}
		if err != nil {
			return // owner went offline
		}
		b = b[n:]
	}
}
