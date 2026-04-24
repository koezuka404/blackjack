package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimitRepository はレート制御の永続化/外部I/O（Redis）を扱うポート。
type RateLimitRepository interface {
	Allow(ctx context.Context, key string) (allowed bool, tokens float64, retryAfterMS int64, err error)
}

type RedisTokenBucketRepository struct {
	rdb      *redis.Client
	capacity int
	refill   float64
}

func NewRedisTokenBucketRepository(rdb *redis.Client, capacity int, refillPerSec float64) *RedisTokenBucketRepository {
	return &RedisTokenBucketRepository{rdb: rdb, capacity: capacity, refill: refillPerSec}
}

var tokenBucketScript = redis.NewScript(`
local tokens_key = KEYS[1]
local ts_key = KEYS[2]
local capacity = tonumber(ARGV[1])
local refill = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local ttl_ms = tonumber(ARGV[4])
local cost = tonumber(ARGV[5])

local tokens = tonumber(redis.call("GET", tokens_key))
if tokens == nil then tokens = capacity end
local last_ms = tonumber(redis.call("GET", ts_key))
if last_ms == nil then last_ms = now_ms end

local elapsed = now_ms - last_ms
if elapsed < 0 then elapsed = 0 end
local refill_tokens = elapsed * refill / 1000.0
tokens = math.min(capacity, tokens + refill_tokens)

local allowed = 0
if tokens >= cost then
  tokens = tokens - cost
  allowed = 1
end

redis.call("SET", tokens_key, tokens, "PX", ttl_ms)
redis.call("SET", ts_key, now_ms, "PX", ttl_ms)
local retry_after_ms = 0
if allowed == 0 then
  retry_after_ms = math.ceil((cost - tokens) * 1000.0 / refill)
  if retry_after_ms < 0 then retry_after_ms = 0 end
end
return {allowed, tokens, retry_after_ms}
`)

func (r *RedisTokenBucketRepository) Allow(ctx context.Context, key string) (bool, float64, int64, error) {
	now := time.Now().UTC().UnixMilli()
	ttl := int64((float64(r.capacity)/r.refill)*2000 + 1000)
	const cost = 1.0
	res, err := tokenBucketScript.Run(
		ctx,
		r.rdb,
		[]string{"rate:" + key + ":tokens", "rate:" + key + ":ts"},
		r.capacity, strconv.FormatFloat(r.refill, 'f', -1, 64), now, ttl, strconv.FormatFloat(cost, 'f', -1, 64),
	).Result()
	if err != nil {
		return false, 0, 0, err
	}
	arr, ok := res.([]any)
	if !ok || len(arr) != 3 {
		return false, 0, 0, errors.New("unexpected lua result")
	}
	allowed, err := toInt64(arr[0])
	if err != nil {
		return false, 0, 0, fmt.Errorf("parse allowed: %w", err)
	}
	tokens, err := toFloat64(arr[1])
	if err != nil {
		return false, 0, 0, fmt.Errorf("parse tokens: %w", err)
	}
	retryAfterMS, err := toInt64(arr[2])
	if err != nil {
		return false, 0, 0, fmt.Errorf("parse retry_after_ms: %w", err)
	}
	return allowed == 1, tokens, retryAfterMS, nil
}

func toInt64(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case float64:
		return int64(x), nil
	case string:
		return strconv.ParseInt(x, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported int64 type %T", v)
	}
}

func toFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case int64:
		return float64(x), nil
	case string:
		return strconv.ParseFloat(x, 64)
	default:
		return 0, fmt.Errorf("unsupported float64 type %T", v)
	}
}
