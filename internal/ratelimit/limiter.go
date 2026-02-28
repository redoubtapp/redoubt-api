package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/redoubtapp/redoubt-api/internal/config"
)

// Result represents the result of a rate limit check.
type Result struct {
	Allowed   bool
	Limit     int
	Remaining int
	ResetAt   time.Time
}

// Limiter implements Redis-backed sliding window rate limiting.
type Limiter struct {
	client *redis.Client
	rules  map[string]config.RateLimitRule
}

// NewLimiter creates a new rate limiter.
func NewLimiter(client *redis.Client, cfg config.RateLimitConfig) *Limiter {
	return &Limiter{
		client: client,
		rules:  cfg.Rules,
	}
}

// Check checks if a request is allowed under the specified rule.
// scope: the rule name (e.g., "register", "login", "general")
// identifier: the unique identifier for the client (e.g., IP, UserID, IP:Email)
func (l *Limiter) Check(ctx context.Context, scope string, identifier string) (*Result, error) {
	rule, ok := l.rules[scope]
	if !ok {
		// If no rule exists, allow the request
		return &Result{
			Allowed:   true,
			Limit:     0,
			Remaining: 0,
			ResetAt:   time.Now(),
		}, nil
	}

	return l.checkSlidingWindow(ctx, scope, identifier, rule.Limit, rule.Window)
}

// checkSlidingWindow implements the sliding window algorithm using Redis sorted sets.
func (l *Limiter) checkSlidingWindow(
	ctx context.Context,
	scope string,
	identifier string,
	limit int,
	window time.Duration,
) (*Result, error) {
	key := fmt.Sprintf("ratelimit:%s:%s", scope, identifier)
	now := time.Now()
	nowUnix := float64(now.UnixNano())
	windowStart := float64(now.Add(-window).UnixNano())

	// Use a pipeline for atomic operations
	pipe := l.client.Pipeline()

	// Remove expired entries
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%f", windowStart))

	// Add current request
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  nowUnix,
		Member: fmt.Sprintf("%d", now.UnixNano()),
	})

	// Count requests in window
	countCmd := pipe.ZCard(ctx, key)

	// Set expiry on the key
	pipe.Expire(ctx, key, window)

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return nil, fmt.Errorf("redis pipeline error: %w", err)
	}

	count := int(countCmd.Val())
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	resetAt := now.Add(window)

	return &Result{
		Allowed:   count <= limit,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   resetAt,
	}, nil
}

// GetRule returns the rule for a given scope.
func (l *Limiter) GetRule(scope string) (config.RateLimitRule, bool) {
	rule, ok := l.rules[scope]
	return rule, ok
}
