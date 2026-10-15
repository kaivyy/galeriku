package security_test

import (
	"strings"
	"testing"

	"github.com/galeriku/core/internal/security"
)

func TestHashPassword_Success(t *testing.T) {
	password := "SecretPass123!"

	hash, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error hashing password: %v", err)
	}

	// Verify format matches $argon2id$v=19$m=65536,t=3,p=4$...
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Errorf("expected hash to start with argon2id prefix, got: %s", hash)
	}

	parts := strings.Split(hash, "$")
	// Format: empty, argon2id, v=19, m=65536,t=3,p=4, salt, key
	if len(parts) != 6 {
		t.Fatalf("expected 6 parts in encoded hash, got %d (parts: %v)", len(parts), parts)
	}

	// Verify that hashing same password again generates a different salt and hash
	hash2, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error hashing second time: %v", err)
	}
	if hash == hash2 {
		t.Error("expected different salts for identical passwords, got identical hashes")
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	_, err := security.HashPassword("")
	if err == nil {
		t.Fatal("expected error for empty password, got nil")
	}
	if err != security.ErrEmptyPassword {
		t.Errorf("expected ErrEmptyPassword, got: %v", err)
	}
}

func TestVerifyPassword_MatchAndMismatch(t *testing.T) {
	password := "MyP@ssw0rd!"
	wrongPassword := "WrongP@ssw0rd!"

	hash, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("failed to hash password: %v", err)
	}

	// Correct password must match
	match, err := security.VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("unexpected error verifying password: %v", err)
	}
	if !match {
		t.Error("expected correct password to match, got false")
	}

	// Wrong password must not match
	match, err = security.VerifyPassword(wrongPassword, hash)
	if err != nil {
		t.Fatalf("unexpected error verifying wrong password: %v", err)
	}
	if match {
		t.Error("expected wrong password to not match, got true")
	}
}

func TestVerifyPassword_InvalidHashes(t *testing.T) {
	testCases := []struct {
		name        string
		hash        string
		expectedErr error
	}{
		{"empty hash", "", security.ErrInvalidHashFormat},
		{"malformed hash", "not-a-hash", security.ErrInvalidHashFormat},
		{"wrong algorithm", "$bcrypt$v=19$m=65536,t=3,p=4$salt$hash", security.ErrUnsupportedAlgorithm},
		{"incompatible version", "$argon2id$v=16$m=65536,t=3,p=4$salt$hash", security.ErrIncompatibleVersion},
		{"invalid params format", "$argon2id$v=19$bad_params$salt$hash", security.ErrInvalidHashFormat},
		{"invalid base64 salt", "$argon2id$v=19$m=65536,t=3,p=4$!invalid_salt!$key", security.ErrInvalidHashFormat},
		{"zero iterations", "$argon2id$v=19$m=65536,t=0,p=4$c2FsdHNhbHQxMg$aGFzaGhhc2hoYXNoMTY", security.ErrInvalidHashFormat},
		{"zero parallelism", "$argon2id$v=19$m=65536,t=3,p=0$c2FsdHNhbHQxMg$aGFzaGhhc2hoYXNoMTY", security.ErrInvalidHashFormat},
		{"extreme memory (>256MB)", "$argon2id$v=19$m=1048576,t=3,p=4$c2FsdHNhbHQxMg$aGFzaGhhc2hoYXNoMTY", security.ErrInvalidHashFormat},
		{"truncated short salt", "$argon2id$v=19$m=65536,t=3,p=4$c2hvcnQ$aGFzaGhhc2hoYXNoMTY", security.ErrInvalidHashFormat},
		{"truncated short key", "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHQxMg$c2hvcnQ", security.ErrInvalidHashFormat},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			match, err := security.VerifyPassword("password123", tc.hash)
			if err == nil {
				t.Fatalf("expected error for %s, got match=%v", tc.name, match)
			}
			if tc.expectedErr != nil && err != tc.expectedErr {
				t.Errorf("expected error %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

func TestVerifyPassword_LongPassword(t *testing.T) {
	longPassword := strings.Repeat("A-Very-Long-Password-String-With-Over-100-Chars-!", 10)

	hash, err := security.HashPassword(longPassword)
	if err != nil {
		t.Fatalf("failed to hash long password: %v", err)
	}

	match, err := security.VerifyPassword(longPassword, hash)
	if err != nil {
		t.Fatalf("failed to verify long password: %v", err)
	}
	if !match {
		t.Error("expected long password to verify successfully, got false")
	}
}

func TestValidatePasswordComplexity(t *testing.T) {
	validPasswords := []string{
		"BudiSantoso123!",
		"P@ssword2026",
		"Strong#Pass99",
	}

	for _, p := range validPasswords {
		if err := security.ValidatePasswordComplexity(p); err != nil {
			t.Errorf("expected password %q to be valid, got: %v", p, err)
		}
	}

	invalidCases := []struct {
		password string
		reason   string
	}{
		{"Short1!", "too short (< 8 chars)"},
		{"alllowercasewithdigits123!", "missing uppercase"},
		{"ALLUPPERCASEWITHDIGITS123!", "missing lowercase"},
		{"NoDigitsHereAtAll!", "missing digits"},
		{"NoSymbolsAllowed123", "missing symbol"},
		{strings.Repeat("A1!a", 300), "too long (> 1024 chars)"},
	}

	for _, tc := range invalidCases {
		if err := security.ValidatePasswordComplexity(tc.password); err == nil {
			t.Errorf("expected password %q to fail validation (%s), got nil", tc.password, tc.reason)
		}
	}
}
