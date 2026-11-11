package tox

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"strings"

	"golang.org/x/crypto/curve25519"
)

// The save file keeps the long-term identity (so the Tox ID is stable across
// restarts) and the friend list. The DHT and onion keys are ephemeral and
// regenerated each run, so they are not stored.
//
// v1: per friend = pk(32), name.
// v2: per friend = pk(32), name, note, lastSeen(int64 unix seconds).
// v3: per friend = pk(32), name, note, localName, lastSeen(int64).
// v4: per friend = pk(32), name, note, localName, addedByUs(1), lastSeen(int64).
// Older files still load; for pre-v4 saves addedByUs is inferred from the
// broadcast name (shells announce "toxnet-windows"/"toxnet-linux").
var saveMagicV1 = [8]byte{'T', 'O', 'X', 'N', 'E', 'T', 'G', '1'}
var saveMagicV2 = [8]byte{'T', 'O', 'X', 'N', 'E', 'T', 'G', '2'}
var saveMagicV3 = [8]byte{'T', 'O', 'X', 'N', 'E', 'T', 'G', '3'}
var saveMagicV4 = [8]byte{'T', 'O', 'X', 'N', 'E', 'T', 'G', '4'}

type savedFriend struct {
	pk        Key
	name      string
	localName string
	note      string
	addedByUs bool
	lastSeen  int64
}

func pubFromSecret(sk Key) Key {
	pk, err := curve25519.X25519(sk[:], curve25519.Basepoint)
	if err != nil {
		panic(err)
	}
	var out Key
	copy(out[:], pk)
	return out
}

// takeStr reads a uint16-length-prefixed string, advancing p.
func takeStr(p []byte) (string, []byte, bool) {
	if len(p) < 2 {
		return "", nil, false
	}
	n := int(binary.LittleEndian.Uint16(p))
	p = p[2:]
	if len(p) < n {
		return "", nil, false
	}
	return string(p[:n]), p[n:], true
}

// load reads the save file (if any) into the identity fields and returns the
// saved friends. With no SavePath, or no file yet, it returns (nil, nil) and
// the caller keeps the freshly generated keys.
func (t *Tox) load() ([]savedFriend, error) {
	if t.opts.SavePath == "" {
		return nil, nil
	}
	data, err := os.ReadFile(t.opts.SavePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	v4 := len(data) >= 8 && bytes.Equal(data[:8], saveMagicV4[:])
	v3 := len(data) >= 8 && bytes.Equal(data[:8], saveMagicV3[:])
	v2 := len(data) >= 8 && bytes.Equal(data[:8], saveMagicV2[:])
	v1 := len(data) >= 8 && bytes.Equal(data[:8], saveMagicV1[:])
	if (!v1 && !v2 && !v3 && !v4) || len(data) < 8+32+32+32+4+4 {
		return nil, errors.New("tox: corrupt save file")
	}
	p := data[8:]
	copy(t.realSK[:], p[0:32])
	copy(t.dhtSK[:], p[32:64])
	copy(t.onionSK[:], p[64:96])
	copy(t.nospam[:], p[96:100])
	t.realPK = pubFromSecret(t.realSK)
	t.dhtPK = pubFromSecret(t.dhtSK)
	t.onionPK = pubFromSecret(t.onionSK)
	p = p[100:]

	n := binary.LittleEndian.Uint32(p)
	p = p[4:]
	var friends []savedFriend
	for i := uint32(0); i < n; i++ {
		if len(p) < 32 {
			return nil, errors.New("tox: corrupt save file")
		}
		var sf savedFriend
		copy(sf.pk[:], p[:32])
		p = p[32:]
		var ok bool
		if sf.name, p, ok = takeStr(p); !ok {
			return nil, errors.New("tox: corrupt save file")
		}
		if v2 || v3 || v4 {
			if sf.note, p, ok = takeStr(p); !ok {
				return nil, errors.New("tox: corrupt save file")
			}
			if v3 || v4 {
				if sf.localName, p, ok = takeStr(p); !ok {
					return nil, errors.New("tox: corrupt save file")
				}
			}
			if v4 {
				if len(p) < 1 {
					return nil, errors.New("tox: corrupt save file")
				}
				sf.addedByUs = p[0] != 0
				p = p[1:]
			}
			if len(p) < 8 {
				return nil, errors.New("tox: corrupt save file")
			}
			sf.lastSeen = int64(binary.LittleEndian.Uint64(p))
			p = p[8:]
		}
		if !v4 {
			// Pre-v4 saves don't record how a friend was added; infer it from the
			// broadcast name so existing friends keep their People/Shells grouping.
			lower := strings.ToLower(sf.name)
			sf.addedByUs = !strings.Contains(lower, "windows") && !strings.Contains(lower, "linux")
		}
		friends = append(friends, sf)
	}
	return friends, nil
}

func putStr(b *bytes.Buffer, s string) {
	if len(s) > 0xffff {
		s = s[:0xffff]
	}
	var nl [2]byte
	binary.LittleEndian.PutUint16(nl[:], uint16(len(s)))
	b.Write(nl[:])
	b.WriteString(s)
}

// save writes the identity and friend list atomically (format v4). It is a
// no-op when no SavePath is set. The caller must hold t.mu.
func (t *Tox) save() error {
	if t.opts.SavePath == "" {
		return nil
	}
	var b bytes.Buffer
	b.Write(saveMagicV4[:])
	b.Write(t.realSK[:])
	b.Write(t.dhtSK[:])
	b.Write(t.onionSK[:])
	b.Write(t.nospam[:])
	var live []*friend
	for _, f := range t.friends {
		if !f.removed {
			live = append(live, f)
		}
	}
	var count [4]byte
	binary.LittleEndian.PutUint32(count[:], uint32(len(live)))
	b.Write(count[:])
	for _, f := range live {
		b.Write(f.realPK[:])
		putStr(&b, f.name)
		putStr(&b, f.note)
		putStr(&b, f.localName)
		var ab byte
		if f.addedByUs {
			ab = 1
		}
		b.WriteByte(ab)
		var ls [8]byte
		var sec int64
		if !f.lastSeen.IsZero() {
			sec = f.lastSeen.Unix()
		}
		binary.LittleEndian.PutUint64(ls[:], uint64(sec))
		b.Write(ls[:])
	}
	tmp := t.opts.SavePath + ".tmp"
	if err := os.WriteFile(tmp, b.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, t.opts.SavePath)
}
