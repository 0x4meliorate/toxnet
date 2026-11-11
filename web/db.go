package main

// Persistent activity log in a SQLite database: friends seen, connection
// changes, downloads, and console traffic. Writing is best-effort - a logging
// error is reported but never interrupts the client. The database is a plain
// SQLite file you can open with any sqlite tool.

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, registers the "sqlite" driver name
)

type logStore struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS friends (
	public_key TEXT PRIMARY KEY,
	num        INTEGER,
	first_seen DATETIME,
	removed_at DATETIME
);
CREATE TABLE IF NOT EXISTS connections (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	num        INTEGER,
	public_key TEXT,
	online     INTEGER,
	at         DATETIME
);
CREATE TABLE IF NOT EXISTS downloads (
	id          INTEGER PRIMARY KEY,
	friend_num  INTEGER,
	friend_key  TEXT,
	name        TEXT,
	remote_path TEXT,
	size        INTEGER,
	received    INTEGER,
	status      TEXT,
	started_at  DATETIME,
	finished_at DATETIME
);
CREATE TABLE IF NOT EXISTS console (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	num        INTEGER,
	public_key TEXT,
	direction  TEXT,
	text       TEXT,
	at         DATETIME
);`

// openLog opens (creating if needed) the SQLite activity log at path.
func openLog(path string) (*logStore, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: serialize writers, avoid "database is locked"
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	// Migrate databases created before removed_at existed (ignore if present).
	db.Exec(`ALTER TABLE friends ADD COLUMN removed_at DATETIME`)
	return &logStore{db: db}, nil
}

func (l *logStore) Close() {
	if l != nil && l.db != nil {
		l.db.Close()
	}
}

// exec runs a statement best-effort: a logging failure is reported, not fatal.
func (l *logStore) exec(query string, args ...any) {
	if l == nil || l.db == nil {
		return
	}
	if _, err := l.db.Exec(query, args...); err != nil {
		log.Printf("activity log: %v", err)
	}
}

// ts formats a time as a clean UTC RFC3339 string for storage and querying.
func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// friendSeen records a friend the first time we see them; a re-added friend
// clears any earlier removal time.
func (l *logStore) friendSeen(num uint32, pubkey string) {
	l.exec(`INSERT INTO friends (public_key, num, first_seen) VALUES (?, ?, ?)
		ON CONFLICT(public_key) DO UPDATE SET num = excluded.num, removed_at = NULL`,
		pubkey, num, ts(time.Now()))
}

// friendRemoved records that a friend was removed.
func (l *logStore) friendRemoved(num uint32, pubkey string) {
	l.exec(`INSERT INTO friends (public_key, num, removed_at) VALUES (?, ?, ?)
		ON CONFLICT(public_key) DO UPDATE SET removed_at = excluded.removed_at`,
		pubkey, num, ts(time.Now()))
}

// connection records a friend coming online or going offline.
func (l *logStore) connection(num uint32, pubkey string, online bool) {
	l.exec(`INSERT INTO connections (num, public_key, online, at) VALUES (?, ?, ?, ?)`,
		num, pubkey, b2i(online), ts(time.Now()))
}

// console records one console message to or from a friend.
func (l *logStore) console(num uint32, pubkey, direction, text string) {
	l.exec(`INSERT INTO console (num, public_key, direction, text, at) VALUES (?, ?, ?, ?, ?)`,
		num, pubkey, direction, text, ts(time.Now()))
}

// download upserts a download row by its id, so it reflects the latest state
// (name, size, progress, status, timestamps) as the transfer proceeds.
func (l *logStore) download(idStr string, friendNum uint32, friendKey, name, remotePath string,
	size, received int64, status string, started, finished time.Time) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return
	}
	var fin any
	if !finished.IsZero() {
		fin = ts(finished)
	}
	l.exec(`INSERT INTO downloads
		(id, friend_num, friend_key, name, remote_path, size, received, status, started_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			size = excluded.size, received = excluded.received,
			status = excluded.status, finished_at = excluded.finished_at`,
		id, friendNum, friendKey, name, remotePath, size, received, status, ts(started), fin)
}

// restoredDownload is one completed download read back from the log at startup,
// so the downloads store (Loot) survives restarts.
type restoredDownload struct {
	ID       string
	Friend   uint32
	Name     string
	Path     string
	Size     int64
	Received int64
	Started  time.Time
	Finished time.Time
}

// finishedDownloads returns the completed downloads recorded in the log, oldest
// first, so they can be re-added to the in-memory store after a restart.
func (l *logStore) finishedDownloads() []restoredDownload {
	if l == nil || l.db == nil {
		return nil
	}
	rows, err := l.db.Query(`SELECT id, friend_num, name, remote_path, size, received, started_at, finished_at
		FROM downloads WHERE status = 'done' ORDER BY id ASC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []restoredDownload
	for rows.Next() {
		var d restoredDownload
		var idNum, friend int64
		var started, finished sql.NullString
		if rows.Scan(&idNum, &friend, &d.Name, &d.Path, &d.Size, &d.Received, &started, &finished) != nil {
			continue
		}
		d.ID = strconv.FormatInt(idNum, 10)
		d.Friend = uint32(friend)
		if started.Valid {
			d.Started, _ = time.Parse(time.RFC3339Nano, started.String)
		}
		if finished.Valid {
			d.Finished, _ = time.Parse(time.RFC3339Nano, finished.String)
		}
		out = append(out, d)
	}
	return out
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// logTables whitelists the readable tables and the column each is ordered by
// (newest first); the whitelist also keeps the table name out of SQL injection.
var logTables = map[string]string{
	"friends": "first_seen", "connections": "id", "downloads": "id", "console": "id",
}

// logSelect overrides the column list for a table. The console list sends the
// command text (outgoing messages) but not command output - output stays hidden
// until a row is clicked, which fetches the full text on demand (see consoleRow).
var logSelect = map[string]string{
	"console": "id, num, public_key, direction, " +
		"CASE WHEN direction = 'out' THEN substr(text,1,300) ELSE '' END AS text, " +
		"length(text) AS len, at",
}

// query returns one page of a log table, newest first, as generic maps.
func (l *logStore) query(table string, limit, offset int) ([]map[string]any, error) {
	if l == nil || l.db == nil {
		return []map[string]any{}, nil
	}
	order, ok := logTables[table]
	if !ok {
		return nil, errors.New("unknown log table")
	}
	cols := logSelect[table]
	if cols == "" {
		cols = "*"
	}
	rows, err := l.db.Query("SELECT "+cols+" FROM "+table+" ORDER BY "+order+" DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, c := range names {
			if b, isBytes := vals[i].([]byte); isBytes {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// consoleRow returns the full text and metadata of one console entry.
func (l *logStore) consoleRow(id int64) (map[string]any, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("no activity log")
	}
	var (
		rid, num           int64
		key, dir, text, at string
	)
	err := l.db.QueryRow("SELECT id, num, public_key, direction, text, at FROM console WHERE id = ?", id).
		Scan(&rid, &num, &key, &dir, &text, &at)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": rid, "num": num, "public_key": key, "direction": dir, "text": text, "at": at}, nil
}

// registerLogRoutes serves the activity log tables for the Logs view.
func (s *server) registerLogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/logs/console/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", 400)
			return
		}
		row, err := s.db.consoleRow(id)
		if err != nil {
			http.Error(w, "no such entry", 404)
			return
		}
		writeJSON(w, row)
	})
	mux.HandleFunc("/api/logs/{table}", func(w http.ResponseWriter, r *http.Request) {
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if size <= 0 || size > 500 {
			size = 100
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 0 {
			page = 0
		}
		rows, err := s.db.query(r.PathValue("table"), size, page*size)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		writeJSON(w, rows)
	})
}

