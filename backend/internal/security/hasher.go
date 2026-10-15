package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

var (
	ErrEmptyPassword        = errors.New("password cannot be empty")
	ErrInvalidHashFormat    = errors.New("invalid or malformed argon2id hash format")
	ErrUnsupportedAlgorithm = errors.New("unsupported password hashing algorithm")
	ErrIncompatibleVersion  = errors.New("incompatible argon2 version")
	ErrPasswordComplexity   = errors.New("password does not meet complexity requirements")
)

// Argon2idParams holds the cryptographic tuning parameters for Argon2id.
// Parameters follow OWASP recommendations: m=64MB, t=3, p=4, 16-byte salt, 32-byte key.
type Argon2idParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

var DefaultArgon2idParams = Argon2idParams{
	Memory:      64 * 1024, // 65536 KiB (64 MB)
	Iterations:  3,
	Parallelism: 4,
	SaltLength:  16,
	KeyLength:   32,
}

// HashPassword hashes a plain text password using Argon2id with default parameters.
// Returns a standardized PHC string: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
func HashPassword(password string) (string, error) {
	if len(password) == 0 {
		return "", ErrEmptyPassword
	}

	params := DefaultArgon2idParams
	salt := make([]byte, params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to generate secure salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		b64Salt,
		b64Hash,
	)

	return encoded, nil
}

// VerifyPassword verifies a password against an Argon2id encoded hash using constant-time comparison.
func VerifyPassword(password, encodedHash string) (bool, error) {
	if len(encodedHash) == 0 {
		return false, ErrInvalidHashFormat
	}

	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false, ErrInvalidHashFormat
	}

	if parts[1] != "argon2id" {
		return false, ErrUnsupportedAlgorithm
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, ErrInvalidHashFormat
	}
	if version != argon2.Version {
		return false, ErrIncompatibleVersion
	}

	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, ErrInvalidHashFormat
	}

	// Validate parameter bounds defensively to prevent panics or excessive resource consumption
	if memory < 8*uint32(parallelism) || memory > 256*1024 || iterations == 0 || iterations > 10 || parallelism == 0 {
		return false, ErrInvalidHashFormat
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return false, ErrInvalidHashFormat
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expectedHash) < 16 {
		return false, ErrInvalidHashFormat
	}

	computedHash := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memory,
		parallelism,
		uint32(len(expectedHash)),
	)

	if subtle.ConstantTimeCompare(expectedHash, computedHash) == 1 {
		return true, nil
	}

	return false, nil
}

// ValidatePasswordComplexity validates that a password meets security baseline:
// between 8 and 1024 characters, with at least 1 uppercase, 1 lowercase, 1 digit, and 1 symbol.
func ValidatePasswordComplexity(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("%w: minimum length is 8 characters", ErrPasswordComplexity)
	}
	if len(password) > 1024 {
		return fmt.Errorf("%w: maximum length is 1024 characters", ErrPasswordComplexity)
	}

	var hasUpper, hasLower, hasDigit, hasSymbol bool
	for _, ch := range password {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSymbol = true
		}
	}

	if !hasUpper {
		return fmt.Errorf("%w: must contain at least one uppercase letter", ErrPasswordComplexity)
	}
	if !hasLower {
		return fmt.Errorf("%w: must contain at least one lowercase letter", ErrPasswordComplexity)
	}
	if !hasDigit {
		return fmt.Errorf("%w: must contain at least one digit", ErrPasswordComplexity)
	}
	if !hasSymbol {
		return fmt.Errorf("%w: must contain at least one symbol", ErrPasswordComplexity)
	}

	return nil
}
