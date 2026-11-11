package tox

// File transfers, faithful to c-toxcore's Messenger file-transfer protocol so
// they interoperate on the wire with real Tox clients.
//
// Three lossless packet types ride net_crypto:
//
//	FILE_SENDREQUEST (80)  [slot:1][kind:4 BE][size:8 BE][file_id:32][name:0..255]
//	FILE_CONTROL     (81)  [dir:1][slot:1][type:1][payload...]   type: resume/pause/cancel/seek
//	FILE_DATA        (82)  [slot:1][bytes:0..maxFileData]        position is implicit
//
// Transfers are pull-based: the sender does not push the file. Instead the
// library asks the sending application for the next chunk (OnFileChunkRequest)
// only while the reliable-send window has room, so a transfer fills the link
// without starving messages or other transfers. The receiving application
// accepts an offer with FileControl(..., FileControlResume) and then receives
// the bytes through OnFileRecvChunk. A final OnFileRecvChunk with empty data
// signals the file is complete.
//
// File numbers follow toxcore: a transfer we send is numbered by its slot
// (0..255); a transfer we receive is numbered ((slot+1) << 16). This lets both
// peers refer to the same transfer by the sender-assigned slot.

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
)

// Public file-transfer constants.
const (
	// FileKindData is an ordinary file. FileKindAvatar is a friend avatar (the
	// file_id is the avatar hash so a client can skip an unchanged avatar).
	FileKindData   uint32 = 0
	FileKindAvatar uint32 = 1

	// FileControl values for FileControl. Resume both accepts a new incoming
	// transfer and un-pauses a paused one.
	FileControlResume = 0
	FileControlPause  = 1
	FileControlCancel = 2

	// FileIDLength is the length of a file_id in bytes.
	FileIDLength = 32

	// MaxFilenameLen is the maximum filename length toxcore accepts.
	MaxFilenameLen = 255
)

// maxFileData is the most file bytes that fit in one net_crypto data packet
// alongside the 2-byte file-data header ([packet id][slot]).
const maxFileData = maxCryptoData - 2 // 1371

// fileReserve keeps this many send-window slots free for messages, acks and
// keep-alives so a bulk transfer never blocks control traffic.
const fileReserve = 32

// fileMaxBurst caps how many chunks one pump pass queues. The pump is also
// ack-clocked (run from handleData as acknowledgements arrive), so bounding each
// pass keeps a bulk transfer from flooding the relay's send queue in one go -
// which would drop packets, including keep-alives, and reset the connection.
const fileMaxBurst = 48

// internal control types on the wire (match toxcore Filecontrol).
const (
	fcAccept = 0
	fcPause  = 1
	fcKill   = 2
	fcSeek   = 3
)

// transfer status.
const (
	ftNotAccepted = iota // offer sent/received, not yet accepted
	ftTransferring
	ftFinished // all bytes handed off; awaiting cleanup
)

// paused bitmask.
const (
	pauseUs    = 1 << 0
	pauseOther = 1 << 1
)

// Errors returned by the file-transfer API.
var (
	ErrFriendNotFound = errors.New("tox: no such friend")
	ErrFileNotOnline  = errors.New("tox: friend is not online")
	ErrFileNoSlots    = errors.New("tox: no free file-transfer slots")
	ErrFileName       = errors.New("tox: filename too long")
	ErrFileNotFound   = errors.New("tox: no such file transfer")
	ErrFileBadControl = errors.New("tox: invalid file control for this transfer")
	ErrFileBadChunk   = errors.New("tox: chunk does not match the requested position/size")
)

type fileTransfer struct {
	size        uint64 // total bytes, or streamUnknown for a stream of unknown length
	transferred uint64 // sender: bytes handed to the wire; receiver: bytes delivered
	requested   uint64 // sender only: bytes already asked of the application
	kind        uint32
	name        string
	id          [FileIDLength]byte
	status      int
	paused      int  // pauseUs | pauseOther
	finalSent   bool // sender: the final zero-length chunk request was emitted
}

// streamUnknown is toxcore's UINT64_MAX sentinel for a stream whose length is
// not known in advance; such a transfer ends when a short/empty chunk is sent.
const streamUnknown = ^uint64(0)

func recvFileNum(slot uint8) uint32 { return (uint32(slot) + 1) << 16 }

// parseFileNum splits a public file number into a slot and whether it names an
// incoming (receiving) transfer.
func parseFileNum(n uint32) (slot uint8, receiving bool) {
	if n >= 1<<16 {
		return uint8((n >> 16) - 1), true
	}
	return uint8(n), false
}

