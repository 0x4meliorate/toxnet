package main

// Remote file browser and background download over the remote shell.
//
// The web server sends an OS-specific command (list / file size / base64 of a
// file) wrapped in unique markers; the remote runs it and sends the output back
// as ordinary messages, which the drain loop routes to the waiting request.
// Directory listings and sizes are collected synchronously; file downloads are
// streamed: the base64 reply is decoded to a file on the server as it arrives,
// so a progress bar can track it, and the finished file is served on request.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"toxnet/tox"
)

// msgSink consumes reply messages from a friend until the reply is complete.
type msgSink interface {
	feed(text string) (done bool)
}

// routeToOp feeds a reply message to the friend's pending request. Returns true
// if it was consumed (so it should not go to the console log). feed is only
// called from the single drain goroutine, so it needs no extra locking.
func (s *server) routeToOp(friend uint32, text string) bool {
	s.mu.Lock()
	sink := s.ops[friend]
	s.mu.Unlock()
	if sink == nil {
		return false
	}
	if sink.feed(text) {
		s.mu.Lock()
		if s.ops[friend] == sink {
			delete(s.ops, friend)
		}
		s.mu.Unlock()
	}
	return true
}

// ---- synchronous op (listing, file size) ----

type fileOp struct {
	begin, end string
	buf        strings.Builder
	done       chan string
}

func (op *fileOp) feed(text string) bool {
	op.buf.WriteString(text)
	buf := op.buf.String()
	e := strings.Index(buf, op.end)
	if e < 0 {
		return false
	}
	content := ""
	if b := strings.Index(buf, op.begin); b >= 0 && b+len(op.begin) <= e {
		content = buf[b+len(op.begin) : e]
	}
	select {
	case op.done <- content:
	default:
	}
	return true
}

// friendMu returns the per-friend command lock. The remote has one command
// channel, so every request (list, size, download slice) takes turns on it.
// A download is a loop of small slices, each a separate locked request, so a
// directory listing can slip in between slices - letting you browse while a
// download runs.
func (s *server) friendMu(friend uint32) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.cmdmu[friend]
	if m == nil {
		m = &sync.Mutex{}
		s.cmdmu[friend] = m
	}
	return m
}

// runSync runs one command on the friend's command channel and returns the
// content between the markers. It holds the friend lock for the whole request.
func (s *server) runSync(friend uint32, command, begin, end string, timeout time.Duration) (string, error) {
	if len(command) > 1300 {
		return "", errors.New("path too long")
	}
	fmu := s.friendMu(friend)
	fmu.Lock()
	defer fmu.Unlock()

	op := &fileOp{begin: begin, end: end, done: make(chan string, 1)}
	s.mu.Lock()
	s.ops[friend] = op
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.ops[friend] == op {
			delete(s.ops, friend)
		}
		s.mu.Unlock()
	}()
	if err := s.tx.SendMessage(friend, command); err != nil {
		return "", err
	}
	select {
	case c := <-op.done:
		return c, nil
	case <-time.After(timeout):
		return "", errors.New("the friend did not respond in time")
	}
}

// ---- background download job ----

type dlJob struct {
	ID     string `json:"id"`
	Friend uint32 `json:"friend"`
	Path   string `json:"path"`
	Name   string `json:"name"`

	mu       sync.Mutex
	total    int64
	received int64
	status   string // "running", "done", "error"
	err      string
	file     string

	// protocol-download fields (unused by the shell path)
	fileNum  uint32    // the transfer's file number, once the offer arrives
	bound    bool      // an offer has been matched to this job
	wf       *os.File  // open destination, closed when the transfer completes
	started  time.Time // when the file-send request was issued (for timeout)
	finished time.Time // when the transfer ended (done/error/cancelled)
}

