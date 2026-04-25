package usecase

import (
	"context"
	"fmt"
	"strings"
	"time"

	"blackjack/backend/repository"
)

// RateLimitUsecase は配信層から使うレート制御ユースケース。
type RateLimitUsecase interface {
	Allow(ctx context.Context, key string) (RateLimitResult, error)
	AllowSignup(ctx context.Context, ip string, email string) (RateLimitDecision, error)
	AllowLogin(ctx context.Context, ip string, email string) (RateLimitDecision, error)
	AllowTasks(ctx context.Context, userID uint) (RateLimitDecision, error)
}

type RateLimitResult struct {
	Allowed      bool
	Tokens       float64
	RetryAfterMS int64
}

type RateLimitDecision struct {
	Allowed    bool
	Remaining  float64
	RetryAfter time.Duration
	LimitName  string
}

type rateLimitService struct {
	repo            repository.RateLimitRepository
	defaultRate     float64
	defaultCapacity float64
	signupRate      float64
	signupCapacity  float64
	loginRate       float64
	loginCapacity   float64
	taskRate        float64
	taskCapacity    float64
}

func NewRateLimitUsecase(repo repository.RateLimitRepository) RateLimitUsecase {
	return &rateLimitService{
		repo:            repo,
		defaultRate:     5.0,
		defaultCapacity: 20.0,
		signupRate:      1.0,
		signupCapacity:  5.0,
		loginRate:       1.0,
		loginCapacity:   5.0,
		taskRate:        5.0,
		taskCapacity:    20.0,
	}
}

func (u *rateLimitService) Allow(ctx context.Context, key string) (RateLimitResult, error) {
	allowed, tokens, retryAfterMS, err := u.repo.Allow(
		ctx,
		key,
		u.defaultRate,
		u.defaultCapacity,
		1.0,
		time.Now().UTC().UnixMilli(),
	)
	if err != nil {
		return RateLimitResult{}, err
	}
	return RateLimitResult{
		Allowed:      allowed,
		Tokens:       tokens,
		RetryAfterMS: retryAfterMS,
	}, nil
}

func (u *rateLimitService) AllowSignup(ctx context.Context, ip string, email string) (RateLimitDecision, error) {
	return u.allowUsername(ctx, "signup", ip, email, u.signupRate, u.signupCapacity)
}

func (u *rateLimitService) AllowLogin(ctx context.Context, ip string, email string) (RateLimitDecision, error) {
	return u.allowUsername(ctx, "login", ip, email, u.loginRate, u.loginCapacity)
}

func (u *rateLimitService) AllowTasks(ctx context.Context, userID uint) (RateLimitDecision, error) {
	key := fmt.Sprintf("rl:tasks:user:%d", userID)
	ok, rem, retry, err := u.repo.Allow(ctx, key, u.taskRate, u.taskCapacity, 1.0, time.Now().UTC().UnixMilli())
	if err != nil {
		return RateLimitDecision{}, err
	}
	return RateLimitDecision{
		Allowed:    ok,
		Remaining:  rem,
		RetryAfter: time.Duration(retry) * time.Millisecond,
		LimitName:  "tasks:user",
	}, nil
}

func (u *rateLimitService) allowUsername(
	ctx context.Context,
	action string,
	ip string,
	email string,
	rate float64,
	capacity float64,
) (RateLimitDecision, error) {
	now := time.Now().UTC().UnixMilli()
	const cost = 1.0

	ipKey := fmt.Sprintf("rl:%s:ip:%s", action, rateLimitIPKey(ip))
	ok, rem, retry, err := u.repo.Allow(ctx, ipKey, rate, capacity, cost, now)
	if err != nil {
		return RateLimitDecision{}, err
	}
	if !ok {
		return RateLimitDecision{
			Allowed:    false,
			Remaining:  rem,
			RetryAfter: time.Duration(retry) * time.Millisecond,
			LimitName:  action + ":ip",
		}, nil
	}

	email = rateLimitEmailKey(email)
	if email == "" {
		return RateLimitDecision{
			Allowed:    true,
			Remaining:  rem,
			RetryAfter: 0,
			LimitName:  action + ":ip",
		}, nil
	}

	emailKey := fmt.Sprintf("rl:%s:email:%s", action, email)
	ok2, rem2, retry2, err := u.repo.Allow(ctx, emailKey, rate, capacity, cost, now)
	if err != nil {
		return RateLimitDecision{}, err
	}
	if !ok2 {
		return RateLimitDecision{
			Allowed:    false,
			Remaining:  rem2,
			RetryAfter: time.Duration(retry2) * time.Millisecond,
			LimitName:  action + ":email",
		}, nil
	}

	remaining := rem
	if rem2 < remaining {
		remaining = rem2
	}
	return RateLimitDecision{
		Allowed:    true,
		Remaining:  remaining,
		RetryAfter: 0,
		LimitName:  action,
	}, nil
}

func rateLimitIPKey(ip string) string {
	v := strings.TrimSpace(ip)
	if v == "" {
		return "unknown"
	}
	v = strings.ReplaceAll(v, ":", "_")
	return strings.ReplaceAll(v, ".", "_")
}

func rateLimitEmailKey(email string) string {
	v := strings.ToLower(strings.TrimSpace(email))
	v = strings.ReplaceAll(v, " ", "")
	return v
}