// ---- callback setters ----

// OnFileRecv is called when a friend offers us a file. Accept it with
// FileControl(friend, fileNum, FileControlResume) (optionally after FileSeek to
// resume), or reject it with FileControlCancel. fileNum identifies the transfer
// in later calls and callbacks.
func (t *Tox) OnFileRecv(fn func(friend, fileNum uint32, kind uint32, size uint64, name string)) {
	t.mu.Lock()
	t.cbFileRecv = fn
	t.mu.Unlock()
}

// OnFileRecvChunk is called with each piece of an incoming file, in order,
// starting at byte position. A final call with empty data means the file is
// complete. Only called for transfers you accepted.
func (t *Tox) OnFileRecvChunk(fn func(friend, fileNum uint32, position uint64, data []byte)) {
	t.mu.Lock()
	t.cbFileRecvChunk = fn
	t.mu.Unlock()
}

// OnFileChunkRequest is called when the library needs the next chunk of a file
// you are sending: respond by calling FileSendChunk(friend, fileNum, position,
// data) with exactly length bytes read at position. A request with length 0
// means the transfer is complete and you should release the file. Requests are
// paced to the connection, so there is always room to send the chunk.
func (t *Tox) OnFileChunkRequest(fn func(friend, fileNum uint32, position uint64, length int)) {
	t.mu.Lock()
	t.cbFileChunkReq = fn
	t.mu.Unlock()
}

// OnFileControl is called when a friend pauses (FileControlPause), resumes
// (FileControlResume) or cancels (FileControlCancel) a transfer.
func (t *Tox) OnFileControl(fn func(friend, fileNum uint32, control int)) {
	t.mu.Lock()
	t.cbFileControl = fn
	t.mu.Unlock()
}

// ---- outgoing API ----

