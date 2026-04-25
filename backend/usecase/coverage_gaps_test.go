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

type savedUsecaseHooks struct {
	shuffleIntn            func(*big.Int) (*big.Int, error)
	marshalGameJSON        func(any) ([]byte, error)
	newRoomForCreate       func(string, string, time.Time) (*model.Room, error)
	newRoomPlayerForJoin   func(string, string, int, time.Time) (*model.RoomPlayer, error)
	newGameSessionUC       func(string, string, int, time.Time) (*model.GameSession, error)
	newDealerStateUC       func(string) (*model.DealerState, error)
	newPlayerStateUC       func(string, string, int) (*model.PlayerState, error)
	roomRecalculateStatus  func(*model.Room, int, bool) error
	gameSessionTransition  func(*model.GameSession, model.SessionStatus) error
	playerStateSetStatus   func(*model.PlayerState, model.PlayerStatus) error
	roomPlayerSetStatusUC  func(*model.RoomPlayer, model.RoomPlayerStatus) error
	playerSetOutcomeUC    func(*model.PlayerState, int, model.Outcome) error
}

func captureUsecaseHooks() savedUsecaseHooks {
	return savedUsecaseHooks{
		shuffleIntn:            shuffleIntn,
		marshalGameJSON:        marshalGameJSON,
		newRoomForCreate:       newRoomForCreate,
		newRoomPlayerForJoin:   newRoomPlayerForJoin,
		newGameSessionUC:       newGameSessionUC,
		newDealerStateUC:       newDealerStateUC,
		newPlayerStateUC:       newPlayerStateUC,
		roomRecalculateStatus:  roomRecalculateStatus,
		gameSessionTransition:  gameSessionTransition,
		playerStateSetStatus:   playerStateSetStatus,
		roomPlayerSetStatusUC:  roomPlayerSetStatusUC,
		playerSetOutcomeUC:     playerSetOutcomeUC,
	}
}

func restoreUsecaseHooks(h savedUsecaseHooks) {
	shuffleIntn = h.shuffleIntn
	marshalGameJSON = h.marshalGameJSON
	newRoomForCreate = h.newRoomForCreate
	newRoomPlayerForJoin = h.newRoomPlayerForJoin
	newGameSessionUC = h.newGameSessionUC
	newDealerStateUC = h.newDealerStateUC
	newPlayerStateUC = h.newPlayerStateUC
	roomRecalculateStatus = h.roomRecalculateStatus
	gameSessionTransition = h.gameSessionTransition
	playerStateSetStatus = h.playerStateSetStatus
	roomPlayerSetStatusUC = h.roomPlayerSetStatusUC
	playerSetOutcomeUC = h.playerSetOutcomeUC
}

func withUsecaseHookRestore(t *testing.T) {
	t.Helper()
	h := captureUsecaseHooks()
	t.Cleanup(func() { restoreUsecaseHooks(h) })
}

func TestCoverage_CreateRoomAndJoinRoomFactories(t *testing.T) {
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("CreateRoom NewRoom error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		newRoomForCreate = func(string, string, time.Time) (*model.Room, error) {
			return nil, errors.New("new room failed")
		}
		uc := NewRoomUsecase(&authStoreStub{}, nil, nil)
		if _, err := uc.CreateRoom(context.Background(), "u1"); err == nil || err.Error() != "new room failed" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("CreateRoom RecalculateStatus error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		room, _ := model.NewRoom("rid", "u1", time.Now().UTC())
		newRoomForCreate = func(string, string, time.Time) (*model.Room, error) { return room, nil }
		roomRecalculateStatus = func(*model.Room, int, bool) error { return errors.New("recalc failed") }
		uc := NewRoomUsecase(&authStoreStub{}, nil, nil)
		if _, err := uc.CreateRoom(context.Background(), "u1"); err == nil || err.Error() != "recalc failed" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("JoinRoom NewRoomPlayer error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		now := time.Now().UTC()
		base := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return base, nil }}
		newRoomPlayerForJoin = func(string, string, int, time.Time) (*model.RoomPlayer, error) {
			return nil, errors.New("new room player failed")
		}
		uc := NewRoomUsecase(st, nil, nil)
		if _, err := uc.JoinRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "new room player failed" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("JoinRoom RecalculateStatus error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		now := time.Now().UTC()
		base := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{
			getRoomFn:         func(context.Context, string) (*model.Room, error) { return base, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) { return nil, nil },
		}
		roomRecalculateStatus = func(*model.Room, int, bool) error { return errors.New("join recalc failed") }
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, nil, nil)
		if _, err := uc.JoinRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "join recalc failed" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("JoinRoom success returns room", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		now := time.Now().UTC()
		base := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{
			getRoomFn:         func(context.Context, string) (*model.Room, error) { return base, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) { return nil, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, nil, nil)
		got, err := uc.JoinRoom(context.Background(), "r1", "u1")
		if err != nil || got == nil || got.ID != "r1" {
			t.Fatalf("join success: room=%+v err=%v", got, err)
		}
	})
}

