package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"blackjack/backend/dto"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

type RateLimiter interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type RedisTokenBucketLimiter struct {
	rdb      *redis.Client
	capacity int
	refill   float64
}

func NewRedisTokenBucketLimiter(rdb *redis.Client, capacity int, refillPerSec float64) *RedisTokenBucketLimiter {
	return &RedisTokenBucketLimiter{rdb: rdb, capacity: capacity, refill: refillPerSec}
}

var tokenBucketScript = redis.NewScript(`
local tokens_key = KEYS[1]
local ts_key = KEYS[2]
local capacity = tonumber(ARGV[1])
local refill = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local ttl_ms = tonumber(ARGV[4])

local tokens = tonumber(redis.call("GET", tokens_key))
if tokens == nil then tokens = capacity end
local last_ms = tonumber(redis.call("GET", ts_key))
if last_ms == nil then last_ms = now_ms end

local elapsed = now_ms - last_ms
if elapsed < 0 then elapsed = 0 end
local refill_tokens = elapsed * refill / 1000.0
tokens = math.min(capacity, tokens + refill_tokens)

local allowed = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
end

redis.call("SET", tokens_key, tokens, "PX", ttl_ms)
redis.call("SET", ts_key, now_ms, "PX", ttl_ms)
return allowed
`)

func (l *RedisTokenBucketLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().UTC().UnixMilli()
	ttl := int64((float64(l.capacity)/l.refill)*2000 + 1000)
	res, err := tokenBucketScript.Run(ctx, l.rdb,
		[]string{"rate:" + key + ":tokens", "rate:" + key + ":ts"},
		l.capacity, strconv.FormatFloat(l.refill, 'f', -1, 64), now, ttl,
	).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func RateLimitMiddleware(limiter RateLimiter) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if limiter == nil {
				return next(c)
			}
			userID, _ := c.Get("user_id").(string)
			if userID == "" {
				return next(c)
			}
			ok, err := limiter.Allow(c.Request().Context(), "http:"+userID)
			if err != nil {
				return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
			}
			if !ok {
				return c.JSON(http.StatusTooManyRequests, dto.Fail("rate_limited", "too many requests"))
			}
			return next(c)
		}
	}
}
