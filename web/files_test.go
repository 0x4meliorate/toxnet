package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaxDownloadID(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"1", "2", "5", "src.bin", "notanid", "07"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := maxDownloadID(dir); got != 7 { // "07" parses to 7, the highest
		t.Fatalf("maxDownloadID = %d, want 7", got)
	}
	if got := maxDownloadID(t.TempDir()); got != 0 {
		t.Fatalf("maxDownloadID(empty) = %d, want 0", got)
	}
}

func TestNextDownloadID(t *testing.T) {
	s := &server{dlSeq: 5}
	for _, want := range []string{"6", "7", "8"} {
		if got := s.nextDownloadID(); got != want {
			t.Fatalf("nextDownloadID = %q, want %q", got, want)
		}
	}
}
