package usecase

import (
	"context"

	"blackjack/backend/repository"
)

// RateLimitUsecase は配信層から使うレート制御ユースケース。
type RateLimitUsecase interface {
	Allow(ctx context.Context, key string) (bool, error)
}

type rateLimitService struct {
	repo repository.RateLimitRepository
}

func NewRateLimitUsecase(repo repository.RateLimitRepository) RateLimitUsecase {
	return &rateLimitService{repo: repo}
}

func (u *rateLimitService) Allow(ctx context.Context, key string) (bool, error) {
	return u.repo.Allow(ctx, key)
}