func (j *dlJob) snapshot() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	when := j.started
	if !j.finished.IsZero() {
		when = j.finished
	}
	return map[string]any{
		"id": j.ID, "path": j.Path, "name": j.Name, "friend": j.Friend,
		"total": j.total, "received": j.received, "status": j.status, "error": j.err,
		"time": when,
	}
}
func (j *dlJob) fail(m string) {
	j.mu.Lock()
	if j.status == "running" || j.status == "paused" {
		j.status, j.err, j.finished = "error", m, time.Now()
	}
	j.mu.Unlock()
}

// ---- protocol download (Tox file transfer) ----

// ftSendPrefix marks a message to a Go remote as a request to send a file over
// the file-transfer protocol: ftSendPrefix followed by the remote path. The NUL
// keeps it from colliding with a shell command a user might type in the console.
const ftSendPrefix = "FTSEND\x00"

func friendFileKey(friend, fileNum uint32) uint64 {
	return uint64(friend)<<32 | uint64(fileNum)
}

// setupFileTransfer registers the callbacks that receive a file the remote
// sends (over Tox file transfer) in response to an FTSEND request, writing it to
// the matching job's file and tracking progress.
func (s *server) setupFileTransfer() {
	s.tx.OnFileRecv(func(friend, fileNum uint32, kind uint32, size uint64, name string) {
		s.mu.Lock()
		job := s.popAwaiting(friend)
		if job == nil {
			s.mu.Unlock()
			s.tx.FileControl(friend, fileNum, tox.FileControlCancel) // unsolicited offer
			return
		}
		s.bound[friendFileKey(friend, fileNum)] = job
		s.mu.Unlock()
		job.mu.Lock()
		job.fileNum, job.bound, job.total = fileNum, true, int64(size)
		job.mu.Unlock()
		s.tx.FileControl(friend, fileNum, tox.FileControlResume) // accept it
	})
	s.tx.OnFileRecvChunk(func(friend, fileNum uint32, pos uint64, data []byte) {
		s.mu.Lock()
		job := s.bound[friendFileKey(friend, fileNum)]
		s.mu.Unlock()
		if job == nil {
			return
		}
		if len(data) == 0 { // empty chunk: transfer complete
			job.mu.Lock()
			if job.wf != nil {
				job.wf.Close()
				job.wf = nil
			}
			if job.status == "running" || job.status == "paused" {
				job.status, job.finished = "done", time.Now()
			}
			job.mu.Unlock()
			s.mu.Lock()
			delete(s.bound, friendFileKey(friend, fileNum))
			s.mu.Unlock()
			return
		}
		job.mu.Lock()
		if job.wf != nil {
			job.wf.WriteAt(data, int64(pos))
			if end := int64(pos) + int64(len(data)); end > job.received {
				job.received = end
			}
		}
		job.mu.Unlock()
	})
	s.tx.OnFileControl(func(friend, fileNum uint32, control int) {
		if control != tox.FileControlCancel {
			return
		}
		s.mu.Lock()
		key := friendFileKey(friend, fileNum)
		job := s.bound[key]
		delete(s.bound, key)
		s.mu.Unlock()
		if job == nil {
			return
		}
		job.mu.Lock()
		if job.wf != nil {
			job.wf.Close()
			job.wf = nil
		}
		job.mu.Unlock()
		job.fail("the remote cancelled the transfer")
	})
}

// popAwaiting removes and returns the oldest job for friend awaiting an offer.
// Offers arrive in the order requests were sent, so FIFO matches them correctly.
// Call with s.mu held.
func (s *server) popAwaiting(friend uint32) *dlJob {
	q := s.awaiting[friend]
	if len(q) == 0 {
		return nil
	}
	job := q[0]
	s.awaiting[friend] = q[1:]
	return job
}

// dropAwaiting removes job from friend's awaiting queue. Call with s.mu held.
func (s *server) dropAwaiting(friend uint32, job *dlJob) {
	q := s.awaiting[friend]
	for i, j := range q {
		if j == job {
			s.awaiting[friend] = append(q[:i:i], q[i+1:]...)
			return
		}
	}
}

