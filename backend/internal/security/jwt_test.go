package security_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/galeriku/core/internal/security"
	"github.com/golang-jwt/jwt/v5"
)

const testValidSecret = "super_secret_jwt_key_at_least_32_bytes_long_12345!"

func TestGenerateAccessToken_Success(t *testing.T) {
	secret := []byte(testValidSecret)
	userID := "01927c8b-5a12-7800-b611-d1029471abcd"
	email := "budi@galeriku.com"
	caps := []string{"BUYER", "CREATOR"}
	ttl := 15 * time.Minute

	tokenStr, err := security.GenerateAccessToken(secret, userID, email, caps, ttl)
	if err != nil {
		t.Fatalf("unexpected error generating access token: %v", err)
	}

	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts in JWT token string, got %d", len(parts))
	}

	// Verify token claims
	claims, err := security.VerifyAccessToken(secret, tokenStr)
	if err != nil {
		t.Fatalf("unexpected error verifying access token: %v", err)
	}

	if claims.Subject != userID {
		t.Errorf("expected Subject %q, got %q", userID, claims.Subject)
	}
	if claims.Email != email {
		t.Errorf("expected Email %q, got %q", email, claims.Email)
	}
	if len(claims.Capabilities) != 2 || claims.Capabilities[0] != "BUYER" || claims.Capabilities[1] != "CREATOR" {
		t.Errorf("expected Capabilities %v, got %v", caps, claims.Capabilities)
	}
	if claims.Issuer != "galeriku" {
		t.Errorf("expected Issuer 'galeriku', got %q", claims.Issuer)
	}
	if len(claims.Audience) == 0 || claims.Audience[0] != "galeriku-client" {
		t.Errorf("expected Audience 'galeriku-client', got %v", claims.Audience)
	}
	if claims.ID == "" {
		t.Error("expected non-empty JTI (ID) claim")
	}
}

func TestGenerateAccessToken_WeakSecret(t *testing.T) {
	weakSecret := []byte("too-short")
	userID := "01927c8b-5a12-7800-b611-d1029471abcd"
	email := "budi@galeriku.com"
	caps := []string{"BUYER"}

	_, err := security.GenerateAccessToken(weakSecret, userID, email, caps, 15*time.Minute)
	if err == nil {
		t.Fatal("expected error for weak secret (< 32 bytes), got nil")
	}
	if err != security.ErrWeakSecret {
		t.Errorf("expected ErrWeakSecret, got: %v", err)
	}
}

func TestVerifyAccessToken_ExpiredToken(t *testing.T) {
	secret := []byte(testValidSecret)
	userID := "01927c8b-5a12-7800-b611-d1029471abcd"
	email := "budi@galeriku.com"
	caps := []string{"BUYER"}

	// Generate token with negative TTL (already expired)
	tokenStr, err := security.GenerateAccessToken(secret, userID, email, caps, -1*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate expired token: %v", err)
	}

	_, err = security.VerifyAccessToken(secret, tokenStr)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	if err != security.ErrExpiredToken {
		t.Errorf("expected ErrExpiredToken, got: %v", err)
	}
}

