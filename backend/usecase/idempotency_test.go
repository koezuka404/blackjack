package usecase

import (
	"context"
	"errors"
	"testing"

	"blackjack/backend/model"
	"blackjack/backend/repository"
)

func TestEnsureActionIdempotency(t *testing.T) {
	base := &model.ActionLog{
		SessionID:          "s1",
		ActorType:          model.ActorTypeUser,
		ActorUserID:        "u1",
		TargetUserID:       "u1",
		ActionID:           "a1",
		RequestType:        "HIT",
		RequestPayloadHash: "hash1",
	}

	t.Run("invalid action log", func(t *testing.T) {
		_, _, err := EnsureActionIdempotency(context.Background(), &authStoreStub{}, &model.ActionLog{})
		if err == nil {
			t.Fatal("expected validation error")
		}
	})

	t.Run("first request not replay", func(t *testing.T) {
		store := &authStoreStub{
			getActionLogByIDFn: func(context.Context, string, string, string) (*model.ActionLog, error) {
				return nil, repository.ErrNotFound
			},
		}
		snap, replay, err := EnsureActionIdempotency(context.Background(), store, base)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if replay || snap != "" {
			t.Fatalf("unexpected replay result: replay=%v snap=%q", replay, snap)
		}
	})

	t.Run("replay with same hash", func(t *testing.T) {
		store := &authStoreStub{
			getActionLogByIDFn: func(context.Context, string, string, string) (*model.ActionLog, error) {
				return &model.ActionLog{
					RequestPayloadHash: "hash1",
					ResponseSnapshot:   `{"ok":true}`,
				}, nil
			},
		}
		snap, replay, err := EnsureActionIdempotency(context.Background(), store, base)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !replay || snap == "" {
			t.Fatalf("expected replay with snapshot: replay=%v snap=%q", replay, snap)
		}
	})

	t.Run("duplicate action with different hash", func(t *testing.T) {
		store := &authStoreStub{
			getActionLogByIDFn: func(context.Context, string, string, string) (*model.ActionLog, error) {
				return &model.ActionLog{RequestPayloadHash: "other"}, nil
			},
		}
		_, _, err := EnsureActionIdempotency(context.Background(), store, base)
		if !errors.Is(err, model.ErrDuplicateAction) {
			t.Fatalf("expected duplicate action, got: %v", err)
		}
	})

	t.Run("store error", func(t *testing.T) {
		store := &authStoreStub{
			getActionLogByIDFn: func(context.Context, string, string, string) (*model.ActionLog, error) {
				return nil, errors.New("db down")
			},
		}
		_, _, err := EnsureActionIdempotency(context.Background(), store, base)
		if err == nil {
			t.Fatal("expected store error")
		}
	})
}

func TestSaveActionSuccessSnapshot(t *testing.T) {
	called := false
	store := &authStoreStub{
		createActionLogFn: func(_ context.Context, actionLog *model.ActionLog) error {
			called = true
			if actionLog.ResponseSnapshot != `{"ok":true}` {
				t.Fatalf("unexpected snapshot: %s", actionLog.ResponseSnapshot)
			}
			return nil
		},
	}
	action := &model.ActionLog{
		SessionID:          "s1",
		ActorType:          model.ActorTypeUser,
		ActorUserID:        "u1",
		TargetUserID:       "u1",
		ActionID:           "a1",
		RequestType:        "HIT",
		RequestPayloadHash: "hash1",
	}
	if err := SaveActionSuccessSnapshot(context.Background(), store, action, `{"ok":true}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected CreateActionLog call")
	}
}

