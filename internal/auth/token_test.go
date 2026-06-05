package auth

import (
	"testing"
	"time"
)

const testKey = "7f1c6cddfa31603d6b6809e286fc2de30ff50bcd3152cb2fc1d4220317ad56e5"

func TestTokenRoundTrip(t *testing.T) {
	m, err := NewTokenMaker(testKey, time.Hour)
	if err != nil {
		t.Fatalf("NewTokenMaker: %v", err)
	}
	tok, err := m.Issue("user-123")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Fatalf("UserID = %q, want user-123", claims.UserID)
	}
}

func TestNewTokenMakerBadKey(t *testing.T) {
	if _, err := NewTokenMaker("xyz", time.Hour); err == nil {
		t.Fatal("expected error for non-hex key")
	}
	if _, err := NewTokenMaker("abcd", time.Hour); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestVerifyExpired(t *testing.T) {
	m, err := NewTokenMaker(testKey, -time.Minute)
	if err != nil {
		t.Fatalf("NewTokenMaker: %v", err)
	}
	// negative TTL is clamped to 24h, so issue manually with a past expiry.
	m.duration = -time.Minute
	tok, _ := m.Issue("user-123")
	if _, err := m.Verify(tok); err == nil {
		t.Fatal("expected expired token to fail")
	}
}

func TestVerifyTampered(t *testing.T) {
	m, _ := NewTokenMaker(testKey, time.Hour)
	tok, _ := m.Issue("user-123")
	if _, err := m.Verify(tok + "x"); err == nil {
		t.Fatal("expected tampered token to fail")
	}
}

func TestVerifyWrongKey(t *testing.T) {
	m1, _ := NewTokenMaker(testKey, time.Hour)
	m2, _ := NewTokenMaker("00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff", time.Hour)
	tok, _ := m1.Issue("user-123")
	if _, err := m2.Verify(tok); err == nil {
		t.Fatal("expected token signed with other key to fail")
	}
}
