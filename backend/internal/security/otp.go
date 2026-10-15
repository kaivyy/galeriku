package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

var (
	ErrWeakOTPSecret = errors.New("OTP secret key must be at least 16 bytes")
)

const (
	// OTPLength is the standard 6-digit numeric length for Galeriku verification codes.
	OTPLength = 6

	// DefaultOTPExpiry is the standard 3-minute validity window for OTP challenges.
	DefaultOTPExpiry = 3 * time.Minute

	// DefaultMaxAttempts is the maximum number of failed attempts before challenge invalidation.
	DefaultMaxAttempts = 3
)

// GenerateOTP produces a cryptographically secure 6-digit numeric OTP and its SHA-256 digest.
// Raw code is intended for one-time dispatch; only codeHash should be persisted to storage.
func GenerateOTP() (code string, codeHash string, err error) {
	// Range: 0 to 999999 inclusive
	maxVal := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, maxVal)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate random OTP: %w", err)
	}

	code = fmt.Sprintf("%06d", n.Int64())
	codeHash = HashOTP(code)

	return code, codeHash, nil
}

// GenerateKeyedOTP produces a 6-digit numeric OTP and an HMAC-SHA256 digest keyed with server secret.
// This prevents offline reverse-lookup dictionary attacks if PostgreSQL is compromised.
func GenerateKeyedOTP(secret []byte) (code string, codeHash string, err error) {
	if len(secret) < 16 {
		return "", "", ErrWeakOTPSecret
	}

	maxVal := big.NewInt(1000000)
	n, err := rand.Int(rand.Reader, maxVal)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate random OTP: %w", err)
	}

	code = fmt.Sprintf("%06d", n.Int64())
	codeHash, err = HashKeyedOTP(code, secret)
	if err != nil {
		return "", "", err
	}

	return code, codeHash, nil
}

// HashOTP computes the 64-character lowercase hexadecimal SHA-256 digest of an OTP string.
func HashOTP(code string) string {
	digest := sha256.Sum256([]byte(code))
	return hex.EncodeToString(digest[:])
}

// HashKeyedOTP computes the 64-character lowercase hexadecimal HMAC-SHA256 digest of an OTP string using a secret.
func HashKeyedOTP(code string, secret []byte) (string, error) {
	if len(secret) < 16 {
		return "", ErrWeakOTPSecret
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// VerifyKeyedOTP verifies a candidate OTP code against an expected HMAC-SHA256 hash using constant-time comparison.
func VerifyKeyedOTP(code, expectedHash string, secret []byte) bool {
	if len(secret) < 16 || !ValidateOTPFormat(code) {
		return false
	}

	normExpected := strings.ToLower(strings.TrimSpace(expectedHash))
	if len(normExpected) != 64 {
		return false
	}

	computedHash, err := HashKeyedOTP(code, secret)
	if err != nil {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(computedHash), []byte(normExpected)) == 1
}

// VerifyOTP verifies a candidate OTP code against an expected SHA-256 hash using constant-time comparison.
func VerifyOTP(code, expectedHash string) bool {
	normExpected := strings.ToLower(strings.TrimSpace(expectedHash))
	if !ValidateOTPFormat(code) || len(normExpected) != 64 {
		return false
	}

	computedHash := HashOTP(code)
	return subtle.ConstantTimeCompare([]byte(computedHash), []byte(normExpected)) == 1
}

// ValidateOTPFormat checks if a string is exactly 6 numeric digits without whitespace.
func ValidateOTPFormat(code string) bool {
	if len(code) != OTPLength {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}
