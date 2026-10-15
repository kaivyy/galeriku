package security_test

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"

	"github.com/galeriku/core/internal/security"
)

func TestGenerateOTP_FormatAndEntropy(t *testing.T) {
	otpRegex := regexp.MustCompile(`^[0-9]{6}$`)
	hashRegex := regexp.MustCompile(`^[0-9a-f]{64}$`)

	seen := make(map[string]bool)
	const sampleSize = 50

	for i := 0; i < sampleSize; i++ {
		code, hash, err := security.GenerateOTP()
		if err != nil {
			t.Fatalf("unexpected error generating OTP: %v", err)
		}

		if !otpRegex.MatchString(code) {
			t.Errorf("OTP code %q does not match 6-digit numeric pattern", code)
		}

		if !hashRegex.MatchString(hash) {
			t.Errorf("OTP hash %q is not a valid 64-char lowercase SHA-256 hex string", hash)
		}

		// Verify returned hash matches sha256(code)
		expectedDigest := sha256.Sum256([]byte(code))
		expectedHex := hex.EncodeToString(expectedDigest[:])
		if hash != expectedHex {
			t.Errorf("hash mismatch: got %q, expected %q", hash, expectedHex)
		}

		seen[code] = true
	}

	// In 50 random 6-digit codes, we expect high entropy (at least 45 unique codes)
	if len(seen) < 45 {
		t.Errorf("expected high randomness across 50 OTP generations, got only %d unique codes", len(seen))
	}
}

func TestHashOTP_Deterministic(t *testing.T) {
	code := "482910"
	hash1 := security.HashOTP(code)
	hash2 := security.HashOTP(code)

	if hash1 != hash2 {
		t.Errorf("expected deterministic hash output, got %q and %q", hash1, hash2)
	}

	if len(hash1) != 64 {
		t.Errorf("expected 64 characters hash length, got %d", len(hash1))
	}
}

func TestVerifyOTP_SuccessAndFailure(t *testing.T) {
	code := "123456"
	wrongCode := "654321"
	correctHash := security.HashOTP(code)

	// Correct code must match
	if !security.VerifyOTP(code, correctHash) {
		t.Error("expected correct OTP code to verify successfully, got false")
	}

	// Case-insensitivity check (uppercase expected hash should match)
	upperHash := strings.ToUpper(correctHash)
	if !security.VerifyOTP(code, upperHash) {
		t.Error("expected uppercase OTP hash to verify successfully, got false")
	}

	// Wrong code must fail
	if security.VerifyOTP(wrongCode, correctHash) {
		t.Error("expected wrong OTP code to fail verification, got true")
	}

	// Malformed hash must fail safely
	if security.VerifyOTP(code, "invalid-hash-string") {
		t.Error("expected verification against malformed hash to return false, got true")
	}
}

func TestValidateOTPFormat(t *testing.T) {
	validCodes := []string{"123456", "000000", "999999", "048291"}
	for _, c := range validCodes {
		if !security.ValidateOTPFormat(c) {
			t.Errorf("expected code %q to be valid, got false", c)
		}
	}

	invalidCodes := []string{
		"",         // empty
		"12345",    // 5 digits (too short)
		"1234567",  // 7 digits (too long)
		"12345a",   // alphanumeric
		"abcdef",   // alpha
		" 123456",  // leading space
		"123456 ",  // trailing space
		"12-345",   // hyphen
	}
	for _, c := range invalidCodes {
		if security.ValidateOTPFormat(c) {
			t.Errorf("expected code %q to be invalid, got true", c)
		}
	}
}

func TestKeyedOTP_DeterministicAndKeyBound(t *testing.T) {
	code := "482910"
	secret1 := []byte("secret_key_one_32_bytes_long_12345")
	secret2 := []byte("secret_key_two_32_bytes_long_67890")

	hash1, err := security.HashKeyedOTP(code, secret1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	hash2, err := security.HashKeyedOTP(code, secret1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("expected deterministic hash output, got %q and %q", hash1, hash2)
	}

	if len(hash1) != 64 {
		t.Errorf("expected 64 characters hash length, got %d", len(hash1))
	}

	// Different secrets must produce different hashes for the same OTP
	hashDiffSecret, err := security.HashKeyedOTP(code, secret2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash1 == hashDiffSecret {
		t.Errorf("expected different secrets to produce different hashes, got identical hash %q", hash1)
	}

	// Weak secret rejection (minimum 16 bytes)
	_, err = security.HashKeyedOTP(code, []byte("short"))
	if err == nil {
		t.Error("expected error for weak secret, got nil")
	}
}

func TestVerifyKeyedOTP(t *testing.T) {
	code := "123456"
	wrongCode := "654321"
	secret := []byte("strong_otp_secret_key_32_bytes_long")
	wrongSecret := []byte("wrong_otp_secret_key_32_bytes_long!")

	hash, err := security.HashKeyedOTP(code, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Correct code and secret
	if !security.VerifyKeyedOTP(code, hash, secret) {
		t.Error("expected correct code and secret to verify, got false")
	}

	// Wrong code
	if security.VerifyKeyedOTP(wrongCode, hash, secret) {
		t.Error("expected wrong code to fail verification, got true")
	}

	// Wrong secret
	if security.VerifyKeyedOTP(code, hash, wrongSecret) {
		t.Error("expected wrong secret to fail verification, got true")
	}

	// Malformed hash
	if security.VerifyKeyedOTP(code, "invalid-hash", secret) {
		t.Error("expected malformed hash to fail verification, got true")
	}

	// Malformed code
	if security.VerifyKeyedOTP("12345", hash, secret) {
		t.Error("expected 5-digit code to fail verification, got true")
	}
}

func TestGenerateKeyedOTP(t *testing.T) {
	secret := []byte("strong_otp_secret_key_32_bytes_long")

	code, codeHash, err := security.GenerateKeyedOTP(secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !security.ValidateOTPFormat(code) {
		t.Errorf("generated code %q failed format validation", code)
	}

	if !security.VerifyKeyedOTP(code, codeHash, secret) {
		t.Error("generated OTP failed verification against its keyed hash")
	}
}
