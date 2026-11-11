package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestRestoreDownloads checks that a completed download recorded in the log, with
// its file still on disk, is re-listed in the in-memory store after a restart.
func TestRestoreDownloads(t *testing.T) {
	dir := t.TempDir()
	db, err := openLog(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("openLog: %v", err)
	}
	defer db.Close()

	// A completed download logged earlier, plus its file saved under the id.
	db.download("1", 0, "KEY", "report.txt", "/remote/report.txt", 100, 100, "done", time.Now(), time.Now())
	db.download("2", 0, "KEY", "half.bin", "/remote/half.bin", 100, 40, "error", time.Now(), time.Now())
	dldir := filepath.Join(dir, "downloads")
	if err := os.MkdirAll(dldir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dldir, "1"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &server{db: db, dldir: dldir, jobs: map[string]*dlJob{}}
	s.restoreDownloads()

	if len(s.jobList) != 1 || s.jobs["1"] == nil {
		t.Fatalf("expected the one finished download restored, got %d jobs", len(s.jobList))
	}
	if s.jobs["2"] != nil {
		t.Errorf("non-done download should not be restored")
	}
	j := s.jobs["1"]
	if j.Name != "report.txt" || j.status != "done" || j.total != 100 || j.received != 100 {
		t.Errorf("restored job fields wrong: %+v", j)
	}
	if j.file != filepath.Join(dldir, "1") {
		t.Errorf("restored job file = %q, want the on-disk path", j.file)
	}
}
