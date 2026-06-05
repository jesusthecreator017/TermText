package client

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jesusthecreator017/TermText/internal/crypto"
)

type keyFile struct {
	Pub  string `json:"pub"`  // base64, not secret
	Priv string `json:"priv"` // base64 of password-encrypted blob
}

func keyDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "termtext", "keys"), nil
}

func keyFilePath(username string) (string, error) {
	dir, err := keyDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, username+".json"), nil
}

func HasKeys(username string) bool {
	path, err := keyFilePath(username)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// SaveKeys persists the public key in the clear and the private key encrypted
// under the user's password.
func SaveKeys(username, password string, pub, priv *[crypto.KeySize]byte) error {
	blob, err := crypto.EncryptPrivateKey(password, priv)
	if err != nil {
		return err
	}
	kf := keyFile{
		Pub:  base64.StdEncoding.EncodeToString(pub[:]),
		Priv: base64.StdEncoding.EncodeToString(blob),
	}
	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return err
	}
	path, err := keyFilePath(username)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func LoadKeys(username, password string) (*[crypto.KeySize]byte, *[crypto.KeySize]byte, error) {
	path, err := keyFilePath(username)
	if err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var kf keyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return nil, nil, err
	}
	pubBytes, err := base64.StdEncoding.DecodeString(kf.Pub)
	if err != nil || len(pubBytes) != crypto.KeySize {
		return nil, nil, crypto.ErrBadKey
	}
	blob, err := base64.StdEncoding.DecodeString(kf.Priv)
	if err != nil {
		return nil, nil, crypto.ErrBadKey
	}
	priv, err := crypto.DecryptPrivateKey(password, blob)
	if err != nil {
		return nil, nil, err
	}
	var pub [crypto.KeySize]byte
	copy(pub[:], pubBytes)
	return &pub, priv, nil
}

// --- TOFU: remember each peer's key fingerprint, warn on change ---

func knownKeysPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "termtext", "known_keys.json"), nil
}

func LoadKnownKeys() map[string]string {
	path, err := knownKeysPath()
	if err != nil {
		return map[string]string{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	m := map[string]string{}
	_ = json.Unmarshal(data, &m)
	return m
}

func SaveKnownKey(userID, fingerprint string) error {
	known := LoadKnownKeys()
	known[userID] = fingerprint
	data, err := json.MarshalIndent(known, "", "  ")
	if err != nil {
		return err
	}
	path, err := knownKeysPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