func TestVerifyAccessToken_TamperedSignature(t *testing.T) {
	secret := []byte(testValidSecret)
	userID := "01927c8b-5a12-7800-b611-d1029471abcd"
	email := "budi@galeriku.com"
	caps := []string{"BUYER"}

	tokenStr, err := security.GenerateAccessToken(secret, userID, email, caps, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	parts := strings.Split(tokenStr, ".")
	// Tamper with signature part
	tamperedToken := parts[0] + "." + parts[1] + ".invalidSignatureBitsHere"

	_, err = security.VerifyAccessToken(secret, tamperedToken)
	if err == nil {
		t.Fatal("expected error for tampered signature, got nil")
	}
	if err != security.ErrInvalidSignature {
		t.Errorf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestVerifyAccessToken_WrongSecret(t *testing.T) {
	secret1 := []byte(testValidSecret)
	secret2 := []byte("another_completely_different_secret_key_32_bytes!")

	tokenStr, err := security.GenerateAccessToken(secret1, "user1", "user1@galeriku.com", []string{"BUYER"}, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	_, err = security.VerifyAccessToken(secret2, tokenStr)
	if err == nil {
		t.Fatal("expected error verifying with wrong secret, got nil")
	}
	if err != security.ErrInvalidSignature {
		t.Errorf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestVerifyAccessToken_NoneAlgorithmAttack(t *testing.T) {
	secret := []byte(testValidSecret)

	// Create an unauthenticated token with "none" algorithm
	claims := jwt.MapClaims{
		"sub": "attacker",
		"iss": "galeriku",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tokenStr, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to create none-algorithm token: %v", err)
	}

	_, err = security.VerifyAccessToken(secret, tokenStr)
	if err == nil {
		t.Fatal("expected rejection of none algorithm token, got nil")
	}
	if err != security.ErrInvalidAlgorithm {
		t.Errorf("expected ErrInvalidAlgorithm, got: %v", err)
	}
}

func TestVerifyAccessToken_MalformedTokens(t *testing.T) {
	secret := []byte(testValidSecret)

	malformedCases := []struct {
		name  string
		token string
	}{
		{"empty string", ""},
		{"single part", "onlyonepart"},
		{"two parts", "header.payload"},
		{"four parts", "part1.part2.part3.part4"},
		{"invalid base64", "header!bad.payload!bad.sig!bad"},
	}

	for _, tc := range malformedCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := security.VerifyAccessToken(secret, tc.token)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestVerifyAccessToken_InvalidPayloadJson(t *testing.T) {
	secret := []byte(testValidSecret)

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`not-json`))
	// Valid signature won't match, or parsing json will fail
	tokenStr := header + "." + payload + ".dummySig"

	_, err := security.VerifyAccessToken(secret, tokenStr)
	if err == nil {
		t.Fatal("expected error for invalid payload json, got nil")
	}
}

func TestGenerateAccessToken_EmptySubjectOrEmail(t *testing.T) {
	secret := []byte(testValidSecret)

	_, err := security.GenerateAccessToken(secret, "", "test@galeriku.com", []string{"BUYER"}, 15*time.Minute)
	if err != security.ErrInvalidClaims {
		t.Errorf("expected ErrInvalidClaims for empty subject, got: %v", err)
	}

	_, err = security.GenerateAccessToken(secret, "user123", "", []string{"BUYER"}, 15*time.Minute)
	if err != security.ErrInvalidClaims {
		t.Errorf("expected ErrInvalidClaims for empty email, got: %v", err)
	}
}

func TestVerifyAccessToken_InvalidIssuerOrAudience(t *testing.T) {
	secret := []byte(testValidSecret)

	// Token with wrong issuer
	claimsWrongIss := security.AccessTokenClaims{
		Email: "test@galeriku.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user123",
			Issuer:    "malicious-issuer",
			Audience:  jwt.ClaimStrings{security.JWTAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	tokenIss := jwt.NewWithClaims(jwt.SigningMethodHS256, claimsWrongIss)
	signedIss, err := tokenIss.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	_, err = security.VerifyAccessToken(secret, signedIss)
	if err != security.ErrInvalidClaims {
		t.Errorf("expected ErrInvalidClaims for wrong issuer, got: %v", err)
	}

	// Token with wrong audience
	claimsWrongAud := security.AccessTokenClaims{
		Email: "test@galeriku.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user123",
			Issuer:    security.JWTIssuer,
			Audience:  jwt.ClaimStrings{"foreign-audience"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
		},
	}
	tokenAud := jwt.NewWithClaims(jwt.SigningMethodHS256, claimsWrongAud)
	signedAud, err := tokenAud.SignedString(secret)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	_, err = security.VerifyAccessToken(secret, signedAud)
	if err != security.ErrInvalidClaims {
		t.Errorf("expected ErrInvalidClaims for wrong audience, got: %v", err)
	}
}

func TestGenerateAccessToken_JTI_UUIDv7(t *testing.T) {
	secret := []byte(testValidSecret)

	tokenStr, err := security.GenerateAccessToken(secret, "user123", "test@galeriku.com", []string{"BUYER"}, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	claims, err := security.VerifyAccessToken(secret, tokenStr)
	if err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}

	// JTI should be a valid UUID
	if len(claims.ID) != 36 {
		t.Errorf("expected 36-char UUID string for JTI, got %q", claims.ID)
	}
	// Check version 7 digit: xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx
	if claims.ID[14] != '7' {
		t.Errorf("expected UUIDv7 (version 7 at index 14), got: %s", claims.ID)
	}
}

