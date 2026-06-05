package crypto

import "testing"

func TestSealOpenRoundTrip(t *testing.T) {
	alicePub, alicePriv, _ := GenerateKeypair()
	bobPub, bobPriv, _ := GenerateKeypair()

	body, err := Seal("hello bob", bobPub, alicePriv)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if !IsEncrypted(body) {
		t.Fatal("expected body to be detected as encrypted")
	}
	got, ok := Open(body, alicePub, bobPriv)
	if !ok || got != "hello bob" {
		t.Fatalf("Open = %q, ok=%v", got, ok)
	}
}

func TestOpenRejectsTamperAndWrongKey(t *testing.T) {
	aPub, aPriv, _ := GenerateKeypair()
	bPub, bPriv, _ := GenerateKeypair()
	_, ePriv, _ := GenerateKeypair()

	body, _ := Seal("secret", bPub, aPriv)
	if _, ok := Open(body, aPub, ePriv); ok {
		t.Fatal("wrong recipient key should fail")
	}
	if _, ok := Open(body+"x", aPub, bPriv); ok {
		t.Fatal("tampered body should fail")
	}
}

func TestPrivateKeyEncryption(t *testing.T) {
	_, priv, _ := GenerateKeypair()
	blob, err := EncryptPrivateKey("hunter2pass", priv)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	got, err := DecryptPrivateKey("hunter2pass", blob)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if *got != *priv {
		t.Fatal("round-trip mismatch")
	}
	if _, err := DecryptPrivateKey("wrongpass", blob); err == nil {
		t.Fatal("wrong password should fail")
	}
}

func TestIsEncryptedRejectsPlaintext(t *testing.T) {
	if IsEncrypted("just a normal message") {
		t.Fatal("plaintext should not be detected as encrypted")
	}
}
