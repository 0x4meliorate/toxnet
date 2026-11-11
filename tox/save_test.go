package tox

import (
	"path/filepath"
	"testing"
)

// TestSaveAddedByUsRoundTrip checks the v4 save format preserves each friend's
// addedByUs flag across save() and load().
func TestSaveAddedByUsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "id.save")

	a := &Tox{}
	a.opts.SavePath = path
	a.friends = []*friend{
		{realPK: Key{1}, name: "toxnet-windows", addedByUs: false},
		{realPK: Key{2}, name: "alice", localName: "Alice", note: "pal", addedByUs: true},
	}
	if err := a.save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	b := &Tox{}
	b.opts.SavePath = path
	sfs, err := b.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := map[string]bool{}
	for _, sf := range sfs {
		got[sf.name] = sf.addedByUs
	}
	if !got["alice"] {
		t.Errorf("alice addedByUs = false, want true (we added them)")
	}
	if got["toxnet-windows"] {
		t.Errorf("toxnet-windows addedByUs = true, want false (auto-accepted shell)")
	}
}

// TestAddFriendPromotesExisting checks that calling AddFriend with the Tox ID of
// a friend who added us (addedByUs=false) promotes them instead of erroring or
// duplicating: same friend number, now addedByUs=true, no extra friend.
func TestAddFriendPromotesExisting(t *testing.T) {
	tx := &Tox{evSig: make(chan struct{}, 1)}
	// Simulate an auto-accepted friend (as addFriendLocked would create).
	f, err := tx.addFriendLocked(Key{7})
	if err != nil {
		t.Fatalf("addFriendLocked: %v", err)
	}
	if f.addedByUs {
		t.Fatal("new auto-accepted friend should have addedByUs=false")
	}
	id := makeAddress(f.realPK, [4]byte{1, 2, 3, 4})

	num, err := tx.AddFriend(id, "")
	if err != nil {
		t.Fatalf("AddFriend on existing friend: %v", err)
	}
	if num != f.num {
		t.Errorf("AddFriend returned num %d, want existing %d", num, f.num)
	}
	if len(tx.friends) != 1 {
		t.Errorf("friend count = %d, want 1 (no duplicate)", len(tx.friends))
	}
	if !f.addedByUs {
		t.Errorf("friend addedByUs = false after AddFriend, want true (promoted to People)")
	}
	if f.sendRequest {
		t.Errorf("sendRequest = true, want false (promotion must not send a request)")
	}
}
