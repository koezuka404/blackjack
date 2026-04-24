package usecase

import (
	"context"

	"blackjack/backend/repository"
)

// RateLimitUsecase は配信層から使うレート制御ユースケース。
type RateLimitUsecase interface {
	Allow(ctx context.Context, key string) (RateLimitResult, error)
}

type RateLimitResult struct {
	Allowed      bool
	Tokens       float64
	RetryAfterMS int64
}

type rateLimitService struct {
	repo repository.RateLimitRepository
}

func NewRateLimitUsecase(repo repository.RateLimitRepository) RateLimitUsecase {
	return &rateLimitService{repo: repo}
}

func (u *rateLimitService) Allow(ctx context.Context, key string) (RateLimitResult, error) {
	allowed, tokens, retryAfterMS, err := u.repo.Allow(ctx, key)
	if err != nil {
		return RateLimitResult{}, err
	}
	return RateLimitResult{
		Allowed:      allowed,
		Tokens:       tokens,
		RetryAfterMS: retryAfterMS,
	}, nil
}
