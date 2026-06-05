package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	p := DefaultParams()
	hash, err := HashPassword("hunter2pass", p)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$") {
		t.Fatalf("unexpected hash format: %q", hash)
	}

	ok, err := VerifyPassword("hunter2pass", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}
}

func TestVerifyPasswordWrong(t *testing.T) {
	hash, err := HashPassword("correct horse", DefaultParams())
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := VerifyPassword("battery staple", hash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected mismatch")
	}
}

func TestHashPasswordUniqueSalts(t *testing.T) {
	a, _ := HashPassword("same", DefaultParams())
	b, _ := HashPassword("same", DefaultParams())
	if a == b {
		t.Fatal("expected different hashes due to random salts")
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	cases := []string{
		"",
		"not-a-hash",
		"$argon2id$v=19$m=65536,t=3,p=2$onlyfourparts",
		"$bcrypt$v=19$m=1,t=1,p=1$YWJj$YWJj",
		"$argon2id$v=99$m=65536,t=3,p=2$YWJj$YWJj",
		"$argon2id$v=19$m=65536,t=3,p=2$!!!notbase64$YWJj",
	}
	for _, c := range cases {
		if _, err := VerifyPassword("x", c); err == nil {
			t.Errorf("expected error for malformed hash %q", c)
		}
	}
}
