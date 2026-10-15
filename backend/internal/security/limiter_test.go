package security_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/galeriku/core/internal/security"
	"github.com/redis/go-redis/v9"
)

func getTestRedisClient(t *testing.T) (*redis.Client, func()) {
	if testing.Short() {
		t.Skip("skipping live Redis rate limiter test in short mode")
	}

	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("REDIS_PORT")
	if port == "" {
		port = "6380"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("%s:%s", host, port),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Skipf("skipping live Redis test (cannot connect to %s:%s): %v", host, port, err)
	}

	cleanup := func() {
		_ = rdb.Close()
	}

	return rdb, cleanup
}

func TestRedisRateLimiter_EmptyIdentifier(t *testing.T) {
	limiter := security.NewRedisRateLimiter(nil)
	ctx := context.Background()

	_, err := limiter.AllowOTPSend(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty identifier, got nil")
	}
	if err != security.ErrEmptyIdentifier {
		t.Errorf("expected ErrEmptyIdentifier, got: %v", err)
	}
}

func TestRedisRateLimiter_LiveCooldownAndBurst(t *testing.T) {
	rdb, cleanup := getTestRedisClient(t)
	defer cleanup()

	limiter := security.NewRedisRateLimiter(rdb)
	ctx := context.Background()
	testTarget := fmt.Sprintf("+62812%d", time.Now().UnixNano()%10000000)

	// Ensure clean slate before test
	_ = limiter.Reset(ctx, testTarget)
	defer func() {
		_ = limiter.Reset(ctx, testTarget)
	}()

	// 1. First send must be ALLOWED
	res1, err := limiter.AllowOTPSend(ctx, testTarget)
	if err != nil {
		t.Fatalf("unexpected error on 1st send: %v", err)
	}
	if !res1.Allowed {
		t.Errorf("expected 1st send to be allowed, got denied (reason: %s)", res1.Reason)
	}
	if res1.RetryAfter != 0 {
		t.Errorf("expected 0 RetryAfter on allowed send, got: %v", res1.RetryAfter)
	}

	// 2. Immediate second send must be DENIED due to 60s COOLDOWN
	res2, err := limiter.AllowOTPSend(ctx, testTarget)
	if err != nil {
		t.Fatalf("unexpected error on 2nd send: %v", err)
	}
	if res2.Allowed {
		t.Fatal("expected 2nd send to be blocked by cooldown, but got allowed")
	}
	if res2.Reason != security.LimitReasonCooldown {
		t.Errorf("expected reason %q, got %q", security.LimitReasonCooldown, res2.Reason)
	}
	if res2.RetryAfter <= 0 || res2.RetryAfter > 60*time.Second {
		t.Errorf("expected RetryAfter between 1s and 60s, got: %v", res2.RetryAfter)
	}

	// 3. Clear only cooldown to test burst limit of 3 sends / 10m
	err = limiter.ClearCooldown(ctx, testTarget)
	if err != nil {
		t.Fatalf("failed to clear cooldown for test: %v", err)
	}

	// 2nd effective send
	res3, err := limiter.AllowOTPSend(ctx, testTarget)
	if err != nil {
		t.Fatalf("unexpected error on 2nd effective send: %v", err)
	}
	if !res3.Allowed {
		t.Fatalf("expected 2nd effective send to be allowed, got denied (reason: %s)", res3.Reason)
	}

	_ = limiter.ClearCooldown(ctx, testTarget)

	// 3rd effective send
	res4, err := limiter.AllowOTPSend(ctx, testTarget)
	if err != nil {
		t.Fatalf("unexpected error on 3rd effective send: %v", err)
	}
	if !res4.Allowed {
		t.Fatalf("expected 3rd effective send to be allowed, got denied (reason: %s)", res4.Reason)
	}

	_ = limiter.ClearCooldown(ctx, testTarget)

	// 4th send must be DENIED due to BURST_EXCEEDED (max 3 per 10m)
	res5, err := limiter.AllowOTPSend(ctx, testTarget)
	if err != nil {
		t.Fatalf("unexpected error on 4th send: %v", err)
	}
	if res5.Allowed {
		t.Fatal("expected 4th send to be blocked by burst limit, but got allowed")
	}
	if res5.Reason != security.LimitReasonBurst {
		t.Errorf("expected reason %q, got %q", security.LimitReasonBurst, res5.Reason)
	}
	if res5.RetryAfter <= 0 || res5.RetryAfter > 600*time.Second {
		t.Errorf("expected RetryAfter between 1s and 600s, got: %v", res5.RetryAfter)
	}
}

func TestRedisRateLimiter_TargetIsolation(t *testing.T) {
	rdb, cleanup := getTestRedisClient(t)
	defer cleanup()

	limiter := security.NewRedisRateLimiter(rdb)
	ctx := context.Background()

	targetA := fmt.Sprintf("+62811%d", time.Now().UnixNano()%10000000)
	targetB := fmt.Sprintf("+62899%d", time.Now().UnixNano()%10000000)

	_ = limiter.Reset(ctx, targetA)
	_ = limiter.Reset(ctx, targetB)
	defer func() {
		_ = limiter.Reset(ctx, targetA)
		_ = limiter.Reset(ctx, targetB)
	}()

	// Target A first send
	resA, err := limiter.AllowOTPSend(ctx, targetA)
	if err != nil || !resA.Allowed {
		t.Fatalf("target A send failed: %v", err)
	}

	// Target B first send must be ALLOWED regardless of Target A
	resB, err := limiter.AllowOTPSend(ctx, targetB)
	if err != nil || !resB.Allowed {
		t.Fatalf("target B send should be allowed independently, got denied: %v", err)
	}
}

func TestRedisRateLimiter_LoginThrottling(t *testing.T) {
	rdb, cleanup := getTestRedisClient(t)
	defer cleanup()

	limiter := security.NewRedisRateLimiter(rdb)
	ctx := context.Background()
	testUser := fmt.Sprintf("user_%d@galeriku.com", time.Now().UnixNano())

	// 10 attempts allowed
	for i := 1; i <= security.LoginMaxBurstCount; i++ {
		res, err := limiter.AllowLogin(ctx, testUser)
		if err != nil {
			t.Fatalf("unexpected error on login attempt %d: %v", i, err)
		}
		if !res.Allowed {
			t.Fatalf("expected login attempt %d to be allowed, got denied", i)
		}
	}

	// 11th attempt must be blocked
	resBlock, err := limiter.AllowLogin(ctx, testUser)
	if err != nil {
		t.Fatalf("unexpected error on 11th login attempt: %v", err)
	}
	if resBlock.Allowed {
		t.Fatal("expected 11th login attempt to be blocked, but was allowed")
	}
	if resBlock.Reason != security.LimitReasonBurst {
		t.Errorf("expected reason %q, got %q", security.LimitReasonBurst, resBlock.Reason)
	}
	if resBlock.RetryAfter <= 0 {
		t.Errorf("expected positive RetryAfter, got: %v", resBlock.RetryAfter)
	}
}
