// Package crypto provides the E2EE primitives for DMs: Curve25519 key pairs,
// NaCl box message sealing, and password-encrypted private-key storage.
//
// The private-key-at-rest KDF is deliberately a SEPARATE argon2id call from the
// authentication password hash — the same password is used, but never the same
// derived bytes.
package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

const KeySize = 32

// GenerateKeypair returns a fresh Curve25519 (public, private) pair.
func GenerateKeypair() (*[KeySize]byte, *[KeySize]byte, error) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return pub, priv, nil
}

type sealed struct {
	N string `json:"n"` // base64 nonce
	C string `json:"c"` // base64 ciphertext
}

// Seal encrypts plaintext for peerPub using myPriv, returning a self-describing
// JSON envelope safe to store as a message body.
func Seal(plaintext string, peerPub, myPriv *[KeySize]byte) (string, error) {
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", err
	}
	ct := box.Seal(nil, []byte(plaintext), &nonce, peerPub, myPriv)
	raw, err := json.Marshal(sealed{
		N: base64.StdEncoding.EncodeToString(nonce[:]),
		C: base64.StdEncoding.EncodeToString(ct),
	})
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// Open decrypts a Seal envelope. ok is false on any tamper/parse/auth failure.
func Open(body string, peerPub, myPriv *[KeySize]byte) (string, bool) {
	var s sealed
	if err := json.Unmarshal([]byte(body), &s); err != nil || s.N == "" || s.C == "" {
		return "", false
	}
	nonceBytes, err := base64.StdEncoding.DecodeString(s.N)
	if err != nil || len(nonceBytes) != 24 {
		return "", false
	}
	ct, err := base64.StdEncoding.DecodeString(s.C)
	if err != nil {
		return "", false
	}
	var nonce [24]byte
	copy(nonce[:], nonceBytes)
	out, ok := box.Open(nil, ct, &nonce, peerPub, myPriv)
	if !ok {
		return "", false
	}
	return string(out), true
}

// IsEncrypted reports whether body is a Seal envelope (vs legacy plaintext).
func IsEncrypted(body string) bool {
	var s sealed
	if err := json.Unmarshal([]byte(body), &s); err != nil {
		return false
	}
	return s.N != "" && s.C != ""
}

// Fingerprint is a short, stable identifier of a public key for TOFU display.
func Fingerprint(pub *[KeySize]byte) string {
	sum := sha256.Sum256(pub[:])
	return fmt.Sprintf("%x", sum[:8])
}

// privKDF derives the at-rest symmetric key. SEPARATE from the auth hash.
func privKDF(password string, salt []byte) [KeySize]byte {
	key := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, KeySize)
	var k [KeySize]byte
	copy(k[:], key)
	return k
}

// EncryptPrivateKey returns salt(16) || nonce(24) || secretbox(priv).
func EncryptPrivateKey(password string, priv *[KeySize]byte) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	var nonce [24]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, err
	}
	key := privKDF(password, salt)
	ct := secretbox.Seal(nil, priv[:], &nonce, &key)

	out := make([]byte, 0, len(salt)+len(nonce)+len(ct))
	out = append(out, salt...)
	out = append(out, nonce[:]...)
	out = append(out, ct...)
	return out, nil
}

var ErrBadKey = errors.New("crypto: wrong password or corrupt key file")

func DecryptPrivateKey(password string, blob []byte) (*[KeySize]byte, error) {
	if len(blob) < 16+24+secretbox.Overhead+KeySize {
		return nil, ErrBadKey
	}
	salt := blob[:16]
	var nonce [24]byte
	copy(nonce[:], blob[16:40])
	ct := blob[40:]

	key := privKDF(password, salt)
	out, ok := secretbox.Open(nil, ct, &nonce, &key)
	if !ok || len(out) != KeySize {
		return nil, ErrBadKey
	}
	var priv [KeySize]byte
	copy(priv[:], out)
	return &priv, nil
}