// watchFriends logs each friend the first time it appears and every
// online/offline change thereafter.
func (s *server) watchFriends() {
	online := map[string]bool{}
	seen := map[string]bool{}
	for {
		present := map[string]bool{}
		for _, f := range s.tx.Friends() {
			present[f.PublicKey] = true
			if !seen[f.PublicKey] {
				seen[f.PublicKey] = true
				s.db.friendSeen(f.Num, f.PublicKey)
			}
			if f.Online != online[f.PublicKey] {
				online[f.PublicKey] = f.Online
				s.db.connection(f.Num, f.PublicKey, f.Online)
			}
		}
		// A removed friend leaves the list; forget it so a re-add is logged fresh
		// (the removal itself is recorded by the delete handler).
		for key := range seen {
			if !present[key] {
				delete(seen, key)
				delete(online, key)
			}
		}
		time.Sleep(2 * time.Second)
	}
}

// flushDownloads periodically records the current state of every download
// (name, remote path, size, progress, friend and status) in the activity log.
func (s *server) flushDownloads() {
	for {
		time.Sleep(2 * time.Second)
		s.mu.Lock()
		jobs := make([]*dlJob, 0, len(s.jobList))
		for _, id := range s.jobList {
			if j := s.jobs[id]; j != nil {
				jobs = append(jobs, j)
			}
		}
		s.mu.Unlock()
		for _, j := range jobs {
			j.mu.Lock()
			id, num, name, path := j.ID, j.Friend, j.Name, j.Path
			total, received, status := j.total, j.received, j.status
			started, finished := j.started, j.finished
			j.mu.Unlock()
			s.db.download(id, num, s.tx.FriendPublicKey(num), name, path,
				total, received, status, started, finished)
		}
	}
}
