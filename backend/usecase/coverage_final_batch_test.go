package usecase

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"blackjack/backend/model"
	"blackjack/backend/repository"

)

func TestCoverage_initialDealFailures(t *testing.T) {
	now := time.Now().UTC()
	uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{}).(*roomService)

	t.Run("second draw exhausted", func(t *testing.T) {
		sess, _ := model.NewGameSession("sid", "r1", 1, now)
		sess.SetDeck([]model.StoredCard{{Rank: "2", Suit: "C"}})
		p, _ := model.NewPlayerState(sess.ID, "u1", 1)
		d, _ := model.NewDealerState(sess.ID)
		if err := uc.initialDeal(sess, p, d); !errors.Is(err, model.ErrDeckExhausted) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("apply fails on first player card", func(t *testing.T) {
		sess, _ := model.NewGameSession("sid", "r1", 1, now)
		sess.SetDeck([]model.StoredCard{
			{Rank: "2", Suit: "C"}, {Rank: "3", Suit: "D"}, {Rank: "4", Suit: "H"}, {Rank: "5", Suit: "S"},
		})
		p, _ := model.NewPlayerState(sess.ID, "u1", 1)
		d, _ := model.NewDealerState(sess.ID)
		uc2 := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, failingEngine{applyErr: errors.New("id1")}).(*roomService)
		if err := uc2.initialDeal(sess, p, d); err == nil || err.Error() != "id1" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_dealerresultHooks(t *testing.T) {
	now := time.Now().UTC()
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)
	uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{}).(*roomService)

	mk := func(id string) (*model.GameSession, *model.PlayerState, *model.DealerState) {
		sess, _ := model.NewGameSession(id, "r1", 1, now)
		sess.Status = model.SessionStatusDealerTurn
		p, _ := model.NewPlayerState(sess.ID, "u1", 1)
		d, _ := model.NewDealerState(sess.ID)
		return sess, p, d
	}

	t.Run("first transition error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess, p, d := mk("s1")
		gameSessionTransition = func(*model.GameSession, model.SessionStatus) error { return errors.New("dr1") }
		if _, err := uc.dealerresult(sess, p, d, now); err == nil || err.Error() != "dr1" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("SetOutcome error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		playerSetOutcomeUC = func(*model.PlayerState, int, model.Outcome) error { return errors.New("outcome") }
		s2, p2, d2 := mk("s2")
		if _, err := uc.dealerresult(s2, p2, d2, now); err == nil || err.Error() != "outcome" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("marshal error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		marshalGameJSON = func(any) ([]byte, error) { return nil, errors.New("dr json") }
		s3, p3, d3 := mk("s3")
		if _, err := uc.dealerresult(s3, p3, d3, now); err == nil || err.Error() != "dr json" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("second transition error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		def := gameSessionTransition
		calls := 0
		gameSessionTransition = func(s *model.GameSession, next model.SessionStatus) error {
			calls++
			if next == model.SessionStatusResetting {
				return errors.New("dr2")
			}
			return def(s, next)
		}
		s4, p4, d4 := mk("s4")
		if _, err := uc.dealerresult(s4, p4, d4, now); err == nil || err.Error() != "dr2" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_MarkConnectedDisconnectedSetStatusErrors(t *testing.T) {
	now := time.Now().UTC()
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("MarkConnected", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		p := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerDisconnected, JoinedAt: now}
		st := &authStoreStub{getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return p, nil }}
		roomPlayerSetStatusUC = func(*model.RoomPlayer, model.RoomPlayerStatus) error { return errors.New("mc set") }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if err := uc.MarkConnected(context.Background(), "r1", "u1"); err == nil || err.Error() != "mc set" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("MarkDisconnected", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		p := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
		st := &authStoreStub{getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return p, nil }}
		roomPlayerSetStatusUC = func(*model.RoomPlayer, model.RoomPlayerStatus) error { return errors.New("md set") }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if err := uc.MarkDisconnected(context.Background(), "r1", "u1"); err == nil || err.Error() != "md set" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_processRematchDeadlineMore(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(time.Hour)
	past := now.Add(-2 * time.Second)
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("deadline not yet due", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 1,
			RematchDeadlineAt: &future, CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil }}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.processRematchDeadline(context.Background(), "s1"); err != nil {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("list room players error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 1,
			RematchDeadlineAt: &past, CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return nil, errors.New("pr list rp")
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.processRematchDeadline(context.Background(), "s1"); err == nil || err.Error() != "pr list rp" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("finalize when single eligible did not agree", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 1,
			RematchDeadlineAt: &past, CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return []*model.RoomPlayer{{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}}, nil
			},
			listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
			updateRoomFn:       func(context.Context, *model.Room) error { return nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.processRematchDeadline(context.Background(), "s1"); err != nil {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_playerStandMoreBranches(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: ptrString("s1"), CreatedAt: now, UpdatedAt: now}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("wrong session status no-op", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1, TurnSeat: 1,
			TurnDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil }}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err != nil {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("deadline still in future no-op", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			TurnDeadlineAt: ptrTime(now.Add(time.Hour)), CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil }}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err != nil {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("get room error", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			TurnDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return nil, errors.New("ps room") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "ps room" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("list player states error", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			TurnDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return nil, errors.New("ps list") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "ps list" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("stand SetStatus and Transition and recalc errors", func(t *testing.T) {
		makeStandUC := func() (*roomService, *model.GameSession, *model.PlayerState, *model.DealerState) {
			sess := &model.GameSession{
				ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
				TurnDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
			}
			player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "8", Suit: "H"}}}
			dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "7", Suit: "D"}}}
			st := &authStoreStub{
				getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
				getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
				listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
				getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
				updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			}
			st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
			uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
			return uc, sess, player, dealer
		}

		restoreUsecaseHooks(baseline)
		playerStateSetStatus = func(*model.PlayerState, model.PlayerStatus) error { return errors.New("ps set") }
		uc1, _, _, _ := makeStandUC()
		if err := uc1.playerStand(context.Background(), "s1"); err == nil || err.Error() != "ps set" {
			t.Fatalf("got %v", err)
		}

		restoreUsecaseHooks(baseline)
		gameSessionTransition = func(*model.GameSession, model.SessionStatus) error { return errors.New("ps gt") }
		uc2, _, _, _ := makeStandUC()
		if err := uc2.playerStand(context.Background(), "s1"); err == nil || err.Error() != "ps gt" {
			t.Fatalf("got %v", err)
		}

		restoreUsecaseHooks(baseline)
		roomRecalculateStatus = func(*model.Room, int, bool) error { return errors.New("ps rc") }
		uc3, _, _, _ := makeStandUC()
		if err := uc3.playerStand(context.Background(), "s1"); err == nil || err.Error() != "ps rc" {
			t.Fatalf("got %v", err)
		}

		restoreUsecaseHooks(baseline)
		marshalGameJSON = func(any) ([]byte, error) { return nil, errors.New("ps json") }
		uc4, _, _, _ := makeStandUC()
		if err := uc4.playerStand(context.Background(), "s1"); err == nil || err.Error() != "ps json" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_dealerTurnMoreBranches(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("get session error", func(t *testing.T) {
		st := &authStoreStub{getSessionFn: func(context.Context, string) (*model.GameSession, error) { return nil, errors.New("dt gs") }}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt gs" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("wrong status no-op", func(t *testing.T) {
		sess := &model.GameSession{ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil }}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err != nil {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("get room error", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return nil, errors.New("dt gr") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt gr" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("list players error", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return nil, errors.New("dt lp") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt lp" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("get dealer error", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return nil, errors.New("dt gd") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{value: 16}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt gd" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("dealerresult error on terminal", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{}, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "D"}, {Rank: "7", Suit: "S"}}}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{bust: true}, appendEngine{}).(*roomService)
		gameSessionTransition = func(*model.GameSession, model.SessionStatus) error { return errors.New("dt dr") }
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt dr" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("terminal txn update player error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{}, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "D"}, {Rank: "7", Suit: "S"}}}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			updatePlayerStateFn:      func(context.Context, *model.PlayerState) error { return errors.New("dt up") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{bust: true}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt up" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("terminal createRoundLog error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{}, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "D"}, {Rank: "7", Suit: "S"}}}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			updatePlayerStateFn:      func(context.Context, *model.PlayerState) error { return nil },
			updateDealerStateFn:      func(context.Context, *model.DealerState) error { return nil },
			createRoundLogFn:         func(context.Context, *model.RoundLog) error { return errors.New("dt rl") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{bust: true}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt rl" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("terminal update room error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{}, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "D"}, {Rank: "7", Suit: "S"}}}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			updatePlayerStateFn:      func(context.Context, *model.PlayerState) error { return nil },
			updateDealerStateFn:      func(context.Context, *model.DealerState) error { return nil },
			createRoundLogFn:         func(context.Context, *model.RoundLog) error { return nil },
			updateRoomFn:             func(context.Context, *model.Room) error { return errors.New("dt ur") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{bust: true}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt ur" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("draw path errors", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "5", Suit: "D"}, {Rank: "6", Suit: "S"}}}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{value: 11}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); !errors.Is(err, model.ErrDeckExhausted) {
			t.Fatalf("draw draw: got %v", err)
		}

		sess2 := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player2 := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		dealer2 := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "5", Suit: "D"}, {Rank: "6", Suit: "S"}}}
		st2 := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess2, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player2}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer2, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			updateDealerStateFn:      func(context.Context, *model.DealerState) error { return nil },
		}
		st2.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st2) }
		uc2 := NewRoomUsecase(st2, fixedEvaluator{value: 11}, failingEngine{applyErr: errors.New("dt draw apply")}).(*roomService)
		if err := uc2.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt draw apply" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_newShuffledDeckShuffleIntnError(t *testing.T) {
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)
	shuffleIntn = func(*big.Int) (*big.Int, error) { return nil, errors.New("rand") }
	d := newShuffledDeck()
	if len(d) != 52 {
		t.Fatalf("expected 52 cards, got %d", len(d))
	}
}