// maxDownloadID returns the highest integer filename in dir, so the id counter
// resumes past files kept from a previous run and never reuses one.
func maxDownloadID(dir string) uint64 {
	var max uint64
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if n, err := strconv.ParseUint(e.Name(), 10, 64); err == nil && n > max {
			max = n
		}
	}
	return max
}

// restoreDownloads repopulates the in-memory downloads store from the activity
// log at startup (files still present on disk), so the Loot panel keeps showing
// previously received files across restarts. Called before any goroutine runs.
func (s *server) restoreDownloads() {
	for _, d := range s.db.finishedDownloads() {
		file := filepath.Join(s.dldir, d.ID)
		if _, err := os.Stat(file); err != nil {
			continue // the file is gone; don't list a dead entry
		}
		s.jobs[d.ID] = &dlJob{ID: d.ID, Friend: d.Friend, Path: d.Path, Name: d.Name,
			total: d.Size, received: d.Received, status: "done", file: file,
			started: d.Started, finished: d.Finished}
		s.jobList = append(s.jobList, d.ID)
		for len(s.jobList) > 200 {
			delete(s.jobs, s.jobList[0])
			s.jobList = s.jobList[1:]
		}
	}
}

// nextDownloadID hands out the next sequential download id. Ids are unique for
// the life of the downloads folder, so files are stored under the id and never
// collide; the real filename is applied only when the file is saved.
func (s *server) nextDownloadID() string {
	s.mu.Lock()
	s.dlSeq++
	id := strconv.FormatUint(s.dlSeq, 10)
	s.mu.Unlock()
	return id
}

// startProtocolDownload asks the remote to send a file and registers a job that
// receives it over the file-transfer protocol as the chunks arrive.
func (s *server) startProtocolDownload(num uint32, path string) *dlJob {
	id := s.nextDownloadID()
	job := &dlJob{ID: id, Friend: num, Path: path, Name: baseName(path),
		status: "running", started: time.Now()}
	wf, err := os.Create(filepath.Join(s.dldir, id))
	if err != nil {
		job.status, job.err = "error", "cannot create file on server"
		return job
	}
	job.wf, job.file = wf, wf.Name()

	s.mu.Lock()
	s.jobs[id] = job
	s.jobList = append(s.jobList, id)
	for len(s.jobList) > 200 {
		delete(s.jobs, s.jobList[0])
		s.jobList = s.jobList[1:]
	}
	s.awaiting[num] = append(s.awaiting[num], job)
	s.mu.Unlock()

	if err := s.tx.SendMessage(num, ftSendPrefix+path); err != nil {
		s.mu.Lock()
		s.dropAwaiting(num, job)
		s.mu.Unlock()
		job.mu.Lock()
		if job.wf != nil {
			job.wf.Close()
			job.wf = nil
		}
		job.mu.Unlock()
		job.fail("could not reach the remote")
		return job
	}
	go s.offerTimeout(num, job)
	return job
}

// controlJob pauses, resumes or cancels a download. Pause/resume ask the remote
// (via the file-transfer protocol) to stop or restart sending; cancel aborts the
// transfer and discards the partial file.
func (s *server) controlJob(job *dlJob, action string) error {
	job.mu.Lock()
	friend, fileNum, bound, status := job.Friend, job.fileNum, job.bound, job.status
	job.mu.Unlock()

	switch action {
	case "pause":
		if !bound || status != "running" {
			return errors.New("not downloading")
		}
		if err := s.tx.FileControl(friend, fileNum, tox.FileControlPause); err != nil {
			return err
		}
		job.mu.Lock()
		if job.status == "running" {
			job.status = "paused"
		}
		job.mu.Unlock()
	case "resume":
		if !bound || status != "paused" {
			return errors.New("not paused")
		}
		if err := s.tx.FileControl(friend, fileNum, tox.FileControlResume); err != nil {
			return err
		}
		job.mu.Lock()
		if job.status == "paused" {
			job.status = "running"
		}
		job.mu.Unlock()
	case "cancel":
		if status == "done" || status == "cancelled" {
			return nil
		}
		if bound {
			s.tx.FileControl(friend, fileNum, tox.FileControlCancel)
		}
		s.mu.Lock()
		delete(s.bound, friendFileKey(friend, fileNum))
		s.dropAwaiting(friend, job)
		s.mu.Unlock()
		job.mu.Lock()
		if job.wf != nil {
			job.wf.Close()
			job.wf = nil
		}
		if job.status != "done" {
			job.status, job.finished = "cancelled", time.Now()
		}
		job.mu.Unlock()
	default:
		return errors.New("unknown action")
	}
	return nil
}