// FileSend offers a file to a friend. kind is FileKindData or FileKindAvatar;
// size is the total number of bytes (use a streaming transfer by passing
// ^uint64(0) if unknown). fileID, if non-nil, must be FileIDLength bytes and
// uniquely identifies the file so a transfer can be resumed later; if nil a
// random id is generated. It returns the file number used to drive the transfer
// (via OnFileChunkRequest / FileSendChunk).
//
// The transfer does not start until the friend accepts it; you will then begin
// receiving OnFileChunkRequest callbacks.
func (t *Tox) FileSend(friend uint32, kind uint32, size uint64, fileID []byte, filename string) (uint32, error) {
	if len(filename) > MaxFilenameLen {
		return 0, ErrFileName
	}
	if fileID != nil && len(fileID) != FileIDLength {
		return 0, errors.New("tox: file id must be 32 bytes")
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	f := t.friendAt(friend)
	if f == nil {
		return 0, ErrFriendNotFound
	}
	if !f.online || f.conn == nil {
		return 0, ErrFileNotOnline
	}
	if f.fileSend == nil {
		f.fileSend = map[uint8]*fileTransfer{}
	}
	slot, ok := freeSlot(f.fileSend)
	if !ok {
		return 0, ErrFileNoSlots
	}

	ft := &fileTransfer{size: size, kind: kind, name: filename, status: ftNotAccepted}
	if fileID != nil {
		copy(ft.id[:], fileID)
	} else {
		rand.Read(ft.id[:])
	}
	f.fileSend[slot] = ft
	t.sendLossless(f.conn, encFileSendReq(slot, kind, size, ft.id[:], filename))
	return uint32(slot), nil
}

// FileSendChunk supplies the bytes the library asked for in an
// OnFileChunkRequest callback. position must equal the requested position and
// len(data) the requested length (a shorter final write ends a streaming
// transfer). Call it with empty data to answer a zero-length (completion)
// request.
func (t *Tox) FileSendChunk(friend, fileNum uint32, position uint64, data []byte) error {
	if len(data) > maxFileData {
		return ErrFileBadChunk
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	f := t.friendAt(friend)
	if f == nil {
		return ErrFriendNotFound
	}
	if !f.online || f.conn == nil {
		return ErrFileNotOnline
	}
	slot, receiving := parseFileNum(fileNum)
	if receiving {
		return ErrFileNotFound
	}
	ft := f.fileSend[slot]
	if ft == nil || ft.status != ftTransferring {
		return ErrFileNotFound
	}
	if position != ft.transferred {
		return ErrFileBadChunk
	}
	if ft.size != streamUnknown && ft.transferred+uint64(len(data)) > ft.size {
		return ErrFileBadChunk
	}

	t.sendLossless(f.conn, encFileData(slot, data))

	ft.transferred += uint64(len(data))
	// A short chunk (or reaching the known size) is the last one.
	if len(data) < maxFileData || ft.transferred == ft.size {
		if ft.status == ftTransferring {
			ft.status = ftFinished
		}
	}
	return nil
}

// FileControl accepts, pauses, resumes or cancels a transfer. Use
// FileControlResume to accept an incoming offer (OnFileRecv) or to un-pause,
// FileControlPause to pause, and FileControlCancel to abort in either
// direction.
func (t *Tox) FileControl(friend, fileNum uint32, control int) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	f := t.friendAt(friend)
	if f == nil {
		return ErrFriendNotFound
	}
	if !f.online || f.conn == nil {
		return ErrFileNotOnline
	}
	slot, receiving := parseFileNum(fileNum)
	ft := transferAt(f, slot, receiving)
	if ft == nil {
		return ErrFileNotFound
	}

	var ctrl int
	switch control {
	case FileControlResume:
		ctrl = fcAccept
		if ft.status == ftNotAccepted {
			// Accepting a fresh transfer. Only the receiver accepts; a sender's
			// transfer becomes transferring when the peer accepts it.
			if !receiving {
				return ErrFileBadControl
			}
			ft.status = ftTransferring
		} else {
			// Un-pausing: only valid if we were the one who paused.
			if ft.paused&pauseUs == 0 {
				return ErrFileBadControl
			}
			ft.paused &^= pauseUs
		}
	case FileControlPause:
		if ft.status != ftTransferring || ft.paused&pauseUs != 0 {
			return ErrFileBadControl
		}
		ctrl = fcPause
		ft.paused |= pauseUs
	case FileControlCancel:
		ctrl = fcKill
	default:
		return ErrFileBadControl
	}

	t.sendFileControl(f, receiving, slot, ctrl, nil)

	if ctrl == fcKill {
		clearTransfer(f, slot, receiving)
	}
	return nil
}

// FileSeek asks, before accepting an incoming transfer, that the sender begin
// at the given byte position instead of 0. Use it to resume a partial download:
// call FileSeek, then FileControl(FileControlResume). Only valid on an incoming
// transfer that has not been accepted yet.
func (t *Tox) FileSeek(friend, fileNum uint32, position uint64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	f := t.friendAt(friend)
	if f == nil {
		return ErrFriendNotFound
	}
	if !f.online || f.conn == nil {
		return ErrFileNotOnline
	}
	slot, receiving := parseFileNum(fileNum)
	if !receiving {
		return ErrFileBadControl
	}
	ft := f.fileRecv[slot]
	if ft == nil {
		return ErrFileNotFound
	}
	if ft.status != ftNotAccepted || position >= ft.size {
		return ErrFileBadControl
	}
	var pos [8]byte
	binary.BigEndian.PutUint64(pos[:], position)
	t.sendFileControl(f, receiving, slot, fcSeek, pos[:])
	ft.transferred = position
	return nil
}

// FileID returns the 32-byte file identifier of a transfer, for resuming.
func (t *Tox) FileID(friend, fileNum uint32) ([]byte, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	f := t.friendAt(friend)
	if f == nil {
		return nil, ErrFriendNotFound
	}
	slot, receiving := parseFileNum(fileNum)
	ft := transferAt(f, slot, receiving)
	if ft == nil {
		return nil, ErrFileNotFound
	}
	out := make([]byte, FileIDLength)
	copy(out, ft.id[:])
	return out, nil
}

// sendFileControl emits a CONTROL packet. dir is 1 when the transfer is inbound
// from the sender of this packet's point of view, so the peer resolves it to
// the matching transfer on their side.
func (t *Tox) sendFileControl(f *friend, receiving bool, slot uint8, ctrl int, payload []byte) {
	t.sendLossless(f.conn, encFileControl(receiving, slot, ctrl, payload))
}

// ---- pure wire encoders (packet id first, as sendLossless expects) ----

func encFileSendReq(slot uint8, kind uint32, size uint64, id []byte, name string) []byte {
	pkt := make([]byte, 0, 2+4+8+FileIDLength+len(name))
	pkt = append(pkt, pktFileSendReq, slot)
	pkt = binary.BigEndian.AppendUint32(pkt, kind)
	pkt = binary.BigEndian.AppendUint64(pkt, size)
	pkt = append(pkt, id...)
	pkt = append(pkt, name...)
	return pkt
}

func encFileData(slot uint8, data []byte) []byte {
	pkt := make([]byte, 0, 2+len(data))
	pkt = append(pkt, pktFileData, slot)
	pkt = append(pkt, data...)
	return pkt
}

// encFileControl builds a CONTROL packet. dir is 1 when the transfer is inbound
// from this packet sender's point of view, so the peer resolves it to the
// matching transfer on their side.
func encFileControl(receiving bool, slot uint8, ctrl int, payload []byte) []byte {
	dir := byte(0)
	if receiving {
		dir = 1
	}
	pkt := make([]byte, 0, 4+len(payload))
	pkt = append(pkt, pktFileControl, dir, slot, byte(ctrl))
	pkt = append(pkt, payload...)
	return pkt
}

// ---- inbound handlers (run under t.mu) ----

func (t *Tox) handleFileSendReq(f *friend, d []byte) {
	const head = 1 + 4 + 8 + FileIDLength // slot + kind + size + id
	if len(d) < head {
		return
	}
	slot := d[0]
	nameLen := len(d) - head
	if nameLen > MaxFilenameLen {
		return
	}
	if f.fileRecv == nil {
		f.fileRecv = map[uint8]*fileTransfer{}
	}
	if f.fileRecv[slot] != nil {
		return // slot already in use; ignore duplicate offer
	}
	kind := binary.BigEndian.Uint32(d[1:5])
	size := binary.BigEndian.Uint64(d[5:13])
	ft := &fileTransfer{size: size, kind: kind, name: string(d[head:]), status: ftNotAccepted}
	copy(ft.id[:], d[13:13+FileIDLength])
	f.fileRecv[slot] = ft

	num := recvFileNum(slot)
	name := ft.name
	n := f.num
	if cb := t.cbFileRecv; cb != nil {
		t.emit(func() { cb(n, num, kind, size, name) })
	}
}

func (t *Tox) handleFileControl(f *friend, d []byte) {
	if len(d) < 3 {
		return
	}
	// The sender's "inbound" is our "outbound": dir==1 means the packet refers
	// to a transfer we are sending.
	outbound := d[0] == 1
	slot := d[1]
	ctrl := d[2]
	payload := d[3:]

	ft := transferAt(f, slot, !outbound)
	if ft == nil {
		// Unknown transfer: tell the peer to drop it too (unless it's a kill).
		if ctrl != fcKill {
			t.sendFileControl(f, !outbound, slot, fcKill, nil)
		}
		return
	}

	switch ctrl {
	case fcAccept:
		if outbound && ft.status == ftNotAccepted {
			// The peer accepted a file we offered: start pumping chunks.
			ft.status = ftTransferring
			t.numSendingInc(f)
		} else if ft.paused&pauseOther != 0 {
			ft.paused &^= pauseOther
		} else {
			return // resume of a transfer that wasn't paused by them
		}
		t.emitFileControl(f, slot, outbound, FileControlResume)
	case fcPause:
		if ft.status != ftTransferring || ft.paused&pauseOther != 0 {
			return
		}
		ft.paused |= pauseOther
		t.emitFileControl(f, slot, outbound, FileControlPause)
	case fcKill:
		t.emitFileControl(f, slot, outbound, FileControlCancel)
		clearTransfer(f, slot, !outbound)
	case fcSeek:
		// Only the sender honours a seek, before the transfer is accepted.
		if !outbound || ft.status != ftNotAccepted || len(payload) != 8 {
			return
		}
		pos := binary.BigEndian.Uint64(payload)
		if ft.size != streamUnknown && pos >= ft.size {
			return
		}
		ft.requested = pos
		ft.transferred = pos
	}
}

func (t *Tox) handleFileData(f *friend, d []byte) {
	if len(d) < 1 {
		return
	}
	slot := d[0]
	ft := f.fileRecv[slot]
	if ft == nil || ft.status != ftTransferring {
		return
	}
	data := d[1:]
	// Never deliver more than the declared size.
	if ft.size != streamUnknown && ft.transferred+uint64(len(data)) > ft.size {
		data = data[:ft.size-ft.transferred]
	}
	num := recvFileNum(slot)
	n := f.num
	pos := ft.transferred

	if len(data) > 0 {
		cp := append([]byte(nil), data...)
		if cb := t.cbFileRecvChunk; cb != nil {
			t.emit(func() { cb(n, num, pos, cp) })
		}
		ft.transferred += uint64(len(data))
	}

	// End of transfer: a zero-length packet, a short packet, or reaching size.
	done := len(data) == 0 ||
		len(d)-1 != maxFileData ||
		(ft.size != streamUnknown && ft.transferred >= ft.size)
	if done {
		end := ft.transferred
		if cb := t.cbFileRecvChunk; cb != nil {
			t.emit(func() { cb(n, num, end, nil) })
		}
		delete(f.fileRecv, slot)
	}
}

// ---- send pump (run under t.mu from cryptoTick) ----

// fileTick requests chunks for the friend's active outgoing transfers while the
// reliable-send window has room, mirroring toxcore's do_reqchunk_filecb. The
// application fulfils each request via FileSendChunk.
func (t *Tox) fileTick(f *friend, c *cryptoConn) {
	if f.numSending == 0 {
		return
	}
	// Free window slots, less a reserve for control traffic, less the chunks
	// already requested but not yet handed to the wire (the async gap between an
	// OnFileChunkRequest and the matching FileSendChunk).
	inflight := int(c.sendEnd - c.sendStart)
	// Pace to the congestion window, which adapts to what the link can carry
	// (see congestion.go), so a bulk transfer can't overrun the relay.
	budget := c.fileLimit() - inflight
	for _, ft := range f.fileSend {
		if ft.status == ftTransferring {
			budget -= packetsFor(ft.requested - ft.transferred)
		}
	}
	if budget <= 0 {
		// Still finish off completed transfers below, which cost no window.
		budget = 0
	}
	if budget > fileMaxBurst {
		budget = fileMaxBurst // pace: never flood the relay queue in one pass
	}

	for slot, ft := range f.fileSend {
		switch {
		case ft.status == ftFinished && !ft.finalSent:
			// Tell the application the transfer is complete (0-length request).
			ft.finalSent = true
			num := uint32(slot)
			pos := ft.transferred
			n := f.num
			if cb := t.cbFileChunkReq; cb != nil {
				t.emit(func() { cb(n, num, pos, 0) })
			}
			clearTransfer(f, slot, false)
		case ft.status == ftTransferring && ft.paused == 0:
			for budget > 0 {
				if ft.size != streamUnknown && ft.requested >= ft.size {
					break // everything has been requested; waiting on the wire
				}
				length := maxFileData
				if ft.size != streamUnknown {
					if rem := ft.size - ft.requested; rem < uint64(length) {
						length = int(rem)
					}
				}
				pos := ft.requested
				ft.requested += uint64(length)
				num := uint32(slot)
				n := f.num
				if cb := t.cbFileChunkReq; cb != nil {
					t.emit(func() { cb(n, num, pos, length) })
				}
				budget--
				if length < maxFileData {
					break // a short (final) chunk; nothing more to request
				}
			}
		}
	}
}

// ---- small helpers ----

// freeSlot returns the lowest unused slot in m, or ok=false if all 256 are used.
func freeSlot(m map[uint8]*fileTransfer) (uint8, bool) {
	for i := range 256 {
		if _, used := m[uint8(i)]; !used {
			return uint8(i), true
		}
	}
	return 0, false
}

func transferAt(f *friend, slot uint8, receiving bool) *fileTransfer {
	if receiving {
		return f.fileRecv[slot]
	}
	return f.fileSend[slot]
}

// clearTransfer removes a transfer and keeps numSending accurate.
func clearTransfer(f *friend, slot uint8, receiving bool) {
	if receiving {
		delete(f.fileRecv, slot)
		return
	}
	if ft := f.fileSend[slot]; ft != nil {
		if ft.status == ftTransferring || ft.status == ftFinished {
			if f.numSending > 0 {
				f.numSending--
			}
		}
		delete(f.fileSend, slot)
	}
}

func (t *Tox) numSendingInc(f *friend) { f.numSending++ }

func (t *Tox) emitFileControl(f *friend, slot uint8, outbound bool, control int) {
	var num uint32
	if outbound {
		num = uint32(slot)
	} else {
		num = recvFileNum(slot)
	}
	n := f.num
	if cb := t.cbFileControl; cb != nil {
		t.emit(func() { cb(n, num, control) })
	}
}

// packetsFor returns how many data packets n bytes span.
func packetsFor(n uint64) int {
	if n == 0 {
		return 0
	}
	return int((n + maxFileData - 1) / maxFileData)
}
