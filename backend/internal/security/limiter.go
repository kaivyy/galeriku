package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrEmptyIdentifier = errors.New("rate limit identifier cannot be empty")
	ErrRedisClientNil  = errors.New("redis client cannot be nil")
)

const (
	LimitReasonAllowed  = "ALLOWED"
	LimitReasonCooldown = "COOLDOWN"
	LimitReasonBurst    = "BURST_EXCEEDED"

	// OTPCooldownSeconds is the mandatory waiting period between consecutive OTP sends (1 per 60s).
	OTPCooldownSeconds = 60

	// OTPBurstWindowSeconds is the sliding rate-limit accounting window (10 minutes = 600s).
	OTPBurstWindowSeconds = 600

	// OTPMaxBurstCount is the maximum allowable OTP sends within the burst accounting window.
	OTPMaxBurstCount = 3

	// LoginBurstWindowSeconds is the rate-limit window for login attempts (1 minute = 60s).
	LoginBurstWindowSeconds = 60

	// LoginMaxBurstCount is the maximum allowable login attempts within the burst window per identifier.
	LoginMaxBurstCount = 10
)

// RateLimitResult conveys the outcome of a rate check.
type RateLimitResult struct {
	Allowed    bool
	RetryAfter time.Duration
	Reason     string
}

// RateLimiter defines the interface for guarding verification and auth workflows against abuse.
type RateLimiter interface {
	AllowOTPSend(ctx context.Context, identifier string) (RateLimitResult, error)
	AllowLogin(ctx context.Context, identifier string) (RateLimitResult, error)
	ClearCooldown(ctx context.Context, identifier string) error
	Reset(ctx context.Context, identifier string) error
}

// RedisRateLimiter evaluates rate limits atomically in Redis.
type RedisRateLimiter struct {
	client *redis.Client
}

// NewRedisRateLimiter constructs a new rate limiter with the supplied Redis client.
func NewRedisRateLimiter(client *redis.Client) *RedisRateLimiter {
	return &RedisRateLimiter{client: client}
}

// Lua script to atomically check and record rate limits in a single roundtrip.
// Keys: KEYS[1] = cooldown key, KEYS[2] = burst key
// Args: ARGV[1] = cooldown seconds, ARGV[2] = burst seconds, ARGV[3] = burst max count
var otpRateLimitScript = redis.NewScript(`
local cooldown = redis.call('EXISTS', KEYS[1])
if cooldown == 1 then
    local ttl = redis.call('TTL', KEYS[1])
    if ttl < 1 then ttl = 1 end
    return {0, ttl, 'COOLDOWN'}
end

local count = redis.call('GET', KEYS[2])
if count and tonumber(count) >= tonumber(ARGV[3]) then
    local ttl = redis.call('TTL', KEYS[2])
    if ttl < 1 then ttl = 1 end
    return {0, ttl, 'BURST_EXCEEDED'}
end

if tonumber(ARGV[1]) > 0 then
    redis.call('SET', KEYS[1], '1', 'EX', tonumber(ARGV[1]))
end
local new_count = redis.call('INCR', KEYS[2])
if new_count == 1 or redis.call('TTL', KEYS[2]) < 0 then
    redis.call('EXPIRE', KEYS[2], tonumber(ARGV[2]))
end

return {1, 0, 'ALLOWED'}
`)

func (r *RedisRateLimiter) makeKeys(identifier string) (string, string) {
	norm := strings.ToLower(strings.TrimSpace(identifier))
	digest := sha256.Sum256([]byte(norm))
	hashedID := hex.EncodeToString(digest[:16])

	// Use Redis hash tags {...} to guarantee both keys map to the same slot in Redis Cluster
	cooldownKey := fmt.Sprintf("ratelimit:otp:{%s}:cooldown", hashedID)
	burstKey := fmt.Sprintf("ratelimit:otp:{%s}:burst", hashedID)

	return cooldownKey, burstKey
}