type failNthApplyEngine struct {
	n   int
	cur int
	err error
}

func (e *failNthApplyEngine) ApplyPlayerHit(hand []model.StoredCard, draw model.StoredCard) ([]model.StoredCard, error) {
	e.cur++
	if e.cur == e.n {
		return nil, e.err
	}
	return append(append([]model.StoredCard{}, hand...), draw), nil
}

func (e *failNthApplyEngine) ResolveOutcome(model.HandEvaluator, []model.StoredCard, []model.StoredCard) (model.Outcome, error) {
	return model.OutcomePush, nil
}

func TestCoverage_PlayerTurnBlackjackHookErrors(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	makeHitUC := func() (RoomUsecase, *authStoreStub) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "6", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "5", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "7", Suit: "D"}}}
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 11, blackjack: true}, appendEngine{})
		return uc, st
	}

	t.Run("blackjack SetStatus error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		playerStateSetStatus = func(*model.PlayerState, model.PlayerStatus) error { return errors.New("hit bj set") }
		uc, _ := makeHitUC()
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "hit-bj-set"); err == nil || err.Error() != "hit bj set" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("blackjack transition error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		gameSessionTransition = func(*model.GameSession, model.SessionStatus) error { return errors.New("hit bj tx") }
		uc, _ := makeHitUC()
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "hit-bj-tx"); err == nil || err.Error() != "hit bj tx" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_initialDealMoreDrawAndApplyErrors(t *testing.T) {
	now := time.Now().UTC()
	uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{}).(*roomService)

	t.Run("invalid first draw", func(t *testing.T) {
		sess, _ := model.NewGameSession("sid", "r1", 1, now)
		sess.SetDeck([]model.StoredCard{{Rank: "2", Suit: "C"}})
		sess.DrawIndex = 5
		p, _ := model.NewPlayerState(sess.ID, "u1", 1)
		d, _ := model.NewDealerState(sess.ID)
		if err := uc.initialDeal(sess, p, d); !errors.Is(err, model.ErrInvalidDeck) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("apply fail on dealer first hole", func(t *testing.T) {
		sess, _ := model.NewGameSession("sid", "r1", 1, now)
		sess.SetDeck([]model.StoredCard{
			{Rank: "2", Suit: "C"}, {Rank: "3", Suit: "D"}, {Rank: "4", Suit: "H"}, {Rank: "5", Suit: "S"},
		})
		p, _ := model.NewPlayerState(sess.ID, "u1", 1)
		d, _ := model.NewDealerState(sess.ID)
		uc2 := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, &failNthApplyEngine{n: 2, err: errors.New("id2")}).(*roomService)
		if err := uc2.initialDeal(sess, p, d); err == nil || err.Error() != "id2" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("third draw exhausted", func(t *testing.T) {
		sess, _ := model.NewGameSession("sid", "r1", 1, now)
		sess.SetDeck([]model.StoredCard{{Rank: "2", Suit: "C"}, {Rank: "3", Suit: "D"}})
		p, _ := model.NewPlayerState(sess.ID, "u1", 1)
		d, _ := model.NewDealerState(sess.ID)
		if err := uc.initialDeal(sess, p, d); !errors.Is(err, model.ErrDeckExhausted) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("apply fail on second player card", func(t *testing.T) {
		sess, _ := model.NewGameSession("sid", "r1", 1, now)
		sess.SetDeck([]model.StoredCard{
			{Rank: "2", Suit: "C"}, {Rank: "3", Suit: "D"}, {Rank: "4", Suit: "H"}, {Rank: "5", Suit: "S"},
		})
		p, _ := model.NewPlayerState(sess.ID, "u1", 1)
		d, _ := model.NewDealerState(sess.ID)
		uc3 := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, &failNthApplyEngine{n: 3, err: errors.New("id3")}).(*roomService)
		if err := uc3.initialDeal(sess, p, d); err == nil || err.Error() != "id3" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("apply fail on second dealer upcard", func(t *testing.T) {
		sess, _ := model.NewGameSession("sid", "r1", 1, now)
		sess.SetDeck([]model.StoredCard{
			{Rank: "2", Suit: "C"}, {Rank: "3", Suit: "D"}, {Rank: "4", Suit: "H"}, {Rank: "5", Suit: "S"},
		})
		p, _ := model.NewPlayerState(sess.ID, "u1", 1)
		d, _ := model.NewDealerState(sess.ID)
		uc4 := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, &failNthApplyEngine{n: 4, err: errors.New("id4")}).(*roomService)
		if err := uc4.initialDeal(sess, p, d); err == nil || err.Error() != "id4" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_playerStandUpdateSessionRepoError(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: ptrString("s1"), CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
		TurnDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
	}
	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "8", Suit: "H"}}}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "7", Suit: "D"}}}
	st := &authStoreStub{
		getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
		listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
		getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
			return false, errors.New("ps us repo")
		},
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
	if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "ps us repo" {
		t.Fatalf("got %v", err)
	}
}