func TestCoverage_GetRoomAndLeaveRoomBranches(t *testing.T) {
	now := time.Now().UTC()
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("GetRoom store error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("no room") }}
		uc := NewRoomUsecase(st, nil, nil)
		if _, _, err := uc.GetRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "no room" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("GetRoom session non-NotFound error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sid := "s1"
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return nil, errors.New("session db") },
		}
		uc := NewRoomUsecase(st, nil, nil)
		if _, _, err := uc.GetRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "session db" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("LeaveRoom get room error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("leave get room") }}
		uc := NewRoomUsecase(st, nil, nil)
		if _, _, err := uc.LeaveRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "leave get room" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("LeaveRoom already left returns without transaction", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}
		leftP := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerLeft, JoinedAt: now}
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return leftP, nil },
		}
		uc := NewRoomUsecase(st, nil, nil)
		gotRoom, tr, err := uc.LeaveRoom(context.Background(), "r1", "u1")
		if err != nil || tr != nil || gotRoom == nil {
			t.Fatalf("expected early leave ok, got room=%v tr=%v err=%v", gotRoom, tr, err)
		}
	})

	t.Run("LeaveRoom host leaves with two other actives yields ErrRoomFull on recalc", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}
		hostP := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return hostP, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return []*model.RoomPlayer{
					{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now},
					{RoomID: "r1", UserID: "u2", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now},
					{RoomID: "r1", UserID: "u3", SeatNo: 3, Status: model.RoomPlayerActive, JoinedAt: now},
				}, nil
			},
		}
		uc := NewRoomUsecase(st, nil, nil)
		if _, _, err := uc.LeaveRoom(context.Background(), "r1", "u1"); !errors.Is(err, model.ErrRoomFull) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_SelectNextHostSameSeatTieBreak(t *testing.T) {
	now := time.Now().UTC()
	players := []*model.RoomPlayer{
		{UserID: "u_b", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now},
		{UserID: "u_a", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now},
	}
	next := selectNextHost(players, "leaver")
	if next == nil || next.UserID != "u_a" {
		t.Fatalf("expected u_a (lexicographic tie-break), got %+v", next)
	}
}

func TestCoverage_ResetRoomForDebugSuccessAndRecalcError(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("RecalculateStatus error after deletes", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return room, nil }}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		roomRecalculateStatus = func(*model.Room, int, bool) error { return errors.New("reset recalc") }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.ResetRoomForDebug(context.Background(), "r1", "u1"); err == nil || err.Error() != "reset recalc" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("full success returns room", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		local := *room
		st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return &local, nil }}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		out, err := uc.ResetRoomForDebug(context.Background(), "r1", "u1")
		if err != nil || out == nil || out.CurrentSessionID != nil {
			t.Fatalf("expected cleared room, out=%+v err=%v", out, err)
		}
	})
}

func TestCoverage_StartRoomModelErrors(t *testing.T) {
	now := time.Now().UTC()
	baseSt := func() *authStoreStub {
		localRoom := model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
		localPlayer := model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
		return &authStoreStub{
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
			getRoomPlayerFn:    func(context.Context, string, string) (*model.RoomPlayer, error) { return &localPlayer, nil },
			getLatestSessionFn: func(context.Context, string) (*model.GameSession, error) { return nil, repository.ErrNotFound },
		}
	}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("NewGameSession error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		newGameSessionUC = func(string, string, int, time.Time) (*model.GameSession, error) {
			return nil, errors.New("ngs")
		}
		uc := NewRoomUsecase(baseSt(), fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "ngs" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("NewDealerState error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		newDealerStateUC = func(string) (*model.DealerState, error) { return nil, errors.New("nds") }
		uc := NewRoomUsecase(baseSt(), fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "nds" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("NewPlayerState error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		newPlayerStateUC = func(string, string, int) (*model.PlayerState, error) { return nil, errors.New("nps") }
		uc := NewRoomUsecase(baseSt(), fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "nps" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("initial deal engine error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		uc := NewRoomUsecase(baseSt(), fixedEvaluator{}, failingEngine{applyErr: errors.New("deal apply")})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "deal apply" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("Transition to player turn error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		gameSessionTransition = func(*model.GameSession, model.SessionStatus) error { return errors.New("tpt") }
		uc := NewRoomUsecase(baseSt(), fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "tpt" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("StartRoom blackjack branch SetStatus error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		def := playerStateSetStatus
		playerStateSetStatus = func(p *model.PlayerState, st model.PlayerStatus) error {
			if st == model.PlayerStatusBlackjack {
				return errors.New("bj set failed")
			}
			return def(p, st)
		}
		st := baseSt()
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{blackjack: true}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "bj set failed" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("StartRoom room RecalculateStatus error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		roomRecalculateStatus = func(*model.Room, int, bool) error { return errors.New("start room recalc") }
		st := baseSt()
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "start room recalc" {
			t.Fatalf("got %v", err)
		}
	})
}