// AllowOTPSend checks whether an OTP dispatch is permissible for the target identifier.
// Enforces: max 1 send / 60 seconds (cooldown) AND max 3 sends / 10 minutes (burst).
func (r *RedisRateLimiter) AllowOTPSend(ctx context.Context, identifier string) (RateLimitResult, error) {
	if strings.TrimSpace(identifier) == "" {
		return RateLimitResult{Allowed: false}, ErrEmptyIdentifier
	}

	if r.client == nil {
		return RateLimitResult{Allowed: false}, ErrRedisClientNil
	}

	cooldownKey, burstKey := r.makeKeys(identifier)

	keys := []string{cooldownKey, burstKey}
	args := []interface{}{OTPCooldownSeconds, OTPBurstWindowSeconds, OTPMaxBurstCount}

	res, err := otpRateLimitScript.Run(ctx, r.client, keys, args...).Slice()
	if err != nil {
		// Fail closed on Redis operational error
		return RateLimitResult{Allowed: false}, fmt.Errorf("failed to evaluate rate limit in Redis: %w", err)
	}

	if len(res) < 3 {
		return RateLimitResult{Allowed: false}, fmt.Errorf("unexpected script response shape from Redis")
	}

	allowedVal, _ := res[0].(int64)
	ttlVal, _ := res[1].(int64)
	reasonStr, _ := res[2].(string)

	isAllowed := allowedVal == 1
	var retryAfter time.Duration
	if !isAllowed && ttlVal > 0 {
		retryAfter = time.Duration(ttlVal) * time.Second
	}

	return RateLimitResult{
		Allowed:    isAllowed,
		RetryAfter: retryAfter,
		Reason:     reasonStr,
	}, nil
}

// ClearCooldown removes the cooldown key for an identifier (useful for test resets).
func (r *RedisRateLimiter) ClearCooldown(ctx context.Context, identifier string) error {
	if strings.TrimSpace(identifier) == "" {
		return ErrEmptyIdentifier
	}
	if r.client == nil {
		return ErrRedisClientNil
	}

	cooldownKey, _ := r.makeKeys(identifier)
	return r.client.Del(ctx, cooldownKey).Err()
}

func (r *RedisRateLimiter) makeLoginKeys(identifier string) (string, string) {
	norm := strings.ToLower(strings.TrimSpace(identifier))
	digest := sha256.Sum256([]byte(norm))
	hashedID := hex.EncodeToString(digest[:16])

	cooldownKey := fmt.Sprintf("ratelimit:login:{%s}:cooldown", hashedID)
	burstKey := fmt.Sprintf("ratelimit:login:{%s}:burst", hashedID)

	return cooldownKey, burstKey
}

// AllowLogin checks whether a login attempt is permissible for the target identifier.
// Enforces: max 10 attempts / 60 seconds (burst) with zero mandatory cooldown.
func (r *RedisRateLimiter) AllowLogin(ctx context.Context, identifier string) (RateLimitResult, error) {
	if strings.TrimSpace(identifier) == "" {
		return RateLimitResult{Allowed: false}, ErrEmptyIdentifier
	}

	if r.client == nil {
		return RateLimitResult{Allowed: false}, ErrRedisClientNil
	}

	cooldownKey, burstKey := r.makeLoginKeys(identifier)

	keys := []string{cooldownKey, burstKey}
	args := []interface{}{0, LoginBurstWindowSeconds, LoginMaxBurstCount}

	res, err := otpRateLimitScript.Run(ctx, r.client, keys, args...).Slice()
	if err != nil {
		return RateLimitResult{Allowed: false}, fmt.Errorf("failed to evaluate login rate limit in Redis: %w", err)
	}

	if len(res) < 3 {
		return RateLimitResult{Allowed: false}, fmt.Errorf("unexpected script response shape from Redis")
	}

	allowedVal, _ := res[0].(int64)
	ttlVal, _ := res[1].(int64)
	reasonStr, _ := res[2].(string)

	isAllowed := allowedVal == 1
	var retryAfter time.Duration
	if !isAllowed && ttlVal > 0 {
		retryAfter = time.Duration(ttlVal) * time.Second
	}

	return RateLimitResult{
		Allowed:    isAllowed,
		RetryAfter: retryAfter,
		Reason:     reasonStr,
	}, nil
}

// Reset clears both cooldown and burst rate-limiting keys for an identifier.
func (r *RedisRateLimiter) Reset(ctx context.Context, identifier string) error {
	if strings.TrimSpace(identifier) == "" {
		return ErrEmptyIdentifier
	}
	if r.client == nil {
		return ErrRedisClientNil
	}

	otpCooldown, otpBurst := r.makeKeys(identifier)
	loginCooldown, loginBurst := r.makeLoginKeys(identifier)
	return r.client.Del(ctx, otpCooldown, otpBurst, loginCooldown, loginBurst).Err()
}