func TestCoverage_dealerTurnRecalcAndUpdateDealerErrors(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("terminal room recalc error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{}, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "D"}, {Rank: "7", Suit: "S"}}}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		roomRecalculateStatus = func(*model.Room, int, bool) error { return errors.New("dt rc") }
		uc := NewRoomUsecase(st, fixedEvaluator{bust: true}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt rc" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("draw path update dealer error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusDealerTurn, Version: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "5", Suit: "D"}, {Rank: "6", Suit: "S"}}}
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			updateDealerStateFn:      func(context.Context, *model.DealerState) error { return errors.New("dt ud") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 11}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "dt ud" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_authHooks(t *testing.T) {
	t.Run("Signup bcrypt error", func(t *testing.T) {
		prev := signupHashPassword
		signupHashPassword = func([]byte, int) ([]byte, error) { return nil, errors.New("bcrypt down") }
		t.Cleanup(func() { signupHashPassword = prev })
		uc := NewAuthUsecase(&authStoreStub{}, []byte("this-is-a-very-long-secret"))
		if _, err := uc.Signup(context.Background(), "gooduser", "good@example.com", "password12"); err == nil || err.Error() != "bcrypt down" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("Login GetUserByUsername error", func(t *testing.T) {
		st := &authStoreStub{
			getUserByUsernameFn: func(context.Context, string) (*model.User, error) { return nil, errors.New("lookup failed") },
		}
		uc := NewAuthUsecase(st, []byte("this-is-a-very-long-secret"))
		if _, err := uc.Login(context.Background(), "any", "password12"); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("got %v", err)
		}
	})
}
