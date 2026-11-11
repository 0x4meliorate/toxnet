package tox

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// newTestTox builds a Tox with just enough wired up to exercise the
// file-transfer handlers and pump without any network: emit() queues onto evQ,
// which runEvents drains synchronously.
func newTestTox() *Tox {
	return &Tox{evSig: make(chan struct{}, 1)}
}

func (t *Tox) runEvents() {
	t.evMu.Lock()
	q := t.evQ
	t.evQ = nil
	t.evMu.Unlock()
	for _, fn := range q {
		fn()
	}
}

func addFriend(t *Tox) *friend {
	f := &friend{num: uint32(len(t.friends)), online: true}
	t.friends = append(t.friends, f)
	return f
}

func TestFileNumRoundTrip(t *testing.T) {
	for _, slot := range []uint8{0, 1, 42, 255} {
		// sending transfers are numbered by slot directly
		if s, recv := parseFileNum(uint32(slot)); s != slot || recv {
			t.Fatalf("send slot %d: got slot=%d recv=%v", slot, s, recv)
		}
		// receiving transfers are ((slot+1)<<16)
		num := recvFileNum(slot)
		if s, recv := parseFileNum(num); s != slot || !recv {
			t.Fatalf("recv slot %d (num %d): got slot=%d recv=%v", slot, num, s, recv)
		}
	}
}

// TestEncodeSendReqWire pins the SENDREQUEST byte layout to toxcore's, so the
// implementation stays interoperable with real Tox clients.
func TestEncodeSendReqWire(t *testing.T) {
	id := make([]byte, FileIDLength)
	for i := range id {
		id[i] = byte(i)
	}
	pkt := encFileSendReq(7, FileKindData, 0x0102030405060708, id, "hi.txt")

	if pkt[0] != pktFileSendReq {
		t.Fatalf("packet id = %d, want %d", pkt[0], pktFileSendReq)
	}
	if pkt[1] != 7 {
		t.Fatalf("slot = %d, want 7", pkt[1])
	}
	if got := binary.BigEndian.Uint32(pkt[2:6]); got != FileKindData {
		t.Fatalf("kind = %d, want %d", got, FileKindData)
	}
	if got := binary.BigEndian.Uint64(pkt[6:14]); got != 0x0102030405060708 {
		t.Fatalf("size = %#x", got)
	}
	if !bytes.Equal(pkt[14:14+FileIDLength], id) {
		t.Fatalf("file id mismatch")
	}
	if got := string(pkt[14+FileIDLength:]); got != "hi.txt" {
		t.Fatalf("name = %q", got)
	}
}

// TestRecvPipeline drives an incoming transfer through the receive handlers and
// checks the delivered bytes and the empty completion callback.
func TestRecvPipeline(t *testing.T) {
	tx := newTestTox()
	f := addFriend(tx)

	var gotKind uint32
	var gotSize uint64
	var gotName string
	var offered uint32
	tx.OnFileRecv(func(_, num uint32, kind uint32, size uint64, name string) {
		offered, gotKind, gotSize, gotName = num, kind, size, name
	})
	var received []byte
	var completed bool
	tx.OnFileRecvChunk(func(_, _ uint32, _ uint64, data []byte) {
		if len(data) == 0 {
			completed = true
			return
		}
		received = append(received, data...)
	})

	// Offer a 5-byte file in slot 3.
	id := make([]byte, FileIDLength)
	tx.handleFileSendReq(f, encFileSendReq(3, FileKindData, 5, id, "a.bin")[1:])
	tx.runEvents()
	if offered != recvFileNum(3) || gotKind != FileKindData || gotSize != 5 || gotName != "a.bin" {
		t.Fatalf("offer: num=%d kind=%d size=%d name=%q", offered, gotKind, gotSize, gotName)
	}

	// Accept it (receiver side), then feed the data as a single short chunk
	// (a 5-byte file fits in one packet; a short packet is the final one).
	ft := f.fileRecv[3]
	if ft == nil || ft.status != ftNotAccepted {
		t.Fatalf("transfer not registered as not-accepted")
	}
	ft.status = ftTransferring // FileControl(Resume) would do this; it needs a conn to ack

	tx.handleFileData(f, encFileData(3, []byte("hello"))[1:])
	tx.runEvents()

	if string(received) != "hello" {
		t.Fatalf("received %q, want %q", received, "hello")
	}
	if !completed {
		t.Fatalf("completion callback not fired")
	}
	if _, ok := f.fileRecv[3]; ok {
		t.Fatalf("finished transfer not cleared")
	}
}

// TestRecvMultiChunk exercises a file spanning more than one data packet: full
// maxFileData chunks until a single short final chunk ends it.
func TestRecvMultiChunk(t *testing.T) {
	tx := newTestTox()
	f := addFriend(tx)
	var received []byte
	var completed bool
	tx.OnFileRecvChunk(func(_, _ uint32, _ uint64, data []byte) {
		if len(data) == 0 {
			completed = true
			return
		}
		received = append(received, data...)
	})

	size := maxFileData + 3
	full := bytes.Repeat([]byte{'A'}, maxFileData)
	tail := []byte("end")
	id := make([]byte, FileIDLength)
	tx.handleFileSendReq(f, encFileSendReq(0, FileKindData, uint64(size), id, "big")[1:])
	f.fileRecv[0].status = ftTransferring

	tx.handleFileData(f, encFileData(0, full)[1:]) // full chunk: not final
	if _, ok := f.fileRecv[0]; !ok {
		t.Fatalf("transfer ended early after a full chunk")
	}
	tx.handleFileData(f, encFileData(0, tail)[1:]) // short chunk: final
	tx.runEvents()

	if len(received) != size || !bytes.Equal(received[maxFileData:], tail) {
		t.Fatalf("received %d bytes (want %d), tail=%q", len(received), size, received[len(received)-3:])
	}
	if !completed {
		t.Fatalf("completion callback not fired")
	}
	if _, ok := f.fileRecv[0]; ok {
		t.Fatalf("finished transfer not cleared")
	}
}

