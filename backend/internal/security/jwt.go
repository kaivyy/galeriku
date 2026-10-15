package security

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrWeakSecret       = errors.New("JWT signing secret must be at least 32 bytes (256 bits)")
	ErrExpiredToken     = errors.New("access token has expired")
	ErrInvalidSignature = errors.New("access token signature is invalid")
	ErrInvalidAlgorithm = errors.New("unsupported or disallowed JWT signing algorithm")
	ErrMalformedToken   = errors.New("access token string is malformed")
	ErrInvalidClaims    = errors.New("access token contains invalid claims")
)

const (
	JWTIssuer   = "galeriku"
	JWTAudience = "galeriku-client"
)

// AccessTokenClaims represents the payload claims of a Galeriku access token.
type AccessTokenClaims struct {
	Email        string   `json:"email"`
	Capabilities []string `json:"caps"`
	jwt.RegisteredClaims
}

// GenerateAccessToken signs a new short-lived stateless JWT access token with HS256.
func GenerateAccessToken(secret []byte, userID, email string, capabilities []string, ttl time.Duration) (string, error) {
	if len(secret) < 32 {
		return "", ErrWeakSecret
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(email) == "" {
		return "", ErrInvalidClaims
	}

	now := time.Now()
	var jti string
	if jtiV7, err := uuid.NewV7(); err == nil {
		jti = jtiV7.String()
	} else {
		jti = uuid.New().String()
	}

	claims := AccessTokenClaims{
		Email:        email,
		Capabilities: capabilities,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    JWTIssuer,
			Audience:  jwt.ClaimStrings{JWTAudience},
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        jti,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signedToken, err := token.SignedString(secret)
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %w", err)
	}

	return signedToken, nil
}

// VerifyAccessToken verifies the signature, algorithm, and claims of a JWT access token.
func VerifyAccessToken(secret []byte, tokenStr string) (*AccessTokenClaims, error) {
	if len(secret) < 32 {
		return nil, ErrWeakSecret
	}

	trimmed := strings.TrimSpace(tokenStr)
	if trimmed == "" {
		return nil, ErrMalformedToken
	}

	parts := strings.Split(trimmed, ".")
	if len(parts) != 3 {
		return nil, ErrMalformedToken
	}

	// Strictly verify that the token header declares HS256 before signature verification
	parser := jwt.NewParser()
	unverifiedToken, _, err := parser.ParseUnverified(trimmed, &AccessTokenClaims{})
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrMalformedToken, err.Error())
	}
	if alg, ok := unverifiedToken.Header["alg"].(string); !ok || alg != "HS256" {
		return nil, ErrInvalidAlgorithm
	}

	token, err := jwt.ParseWithClaims(
		trimmed,
		&AccessTokenClaims{},
		func(token *jwt.Token) (interface{}, error) {
			// Enforce HS256 exclusively; reject "none", RS256, and other algorithms
			if token.Method != jwt.SigningMethodHS256 {
				return nil, fmt.Errorf("%w: expected HS256, got %v", ErrInvalidAlgorithm, token.Header["alg"])
			}
			return secret, nil
		},
		jwt.WithIssuer(JWTIssuer),
		jwt.WithAudience(JWTAudience),
		jwt.WithValidMethods([]string{"HS256"}),
	)

	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrExpiredToken
		case errors.Is(err, jwt.ErrSignatureInvalid):
			return nil, ErrInvalidSignature
		case errors.Is(err, ErrInvalidAlgorithm):
			return nil, ErrInvalidAlgorithm
		case errors.Is(err, jwt.ErrTokenMalformed):
			return nil, ErrMalformedToken
		case errors.Is(err, jwt.ErrTokenInvalidClaims) || errors.Is(err, jwt.ErrTokenInvalidIssuer) || errors.Is(err, jwt.ErrTokenInvalidAudience):
			return nil, ErrInvalidClaims
		default:
			errStr := err.Error()
			if strings.Contains(errStr, "signature is invalid") {
				return nil, ErrInvalidSignature
			}
			if strings.Contains(errStr, "expected HS256") || strings.Contains(errStr, "signing method") {
				return nil, ErrInvalidAlgorithm
			}
			if strings.Contains(errStr, "token is invalid") || strings.Contains(errStr, "claim") {
				return nil, ErrInvalidClaims
			}
			return nil, fmt.Errorf("%w: %s", ErrMalformedToken, err.Error())
		}
	}

	claims, ok := token.Claims.(*AccessTokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidClaims
	}

	return claims, nil
}
