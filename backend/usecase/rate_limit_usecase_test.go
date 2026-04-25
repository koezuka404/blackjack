package usecase

import (
	"context"
	"errors"
	"testing"
	"time"
)

type allowResp struct {
	allowed bool
	tokens  float64
	retryMS int64
	err     error
}

type fakeRateLimitRepo struct {
	calls []string
	queue []allowResp
}

func (f *fakeRateLimitRepo) Allow(ctx context.Context, key string, rate float64, capacity float64, cost float64, nowMS int64) (bool, float64, int64, error) {
	_ = ctx
	_ = rate
	_ = capacity
	_ = cost
	_ = nowMS
	f.calls = append(f.calls, key)
	if len(f.queue) == 0 {
		return true, 10, 0, nil
	}
	out := f.queue[0]
	f.queue = f.queue[1:]
	return out.allowed, out.tokens, out.retryMS, out.err
}

func TestRateLimitUsecase_Allow(t *testing.T) {
	repo := &fakeRateLimitRepo{
		queue: []allowResp{{allowed: true, tokens: 19, retryMS: 0}},
	}
	u := NewRateLimitUsecase(repo)
	got, err := u.Allow(context.Background(), "http:user-1")
	if err != nil {
		t.Fatalf("allow failed: %v", err)
	}
	if !got.Allowed || got.Tokens != 19 || got.RetryAfterMS != 0 {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestRateLimitUsecase_AllowSignup_IPLimited(t *testing.T) {
	repo := &fakeRateLimitRepo{
		queue: []allowResp{{allowed: false, tokens: 0, retryMS: 900}},
	}
	u := NewRateLimitUsecase(repo)
	got, err := u.AllowSignup(context.Background(), "127.0.0.1", "name@example.com")
	if err != nil {
		t.Fatalf("allow signup failed: %v", err)
	}
	if got.Allowed {
		t.Fatal("expected blocked")
	}
	if got.LimitName != "signup:ip" {
		t.Fatalf("unexpected limit name: %s", got.LimitName)
	}
	if got.RetryAfter != 900*time.Millisecond {
		t.Fatalf("unexpected retry: %s", got.RetryAfter)
	}
}

func TestRateLimitUsecase_AllowLogin_EmailLimited(t *testing.T) {
	repo := &fakeRateLimitRepo{
		queue: []allowResp{
			{allowed: true, tokens: 3, retryMS: 0},
			{allowed: false, tokens: 0, retryMS: 1200},
		},
	}
	u := NewRateLimitUsecase(repo)
	got, err := u.AllowLogin(context.Background(), "127.0.0.1", "name@example.com")
	if err != nil {
		t.Fatalf("allow login failed: %v", err)
	}
	if got.Allowed {
		t.Fatal("expected blocked")
	}
	if got.LimitName != "login:email" {
		t.Fatalf("unexpected limit name: %s", got.LimitName)
	}
}

func TestRateLimitUsecase_AllowSignup_SuccessWithoutEmail(t *testing.T) {
	repo := &fakeRateLimitRepo{
		queue: []allowResp{{allowed: true, tokens: 4, retryMS: 0}},
	}
	u := NewRateLimitUsecase(repo)
	got, err := u.AllowSignup(context.Background(), "127.0.0.1", "   ")
	if err != nil {
		t.Fatalf("allow signup failed: %v", err)
	}
	if !got.Allowed {
		t.Fatal("expected allowed")
	}
	if got.LimitName != "signup:ip" {
		t.Fatalf("unexpected limit name: %s", got.LimitName)
	}
}

func TestRateLimitUsecase_AllowTasks(t *testing.T) {
	repo := &fakeRateLimitRepo{
		queue: []allowResp{{allowed: true, tokens: 15, retryMS: 0}},
	}
	u := NewRateLimitUsecase(repo)
	got, err := u.AllowTasks(context.Background(), 42)
	if err != nil {
		t.Fatalf("allow tasks failed: %v", err)
	}
	if !got.Allowed || got.LimitName != "tasks:user" {
		t.Fatalf("unexpected result: %+v", got)
	}
	if len(repo.calls) != 1 || repo.calls[0] != "rl:tasks:user:42" {
		t.Fatalf("unexpected key calls: %+v", repo.calls)
	}
}

func TestRateLimitUsecase_AllowTasks_BlockedAndError(t *testing.T) {
	t.Run("blocked returns retry and remaining", func(t *testing.T) {
		repo := &fakeRateLimitRepo{
			queue: []allowResp{{allowed: false, tokens: 0, retryMS: 1500}},
		}
		u := NewRateLimitUsecase(repo)
		got, err := u.AllowTasks(context.Background(), 7)
		if err != nil {
			t.Fatalf("allow tasks failed: %v", err)
		}
		if got.Allowed {
			t.Fatal("expected blocked")
		}
		if got.Remaining != 0 || got.RetryAfter != 1500*time.Millisecond || got.LimitName != "tasks:user" {
			t.Fatalf("unexpected blocked result: %+v", got)
		}
	})

	t.Run("repository error propagates", func(t *testing.T) {
		repo := &fakeRateLimitRepo{
			queue: []allowResp{{err: errors.New("tasks repo down")}},
		}
		u := NewRateLimitUsecase(repo)
		if _, err := u.AllowTasks(context.Background(), 7); err == nil || err.Error() != "tasks repo down" {
			t.Fatalf("expected tasks repo down, got %v", err)
		}
	})
}

func TestRateLimitUsecase_AllowLogin_SuccessAndRepositoryErrors(t *testing.T) {
	t.Run("success picks lower remaining", func(t *testing.T) {
		repo := &fakeRateLimitRepo{
			queue: []allowResp{
				{allowed: true, tokens: 4, retryMS: 0},
				{allowed: true, tokens: 2, retryMS: 0},
			},
		}
		u := NewRateLimitUsecase(repo)
		got, err := u.AllowLogin(context.Background(), "127.0.0.1", "name@example.com")
		if err != nil {
			t.Fatalf("allow login failed: %v", err)
		}
		if !got.Allowed || got.Remaining != 2 || got.RetryAfter != 0 || got.LimitName != "login" {
			t.Fatalf("unexpected success result: %+v", got)
		}
	})

	t.Run("ip limiter error propagates", func(t *testing.T) {
		repo := &fakeRateLimitRepo{
			queue: []allowResp{{err: errors.New("ip repo down")}},
		}
		u := NewRateLimitUsecase(repo)
		if _, err := u.AllowLogin(context.Background(), "127.0.0.1", "name@example.com"); err == nil || err.Error() != "ip repo down" {
			t.Fatalf("expected ip repo down, got %v", err)
		}
	})

	t.Run("email limiter error propagates", func(t *testing.T) {
		repo := &fakeRateLimitRepo{
			queue: []allowResp{
				{allowed: true, tokens: 3, retryMS: 0},
				{err: errors.New("email repo down")},
			},
		}
		u := NewRateLimitUsecase(repo)
		if _, err := u.AllowLogin(context.Background(), "127.0.0.1", "name@example.com"); err == nil || err.Error() != "email repo down" {
			t.Fatalf("expected email repo down, got %v", err)
		}
	})
}

func TestRateLimitUsecase_RepositoryError(t *testing.T) {
	repo := &fakeRateLimitRepo{
		queue: []allowResp{{err: errors.New("redis down")}},
	}
	u := NewRateLimitUsecase(repo)
	if _, err := u.Allow(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRateLimitKeyHelpers(t *testing.T) {
	if got := rateLimitIPKey(" 127.0.0.1 "); got != "127_0_0_1" {
		t.Fatalf("unexpected ip key: %s", got)
	}
	if got := rateLimitIPKey(""); got != "unknown" {
		t.Fatalf("unexpected empty ip key: %s", got)
	}
	if got := rateLimitEmailKey(" A B@EXAMPLE.COM "); got != "ab@example.com" {
		t.Fatalf("unexpected email key: %s", got)
	}
}