// friendLabel is a human name for a friend: the nickname we assigned locally if
// any, otherwise a 12-character prefix of their Tox ID. We deliberately ignore
// the broadcast name, which the friend controls, matching the friends list.
func (s *server) friendLabel(num uint32) string {
	for _, f := range s.tx.Friends() {
		if f.Num == num {
			if f.LocalName != "" {
				return f.LocalName
			}
			if len(f.PublicKey) >= 12 {
				return f.PublicKey[:12]
			}
		}
	}
	return "#" + strconv.FormatUint(uint64(num), 10)
}

// offerTimeout fails a job if the remote never offers the file (e.g. a bad path,
// or a remote too old to speak the protocol).
func (s *server) offerTimeout(friend uint32, job *dlJob) {
	time.Sleep(30 * time.Second)
	job.mu.Lock()
	bound := job.bound
	job.mu.Unlock()
	if bound {
		return
	}
	s.mu.Lock()
	s.dropAwaiting(friend, job)
	s.mu.Unlock()
	job.mu.Lock()
	if job.wf != nil {
		job.wf.Close()
		job.wf = nil
	}
	job.mu.Unlock()
	job.fail("the remote did not start the transfer")
}

// ---- commands ----

func (s *server) osOf(num uint32) string {
	for _, f := range s.tx.Friends() {
		if f.Num == num {
			n := strings.ToLower(f.Name)
			switch {
			case strings.Contains(n, "windows"):
				return "windows"
			case strings.Contains(n, "linux"), strings.Contains(n, "darwin"):
				return "unix"
			}
		}
	}
	return ""
}