// TestRecvNeverExceedsSize checks a sender that offers more bytes than declared
// cannot overflow the receiver's view of the file.
func TestRecvNeverExceedsSize(t *testing.T) {
	tx := newTestTox()
	f := addFriend(tx)
	var received []byte
	tx.OnFileRecvChunk(func(_, _ uint32, _ uint64, data []byte) {
		received = append(received, data...)
	})
	id := make([]byte, FileIDLength)
	tx.handleFileSendReq(f, encFileSendReq(0, FileKindData, 3, id, "x")[1:])
	f.fileRecv[0].status = ftTransferring
	tx.handleFileData(f, encFileData(0, []byte("abcdef"))[1:]) // 6 bytes for a 3-byte file
	tx.runEvents()
	if string(received) != "abc" {
		t.Fatalf("received %q, want %q", received, "abc")
	}
}

// TestChunkPumpPacing checks the send pump requests chunks in order and stops
// when the reliable-send window (minus reserve) is exhausted.
func TestChunkPumpPacing(t *testing.T) {
	tx := newTestTox()
	f := addFriend(tx)
	// A congestion window of 100 (above the per-pass burst cap, below the
	// net_crypto window) so this test exercises pacing, not window growth.
	const cwnd = 100
	c := &cryptoConn{f: f, status: csEstablished, cwnd: cwnd, ssthresh: 10000}
	f.conn = c

	type req struct {
		pos uint64
		n   int
	}
	var reqs []req
	tx.OnFileChunkRequest(func(_, _ uint32, pos uint64, n int) {
		reqs = append(reqs, req{pos, n})
	})

	// A large file, accepted and transferring.
	size := uint64(maxFileData) * 1000
	f.fileSend = map[uint8]*fileTransfer{0: {size: size, status: ftTransferring}}
	f.numSending = 1

	tx.fileTick(f, c)
	tx.runEvents()

	// Each pass is capped at fileMaxBurst so a bulk transfer can't flood the
	// relay queue; the window fills over several passes.
	if len(reqs) != fileMaxBurst {
		t.Fatalf("first pass requested %d chunks, want %d (burst cap)", len(reqs), fileMaxBurst)
	}

	// Pump repeatedly without anything reaching the wire (transferred stays 0):
	// the outstanding-gap accounting must stop it at exactly the congestion
	// window, never over-requesting.
	for i := 0; i < 20; i++ {
		tx.fileTick(f, c)
		tx.runEvents()
	}
	wantTotal := cwnd
	if len(reqs) != wantTotal {
		t.Fatalf("after many passes requested %d chunks, want %d (congestion-window cap)", len(reqs), wantTotal)
	}
	// Positions must be sequential full-size chunks starting at 0.
	for i, r := range reqs {
		if r.pos != uint64(i*maxFileData) || r.n != maxFileData {
			t.Fatalf("req %d = {pos:%d n:%d}, want {pos:%d n:%d}", i, r.pos, r.n, i*maxFileData, maxFileData)
		}
	}
	if f.fileSend[0].requested != uint64(wantTotal*maxFileData) {
		t.Fatalf("requested counter = %d", f.fileSend[0].requested)
	}
}

// TestChunkPumpFinish checks the final zero-length request fires once and the
// transfer is cleared.
func TestChunkPumpFinish(t *testing.T) {
	tx := newTestTox()
	f := addFriend(tx)
	c := &cryptoConn{f: f, status: csEstablished}
	f.conn = c

	var zeroReqs int
	tx.OnFileChunkRequest(func(_, _ uint32, _ uint64, n int) {
		if n == 0 {
			zeroReqs++
		}
	})
	f.fileSend = map[uint8]*fileTransfer{0: {size: 10, transferred: 10, status: ftFinished}}
	f.numSending = 1

	tx.fileTick(f, c)
	tx.runEvents()
	tx.fileTick(f, c) // must not fire a second completion
	tx.runEvents()

	if zeroReqs != 1 {
		t.Fatalf("zero-length completion requests = %d, want 1", zeroReqs)
	}
	if _, ok := f.fileSend[0]; ok {
		t.Fatalf("finished transfer not cleared")
	}
	if f.numSending != 0 {
		t.Fatalf("numSending = %d, want 0", f.numSending)
	}
}

func TestPacketsFor(t *testing.T) {
	cases := map[uint64]int{0: 0, 1: 1, uint64(maxFileData): 1, uint64(maxFileData) + 1: 2, uint64(maxFileData) * 3: 3}
	for in, want := range cases {
		if got := packetsFor(in); got != want {
			t.Fatalf("packetsFor(%d) = %d, want %d", in, got, want)
		}
	}
}
