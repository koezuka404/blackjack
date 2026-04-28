package usecase

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"blackjack/backend/model"
	"blackjack/backend/repository"
)

func TestCoverage_DealerResultTimeForfeit_Branches(t *testing.T) {
	now := time.Now().UTC()
	baseSess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusDealerTurn, Version: 1, TurnSeat: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	basePlayer := &model.PlayerState{
		SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand,
		Hand: []model.StoredCard{{Rank: "10", Suit: "H"}, {Rank: "7", Suit: "C"}},
	}
	baseDealer := &model.DealerState{
		SessionID: "s1",
		Hand:      []model.StoredCard{{Rank: "9", Suit: "S"}, {Rank: "8", Suit: "D"}},
	}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("transition to result fails", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := *baseSess
		player := *basePlayer
		dealer := *baseDealer
		gameSessionTransition = func(*model.GameSession, model.SessionStatus) error { return errors.New("drtf tr1") }
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{}).(*roomService)
		if _, err := uc.dealerresultTimeForfeit(&sess, &player, &dealer, now); err == nil || err.Error() != "drtf tr1" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("transition to resetting fails", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := *baseSess
		player := *basePlayer
		dealer := *baseDealer
		calls := 0
		gameSessionTransition = func(_ *model.GameSession, _ model.SessionStatus) error {
			calls++
			if calls == 2 {
				return errors.New("drtf tr2")
			}
			return nil
		}
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{}).(*roomService)
		if _, err := uc.dealerresultTimeForfeit(&sess, &player, &dealer, now); err == nil || err.Error() != "drtf tr2" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("set outcome fails", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := *baseSess
		player := *basePlayer
		dealer := *baseDealer
		playerSetOutcomeUC = func(*model.PlayerState, int, model.Outcome) error { return errors.New("drtf outcome") }
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{}).(*roomService)
		if _, err := uc.dealerresultTimeForfeit(&sess, &player, &dealer, now); err == nil || err.Error() != "drtf outcome" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("marshal fails", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := *baseSess
		player := *basePlayer
		dealer := *baseDealer
		marshalGameJSON = func(any) ([]byte, error) { return nil, errors.New("drtf json") }
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{}).(*roomService)
		if _, err := uc.dealerresultTimeForfeit(&sess, &player, &dealer, now); err == nil || err.Error() != "drtf json" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_PlayerStand_TimeForfeitSuccess(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 2, TurnSeat: 1,
		TurnDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
	}
	player := &model.PlayerState{
		SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive,
		Hand: []model.StoredCard{{Rank: "10", Suit: "H"}, {Rank: "7", Suit: "C"}},
	}
	dealer := &model.DealerState{
		SessionID: "s1",
		Hand:      []model.StoredCard{{Rank: "9", Suit: "S"}, {Rank: "8", Suit: "D"}},
	}
	st := &authStoreStub{
		getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
		listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
		getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
			return true, nil
		},
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
	if err := uc.playerStand(context.Background(), "s1"); err != nil {
		t.Fatalf("playerStand success expected, got %v", err)
	}
	if sess.Status != model.SessionStatusResetting {
		t.Fatalf("expected resetting status after time forfeit, got %s", sess.Status)
	}
}

func TestCoverage_PlayerStand_TimeForfeitTransactionErrorBranches(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	baseRoom := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	baseSess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 2, TurnSeat: 1,
		TurnDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
	}
	basePlayer := &model.PlayerState{
		SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive,
		Hand: []model.StoredCard{{Rank: "10", Suit: "H"}, {Rank: "7", Suit: "C"}},
	}
	baseDealer := &model.DealerState{
		SessionID: "s1",
		Hand:      []model.StoredCard{{Rank: "9", Suit: "S"}, {Rank: "8", Suit: "D"}},
	}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)
	prevPolicy := os.Getenv("BLACKJACK_PLAYER_TIMEOUT_POLICY")
	_ = os.Setenv("BLACKJACK_PLAYER_TIMEOUT_POLICY", "")
	t.Cleanup(func() { _ = os.Setenv("BLACKJACK_PLAYER_TIMEOUT_POLICY", prevPolicy) })

	makeStore := func() *authStoreStub {
		room := *baseRoom
		sess := *baseSess
		player := *basePlayer
		dealer := *baseDealer
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return &sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return &room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{&player}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return &dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				return true, nil
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		return st
	}

	t.Run("create round log error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		st := makeStore()
		st.createRoundLogFn = func(context.Context, *model.RoundLog) error { return errors.New("ps create roundlog failed") }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "ps create roundlog failed" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("snapshot marshal error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		calls := 0
		marshalGameJSON = func(v any) ([]byte, error) {
			calls++
			if calls == 1 {
				return baseline.marshalGameJSON(v)
			}
			return nil, errors.New("ps snapshot marshal failed")
		}
		st := makeStore()
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "ps snapshot marshal failed" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_JoinRoom_RejoinBranches(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	makeStore := func(existing *model.RoomPlayer) *authStoreStub {
		st := &authStoreStub{
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listRoomPlayersFn:  func(context.Context, string) ([]*model.RoomPlayer, error) { return nil, nil },
			getRoomPlayerFn:    func(context.Context, string, string) (*model.RoomPlayer, error) { return existing, nil },
			updateRoomFn:       func(context.Context, *model.Room) error { return nil },
			updateRoomPlayerFn: func(context.Context, *model.RoomPlayer) error { return nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		return st
	}

	t.Run("GetRoomPlayer unexpected error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		st := &authStoreStub{
			getRoomFn:         func(context.Context, string) (*model.Room, error) { return room, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) { return nil, nil },
			getRoomPlayerFn:   func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, errors.New("join getRoomPlayer err") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if _, err := uc.JoinRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "join getRoomPlayer err" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("existing active user returns room full", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		existing := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
		st := makeStore(existing)
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if _, err := uc.JoinRoom(context.Background(), "r1", "u1"); !errors.Is(err, model.ErrRoomFull) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("set status hook error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		roomPlayerSetStatusUC = func(*model.RoomPlayer, model.RoomPlayerStatus) error { return errors.New("join set status failed") }
		existing := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerLeft, JoinedAt: now}
		st := makeStore(existing)
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if _, err := uc.JoinRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "join set status failed" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("update room player error on rejoin", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		existing := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerLeft, JoinedAt: now}
		st := makeStore(existing)
		st.updateRoomPlayerFn = func(context.Context, *model.RoomPlayer) error { return errors.New("join update room player failed") }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if _, err := uc.JoinRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "join update room player failed" {
			t.Fatalf("got %v", err)
		}
	})
}

