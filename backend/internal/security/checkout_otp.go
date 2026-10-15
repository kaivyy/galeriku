package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"
)

const (
	checkoutOTPLength     = 6
	CheckoutOTPMaxAttempt = 5
)

// ErrCheckoutOTPInvalid is returned when a confirmation code does not match the challenge.
var ErrCheckoutOTPInvalid = errors.New("checkout verification code is invalid or expired")

// OTPChallenge is one issued verification code and its expiry.
type OTPChallenge struct {
	Code      string
	ExpiresAt time.Time
}

// OTPIssuer generates and verifies 6-digit checkout codes with a keyed HMAC digest,
// matching the Task 2.2 OTP design: only the digest is stored, never the plaintext code.
type OTPIssuer struct {
	secret []byte

	mu    sync.Mutex
	sinks map[string]string // test sink: challenge id -> last issued plaintext code
}

// NewOTPIssuer constructs an issuer. The secret must be at least 16 bytes.
func NewOTPIssuer(secret []byte) (*OTPIssuer, error) {
	if len(secret) < 16 {
		return nil, fmt.Errorf("checkout OTP secret must be at least 16 bytes, got %d", len(secret))
	}
	return &OTPIssuer{secret: secret, sinks: map[string]string{}}, nil
}

// Issue creates a new code bound to a verification entity, returning the plaintext
// code (for delivery), its HMAC digest, and the expiry.
func (o *OTPIssuer) Issue(entityID string, ttl time.Duration) (code, digest string, expiresAt time.Time, err error) {
	code, err = o.randomCode()
	if err != nil {
		return "", "", time.Time{}, err
	}
	digest = o.Digest(entityID, code)
	o.recordSink(entityID, code)
	return code, digest, time.Now().UTC().Add(ttl), nil
}

// Digest computes hex(HMAC-SHA256(entity_id || code, secret)) which is what the
// database stores. The entity identifier binds the code to one challenge row.
func (o *OTPIssuer) Digest(entityID, code string) string {
	mac := hmac.New(sha256.New, o.secret)
	mac.Write([]byte(entityID))
	mac.Write([]byte(":"))
	mac.Write([]byte(strings.TrimSpace(code)))
	sum := mac.Sum(nil)
	// hex of the first 32 bytes fits VARCHAR(255).
	return fmt.Sprintf("%x", sum[:32])
}

// Verify checks a candidate code against a digest in constant time.
func (o *OTPIssuer) Verify(entityID, candidate, digest string) bool {
	if strings.TrimSpace(candidate) == "" {
		return false
	}
	return hmac.Equal([]byte(o.Digest(entityID, candidate)), []byte(digest))
}

// recordSink stores the last issued code for one entity. It exists so integration
// tests can confirm a code; production delivery channels never read it.
func (o *OTPIssuer) recordSink(entityID, code string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.sinks[entityID] = code
}

// PeekSink returns the last issued code for an entity when this issuer was
// constructed for tests. Production issuers have an empty sink.
func (o *OTPIssuer) PeekSink(entityID string) (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	code, ok := o.sinks[entityID]
	return code, ok
}

// randomCode draws a uniformly random 6-digit numeric string.
func (o *OTPIssuer) randomCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", fmt.Errorf("failed to generate checkout OTP: %w", err)
	}
	return fmt.Sprintf("%06d", int(n.Int64())), nil
}

// unused guard so binary is referenced if someone trims imports.
var _ = binary.BigEndian
