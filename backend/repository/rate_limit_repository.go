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
	Allow(ctx context.Context, key string, rate float64, capacity float64, cost float64, nowMS int64) (allowed bool, tokens float64, retryAfterMS int64, err error)
}

type RedisTokenBucketRepository struct {
	rdb *redis.Client
}

func NewRedisTokenBucketRepository(rdb *redis.Client, capacity int, refillPerSec float64) *RedisTokenBucketRepository {
	_ = capacity
	_ = refillPerSec
	return &RedisTokenBucketRepository{rdb: rdb}
}

var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
local now = tonumber(ARGV[4])

if rate <= 0 or capacity <= 0 or cost <= 0 then
  return {0, 0, 0}
end

local data = redis.call("HMGET", key, "tokens", "ts")
local tokens = tonumber(data[1])
local ts = tonumber(data[2])

if tokens == nil then tokens = capacity end
if ts == nil then ts = now end

local delta = now - ts
if delta < 0 then delta = 0 end

local refill = (delta / 1000.0) * rate
tokens = math.min(capacity, tokens + refill)
ts = now

local allowed = 0
local retry_after_ms = 0

if tokens >= cost then
  allowed = 1
  tokens = tokens - cost
else
  local need = cost - tokens
  retry_after_ms = math.ceil((need / rate) * 1000.0)
end

redis.call("HMSET", key, "tokens", tokens, "ts", ts)

local ttl_ms = math.ceil((capacity / rate) * 2000.0)
if ttl_ms < 1000 then ttl_ms = 1000 end
redis.call("PEXPIRE", key, ttl_ms)

return {allowed, tokens, retry_after_ms}
`)

func (r *RedisTokenBucketRepository) Allow(ctx context.Context, key string, rate float64, capacity float64, cost float64, nowMS int64) (bool, float64, int64, error) {
	if r.rdb == nil {
		return false, 0, 0, errors.New("redis client is nil")
	}
	ctx, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	res, err := tokenBucketScript.Run(
		ctx,
		r.rdb,
		[]string{"rate:" + key},
		strconv.FormatFloat(rate, 'f', -1, 64),
		strconv.FormatFloat(capacity, 'f', -1, 64),
		strconv.FormatFloat(cost, 'f', -1, 64),
		nowMS,
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
