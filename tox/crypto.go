package tox

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"

	"golang.org/x/crypto/nacl/box"
)

// Key is a Curve25519 public key, secret key or precomputed shared key.
type Key = [32]byte

// Nonce is a 24-byte crypto_box nonce.
type Nonce = [24]byte

const macSize = 16

func newKeyPair() (pk, sk Key) {
	p, s, err := box.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return *p, *s
}

func precompute(peerPK, sk *Key) Key {
	var shared Key
	box.Precompute(&shared, peerPK, sk)
	return shared
}

// seal encrypts like toxcore's encrypt_data_symmetric: output is MAC || ciphertext.
func seal(shared *Key, nonce *Nonce, plain []byte) []byte {
	return box.SealAfterPrecomputation(nil, plain, nonce, shared)
}

func open(shared *Key, nonce *Nonce, cipher []byte) ([]byte, bool) {
	return box.OpenAfterPrecomputation(nil, cipher, nonce, shared)
}

func randomNonce() Nonce {
	var n Nonce
	rand.Read(n[:])
	return n
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func randomU64() uint64 {
	return binary.LittleEndian.Uint64(randomBytes(8))
}

// incrementNonce treats the nonce as a big-endian number and adds v to it.
func incrementNonce(n *Nonce, v uint32) {
	var add [24]byte
	binary.BigEndian.PutUint32(add[20:], v)
	carry := uint16(0)
	for i := 23; i >= 0; i-- {
		carry += uint16(n[i]) + uint16(add[i])
		n[i] = byte(carry)
		carry >>= 8
	}
}

// closer reports whether a is closer to target than b in XOR distance.
func closer(target, a, b *Key) bool {
	for i := 0; i < 32; i++ {
		da, db := a[i]^target[i], b[i]^target[i]
		if da != db {
			return da < db
		}
	}
	return false
}

func keyFromHex(s string) (Key, error) {
	var k Key
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != 32 {
		return k, errors.New("tox: invalid public key")
	}
	copy(k[:], b)
	return k, nil
}

func keyHex(k Key) string { return strings.ToUpper(hex.EncodeToString(k[:])) }

// ---- Tox ID (address) ----

func addressChecksum(data []byte) [2]byte {
	var c [2]byte
	for i, b := range data {
		c[i%2] ^= b
	}
	return c
}

func makeAddress(pk Key, nospam [4]byte) string {
	buf := make([]byte, 0, 38)
	buf = append(buf, pk[:]...)
	buf = append(buf, nospam[:]...)
	c := addressChecksum(buf)
	buf = append(buf, c[:]...)
	return strings.ToUpper(hex.EncodeToString(buf))
}

// ParseAddress decodes a 76 hex character Tox ID into its public key and nospam.
func ParseAddress(id string) (Key, [4]byte, error) {
	var pk Key
	var nospam [4]byte
	b, err := hex.DecodeString(strings.TrimSpace(id))
	if err != nil || len(b) != 38 {
		return pk, nospam, errors.New("tox: Tox ID must be 76 hex characters")
	}
	c := addressChecksum(b[:36])
	if !bytes.Equal(c[:], b[36:]) {
		return pk, nospam, errors.New("tox: bad Tox ID checksum")
	}
	copy(pk[:], b[:32])
	copy(nospam[:], b[32:36])
	return pk, nospam, nil
}