func token() string {
	var b [6]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// markers use a '.', which never appears in base64, so a marker can never be a
// substring of the file content we download.
func markers() (string, string, string) {
	t := token()
	return t, "FS.B." + t, "FS.E." + t
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func psEncode(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, len(u)*2)
	for i, r := range u {
		binary.LittleEndian.PutUint16(b[i*2:], r)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func psWrap(script, begin, end string) string {
	// 2>nul discards PowerShell's progress/CLIXML stderr noise (e.g. "Preparing
	// modules for first use"), which the remote would otherwise merge into the
	// captured output and corrupt the reply.
	return "echo " + begin + "&powershell -NoProfile -EncodedCommand " + psEncode(script) + " 2>nul&echo " + end
}

func listCmd(osName, path, begin, end string) string {
	if osName == "windows" {
		return psWrap("Get-ChildItem -Force -LiteralPath '"+strings.ReplaceAll(path, "'", "''")+
			"' | ForEach-Object { if ($_.PSIsContainer) { 'D/' + $_.Name } else { 'F/' + $_.Name } }", begin, end)
	}
	return "printf '%s\\n' " + begin + "; ls -1Ap -- " + shQuote(path) +
		" 2>&1 | sed -e 's#/$##;t d' -e 's#^#F/#;b' -e ':d s#^#D/#'; printf '%s\\n' " + end
}

type fsEntry struct {
	Name string `json:"name"`
	Dir  bool   `json:"dir"`
}

func parseList(content string) []fsEntry {
	out := []fsEntry{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 2 || line[1] != '/' {
			continue
		}
		switch line[0] {
		case 'D':
			out = append(out, fsEntry{Name: line[2:], Dir: true})
		case 'F':
			out = append(out, fsEntry{Name: line[2:], Dir: false})
		}
	}
	return out
}

// baseName returns the final component of a remote path, accepting either
// separator (a Windows path may use '/', which the filesystem also accepts).
func baseName(path string) string {
	path = strings.TrimRight(path, `/\`)
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		path = path[i+1:]
	}
	if path == "" {
		return "download"
	}
	return path
}

func (s *server) registerFileRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/files/{num}", func(w http.ResponseWriter, r *http.Request) {
		num, err := strconv.ParseUint(r.PathValue("num"), 10, 32)
		if err != nil {
			http.Error(w, "bad friend number", 400)
			return
		}
		osName := s.osOf(uint32(num))
		if osName == "" {
			http.Error(w, "this friend is not a toxnet remote shell (no OS to browse)", 400)
			return
		}
		path := r.URL.Query().Get("path")
		if path == "" {
			if osName == "windows" {
				path = "C:\\"
			} else {
				path = "/"
			}
		}
		_, begin, end := markers()
		content, err := s.runSync(uint32(num), listCmd(osName, path, begin, end), begin, end, 25*time.Second)
		if err != nil {
			http.Error(w, err.Error(), 502)
			return
		}
		writeJSON(w, map[string]any{"path": path, "os": osName, "entries": parseList(content)})
	})

	// Start a background download: query size, then fetch the file in slices.
	mux.HandleFunc("POST /api/download/{num}", func(w http.ResponseWriter, r *http.Request) {
		num64, err := strconv.ParseUint(r.PathValue("num"), 10, 32)
		if err != nil {
			http.Error(w, "bad friend number", 400)
			return
		}
		num := uint32(num64)
		var body struct {
			Path string `json:"path"`
		}
		osName := s.osOf(num)
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Path == "" || osName == "" {
			http.Error(w, "bad request", 400)
			return
		}
		// Both remotes (Go and the Windows asm port) stream the file over the Tox
		// file-transfer protocol; osName only picks the path-separator for the name.
		writeJSON(w, s.startProtocolDownload(num, body.Path).snapshot())
	})

	// Jobs for a friend (progress).
	mux.HandleFunc("/api/download-jobs/{num}", func(w http.ResponseWriter, r *http.Request) {
		num, err := strconv.ParseUint(r.PathValue("num"), 10, 32)
		if err != nil {
			http.Error(w, "bad friend number", 400)
			return
		}
		s.mu.Lock()
		out := []map[string]any{}
		for _, id := range s.jobList {
			if j := s.jobs[id]; j != nil && j.Friend == uint32(num) {
				out = append(out, j.snapshot())
			}
		}
		s.mu.Unlock()
		writeJSON(w, out)
	})

	// Universal downloads store: every download, newest first, with the friend
	// it came from, the remote path, size and time.
	mux.HandleFunc("/api/downloads", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		jobs := make([]*dlJob, 0, len(s.jobList))
		for i := len(s.jobList) - 1; i >= 0; i-- {
			if j := s.jobs[s.jobList[i]]; j != nil {
				jobs = append(jobs, j)
			}
		}
		s.mu.Unlock()
		out := []map[string]any{}
		for _, j := range jobs {
			snap := j.snapshot()
			snap["friendName"] = s.friendLabel(j.Friend)
			out = append(out, snap)
		}
		writeJSON(w, out)
	})

	// Pause, resume or cancel a download.
	mux.HandleFunc("POST /api/download/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		job := s.jobs[r.PathValue("id")]
		s.mu.Unlock()
		if job == nil {
			http.Error(w, "no such download", 404)
			return
		}
		if err := s.controlJob(job, r.PathValue("action")); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, job.snapshot())
	})

	// Serve a finished download.
	mux.HandleFunc("/api/download-file/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		job := s.jobs[r.PathValue("id")]
		s.mu.Unlock()
		if job == nil {
			http.Error(w, "no such download", 404)
			return
		}
		if job.isDoneOK() == false {
			http.Error(w, "download not finished", 409)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(job.Name, `"`, "")+`"`)
		http.ServeFile(w, r, job.file)
	})
}

func (j *dlJob) isDoneOK() bool { j.mu.Lock(); defer j.mu.Unlock(); return j.status == "done" }
