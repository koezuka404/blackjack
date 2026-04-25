package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	"blackjack/backend/model"
	"blackjack/backend/repository"
)

type fixedEvaluator struct {
	value      int
	blackjack  bool
	bust       bool
	soft       bool
}

func (f fixedEvaluator) Value([]model.StoredCard) int      { return f.value }
func (f fixedEvaluator) IsBlackjack([]model.StoredCard) bool { return f.blackjack }
func (f fixedEvaluator) IsBust([]model.StoredCard) bool    { return f.bust }
func (f fixedEvaluator) IsSoft([]model.StoredCard) bool    { return f.soft }

type appendEngine struct{}

func (appendEngine) ApplyPlayerHit(hand []model.StoredCard, draw model.StoredCard) ([]model.StoredCard, error) {
	return append(append([]model.StoredCard{}, hand...), draw), nil
}
func (appendEngine) ResolveOutcome(model.HandEvaluator, []model.StoredCard, []model.StoredCard) (model.Outcome, error) {
	return model.OutcomePush, nil
}

type failingEngine struct {
	applyErr   error
	resolveErr error
}

func (f failingEngine) ApplyPlayerHit(hand []model.StoredCard, draw model.StoredCard) ([]model.StoredCard, error) {
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	return append(append([]model.StoredCard{}, hand...), draw), nil
}

func (f failingEngine) ResolveOutcome(model.HandEvaluator, []model.StoredCard, []model.StoredCard) (model.Outcome, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	return model.OutcomePush, nil
}

func TestRoomUsecase_CreateRoom(t *testing.T) {
	uc := NewRoomUsecase(&authStoreStub{}, nil, nil)
	if _, err := uc.CreateRoom(context.Background(), ""); !errors.Is(err, ErrUnauthorizedUser) {
		t.Fatalf("expected unauthorized, got %v", err)
	}

	st := &authStoreStub{}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error {
		return fn(st)
	}
	uc = NewRoomUsecase(st, nil, nil)
	room, err := uc.CreateRoom(context.Background(), "u1")
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}
	if room.HostUserID != "u1" || room.Status != model.RoomStatusWaiting {
		t.Fatalf("unexpected room: %+v", room)
	}
}

func TestRoomUsecase_CreateRoom_TransactionFails(t *testing.T) {
	st := &authStoreStub{
		createRoomFn: func(context.Context, *model.Room) error { return errors.New("create room persist failed") },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, nil, nil)
	if _, err := uc.CreateRoom(context.Background(), "u1"); err == nil || err.Error() != "create room persist failed" {
		t.Fatalf("expected create room persist failed, got %v", err)
	}
}

func TestRoomUsecase_JoinRoom(t *testing.T) {
	now := time.Now().UTC()
	baseRoom := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}

	st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return baseRoom, nil }}
	uc := NewRoomUsecase(st, nil, nil)
	if _, err := uc.JoinRoom(context.Background(), "r1", ""); !errors.Is(err, ErrUnauthorizedUser) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	if _, err := uc.JoinRoom(context.Background(), "", "u1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}

	st2 := &authStoreStub{
		getRoomFn:         func(context.Context, string) (*model.Room, error) { return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying}, nil },
		listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) { return nil, nil },
	}
	uc2 := NewRoomUsecase(st2, nil, nil)
	if _, err := uc2.JoinRoom(context.Background(), "r1", "u1"); !errors.Is(err, ErrInvalidGameState) {
		t.Fatalf("expected invalid state, got %v", err)
	}

	st3 := &authStoreStub{
		getRoomFn: func(context.Context, string) (*model.Room, error) { return baseRoom, nil },
	}
	uc3 := NewRoomUsecase(st3, nil, nil)
	if _, err := uc3.JoinRoom(context.Background(), "r1", "u2"); !errors.Is(err, ErrForbiddenAction) {
		t.Fatalf("expected forbidden, got %v", err)
	}

	st4 := &authStoreStub{
		getRoomFn: func(context.Context, string) (*model.Room, error) { return baseRoom, nil },
		listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
			return []*model.RoomPlayer{{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive}}, nil
		},
	}
	uc4 := NewRoomUsecase(st4, nil, nil)
	if _, err := uc4.JoinRoom(context.Background(), "r1", "u1"); !errors.Is(err, model.ErrRoomFull) {
		t.Fatalf("expected room full, got %v", err)
	}

	st5 := &authStoreStub{
		getRoomFn: func(context.Context, string) (*model.Room, error) { return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}, nil },
		listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
			return []*model.RoomPlayer{}, nil
		},
		createRoomPlayerFn: func(context.Context, *model.RoomPlayer) error { return repository.ErrAlreadyExists },
	}
	st5.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st5) }
	uc5 := NewRoomUsecase(st5, nil, nil)
	if _, err := uc5.JoinRoom(context.Background(), "r1", "u1"); !errors.Is(err, model.ErrRoomFull) {
		t.Fatalf("expected mapped room full, got %v", err)
	}

	st6 := &authStoreStub{
		getRoomFn: func(context.Context, string) (*model.Room, error) {
			return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}, nil
		},
		listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) { return nil, nil },
		createRoomPlayerFn: func(context.Context, *model.RoomPlayer) error {
			return errors.New("create room player persist failed")
		},
	}
	st6.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st6) }
	uc6 := NewRoomUsecase(st6, nil, nil)
	if _, err := uc6.JoinRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "create room player persist failed" {
		t.Fatalf("expected create room player persist failed, got %v", err)
	}
}

func TestRoomUsecase_JoinRoom_RepositoryErrors(t *testing.T) {
	now := time.Now().UTC()
	baseRoom := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}

	t.Run("get room error", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("room load failed") },
		}
		uc := NewRoomUsecase(st, nil, nil)
		if _, err := uc.JoinRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "room load failed" {
			t.Fatalf("expected room load failed, got %v", err)
		}
	})

	t.Run("list room players error", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:         func(context.Context, string) (*model.Room, error) { return baseRoom, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) { return nil, errors.New("list players failed") },
		}
		uc := NewRoomUsecase(st, nil, nil)
		if _, err := uc.JoinRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "list players failed" {
			t.Fatalf("expected list players failed, got %v", err)
		}
	})

	t.Run("update room after join fails", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return baseRoom, nil },
			listRoomPlayersFn:  func(context.Context, string) ([]*model.RoomPlayer, error) { return nil, nil },
			createRoomPlayerFn: func(context.Context, *model.RoomPlayer) error { return nil },
			updateRoomFn:       func(context.Context, *model.Room) error { return errors.New("update room after join failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, nil, nil)
		if _, err := uc.JoinRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "update room after join failed" {
			t.Fatalf("expected update room after join failed, got %v", err)
		}
	})
}

func TestRoomUsecase_GetRoomAndListRooms(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}

	st := &authStoreStub{
		getRoomFn: func(context.Context, string) (*model.Room, error) { return room, nil },
	}
	uc := NewRoomUsecase(st, nil, nil)
	if _, _, err := uc.GetRoom(context.Background(), "r1", ""); !errors.Is(err, ErrUnauthorizedUser) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	if _, _, err := uc.GetRoom(context.Background(), "", "u1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}

	// Non-host + no membership should be forbidden.
	st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, repository.ErrNotFound }
	if _, _, err := uc.GetRoom(context.Background(), "r1", "u2"); !errors.Is(err, ErrForbiddenAction) {
		t.Fatalf("expected forbidden, got %v", err)
	}

	// Session pointer exists, but missing session row is tolerated.
	sid := "s1"
	roomWithSession := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	st2 := &authStoreStub{
		getRoomFn:    func(context.Context, string) (*model.Room, error) { return roomWithSession, nil },
		getSessionFn: func(context.Context, string) (*model.GameSession, error) { return nil, repository.ErrNotFound },
	}
	uc2 := NewRoomUsecase(st2, nil, nil)
	gotRoom, sess, err := uc2.GetRoom(context.Background(), "r1", "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotRoom == nil || sess != nil {
		t.Fatalf("unexpected room/session: room=%+v sess=%+v", gotRoom, sess)
	}

	if _, err := uc2.ListRooms(context.Background(), ""); !errors.Is(err, ErrUnauthorizedUser) {
		t.Fatalf("expected unauthorized list, got %v", err)
	}
	st2.listRoomsByUserIDFn = func(context.Context, string) ([]*model.Room, error) {
		return []*model.Room{room}, nil
	}
	rooms, err := uc2.ListRooms(context.Background(), "u1")
	if err != nil || len(rooms) != 1 {
		t.Fatalf("unexpected list result: rooms=%d err=%v", len(rooms), err)
	}

	// Non-host member with LEFT status should be forbidden.
	st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) {
		return &model.RoomPlayer{RoomID: "r1", UserID: "u2", SeatNo: 1, Status: model.RoomPlayerLeft, JoinedAt: now}, nil
	}
	if _, _, err := uc.GetRoom(context.Background(), "r1", "u2"); !errors.Is(err, ErrForbiddenAction) {
		t.Fatalf("expected forbidden for left membership, got %v", err)
	}

	// Non-host active membership can read room.
	st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) {
		return &model.RoomPlayer{RoomID: "r1", UserID: "u2", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}, nil
	}
	if got, sess3, err := uc.GetRoom(context.Background(), "r1", "u2"); err != nil || got == nil || sess3 != nil {
		t.Fatalf("expected readable room for active member: room=%+v sess=%+v err=%v", got, sess3, err)
	}

	st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) {
		return nil, errors.New("get room member db failed")
	}
	if _, _, err := uc.GetRoom(context.Background(), "r1", "u2"); err == nil || err.Error() != "get room member db failed" {
		t.Fatalf("expected get room member db failed, got %v", err)
	}
}

func TestRoomUsecase_LeaveRoom(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
	player := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}

	st := &authStoreStub{
		getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
		getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil },
		listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
			return []*model.RoomPlayer{
				player,
				{RoomID: "r1", UserID: "u2", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now},
			}, nil
		},
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, nil, nil)

	if _, _, err := uc.LeaveRoom(context.Background(), "r1", ""); !errors.Is(err, ErrUnauthorizedUser) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	if _, _, err := uc.LeaveRoom(context.Background(), "", "u1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}

	roomWithSession := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: ptrString("s1"), CreatedAt: now, UpdatedAt: now}
	st.getRoomFn = func(context.Context, string) (*model.Room, error) { return roomWithSession, nil }
	if _, _, err := uc.LeaveRoom(context.Background(), "r1", "u1"); !errors.Is(err, ErrInvalidGameState) {
		t.Fatalf("expected invalid game state, got %v", err)
	}

	st.getRoomFn = func(context.Context, string) (*model.Room, error) { return room, nil }
	updated, transfer, err := uc.LeaveRoom(context.Background(), "r1", "u1")
	if err != nil {
		t.Fatalf("leave room failed: %v", err)
	}
	if updated == nil || transfer == nil || transfer.NewHostUserID != "u2" {
		t.Fatalf("unexpected leave result: room=%+v transfer=%+v", updated, transfer)
	}
}

func TestRoomUsecase_LeaveRoom_TransactionErrors(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
	activePlayer := func() *model.RoomPlayer {
		return &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
	}

	t.Run("get room player error", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, errors.New("get leaver failed") },
		}
		uc := NewRoomUsecase(st, nil, nil)
		if _, _, err := uc.LeaveRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "get leaver failed" {
			t.Fatalf("expected get leaver failed, got %v", err)
		}
	})

	t.Run("list room players error", func(t *testing.T) {
		player := activePlayer()
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return nil, errors.New("list for leave failed")
			},
		}
		uc := NewRoomUsecase(st, nil, nil)
		if _, _, err := uc.LeaveRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "list for leave failed" {
			t.Fatalf("expected list for leave failed, got %v", err)
		}
	})

	t.Run("update room player error", func(t *testing.T) {
		player := activePlayer()
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return []*model.RoomPlayer{player}, nil
			},
			updateRoomPlayerFn: func(context.Context, *model.RoomPlayer) error { return errors.New("update leaver failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, nil, nil)
		if _, _, err := uc.LeaveRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "update leaver failed" {
			t.Fatalf("expected update leaver failed, got %v", err)
		}
	})

	t.Run("update room error", func(t *testing.T) {
		player := activePlayer()
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return []*model.RoomPlayer{player}, nil
			},
			updateRoomPlayerFn: func(context.Context, *model.RoomPlayer) error { return nil },
			updateRoomFn:       func(context.Context, *model.Room) error { return errors.New("update room on leave failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, nil, nil)
		if _, _, err := uc.LeaveRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "update room on leave failed" {
			t.Fatalf("expected update room on leave failed, got %v", err)
		}
	})
}

func ptrString(v string) *string { return &v }

func TestRoomUsecase_StartRoom_PreconditionBranches(t *testing.T) {
	now := time.Now().UTC()
	baseRoom := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
	player := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}

	st := &authStoreStub{
		getRoomFn:       func(context.Context, string) (*model.Room, error) { return baseRoom, nil },
		getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil },
	}
	uc := NewRoomUsecase(st, nil, nil)

	if _, _, err := uc.StartRoom(context.Background(), "r1", ""); !errors.Is(err, ErrUnauthorizedUser) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	if _, _, err := uc.StartRoom(context.Background(), "", "u1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}

	st.getRoomFn = func(context.Context, string) (*model.Room, error) { return nil, repository.ErrNotFound }
	if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}

	st.getRoomFn = func(context.Context, string) (*model.Room, error) { return &model.Room{ID: "r1", HostUserID: "u2", Status: model.RoomStatusReady}, nil }
	if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); !errors.Is(err, ErrForbiddenAction) {
		t.Fatalf("expected forbidden, got %v", err)
	}

	st.getRoomFn = func(context.Context, string) (*model.Room, error) { return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting}, nil }
	if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); !errors.Is(err, ErrInvalidGameState) {
		t.Fatalf("expected invalid state for waiting room, got %v", err)
	}

	st.getRoomFn = func(context.Context, string) (*model.Room, error) { return baseRoom, nil }
	st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) {
		return &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerLeft, JoinedAt: now}, nil
	}
	if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); !errors.Is(err, ErrInvalidGameState) {
		t.Fatalf("expected invalid state for left player, got %v", err)
	}

	st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil }
	sid := "s1"
	roomWithSession := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CurrentSessionID: &sid}
	st.getRoomFn = func(context.Context, string) (*model.Room, error) { return roomWithSession, nil }
	if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); !errors.Is(err, ErrInvalidGameState) {
		t.Fatalf("expected invalid state when current_session_id exists, got %v", err)
	}

	st.getRoomFn = func(context.Context, string) (*model.Room, error) { return baseRoom, nil }
	st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) {
		return nil, errors.New("get room player for start failed")
	}
	if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "get room player for start failed" {
		t.Fatalf("expected get room player for start failed, got %v", err)
	}

	st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil }
	st.getLatestSessionFn = func(context.Context, string) (*model.GameSession, error) {
		return nil, errors.New("db down")
	}
	if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "db down" {
		t.Fatalf("expected latest session error, got %v", err)
	}
}

func TestRoomUsecase_StartRoom_Success(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
	player := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}

	var createdSession, createdPlayer, createdDealer, updatedRoom bool
	st := &authStoreStub{
		getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
		getRoomPlayerFn:    func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil },
		getLatestSessionFn: func(context.Context, string) (*model.GameSession, error) { return nil, repository.ErrNotFound },
		createSessionFn:    func(context.Context, *model.GameSession) error { createdSession = true; return nil },
		createPlayerStateFn: func(context.Context, *model.PlayerState) error {
			createdPlayer = true
			return nil
		},
		createDealerStateFn: func(context.Context, *model.DealerState) error {
			createdDealer = true
			return nil
		},
		updateRoomFn: func(context.Context, *model.Room) error { updatedRoom = true; return nil },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }

	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
	gotRoom, sess, err := uc.StartRoom(context.Background(), "r1", "u1")
	if err != nil {
		t.Fatalf("start room failed: %v", err)
	}
	if gotRoom == nil || sess == nil || gotRoom.CurrentSessionID == nil || *gotRoom.CurrentSessionID != sess.ID {
		t.Fatalf("unexpected start result: room=%+v sess=%+v", gotRoom, sess)
	}
	if !(createdSession && createdPlayer && createdDealer && updatedRoom) {
		t.Fatalf("expected all persistence hooks to run: session=%v player=%v dealer=%v room=%v", createdSession, createdPlayer, createdDealer, updatedRoom)
	}
}

func TestRoomUsecase_StartRoom_IncrementsRoundNoFromLatestSession(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
	player := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
	var roundNo int
	st := &authStoreStub{
		getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
		getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil },
		getLatestSessionFn: func(context.Context, string) (*model.GameSession, error) {
			return &model.GameSession{ID: "prev", RoomID: "r1", RoundNo: 4, CreatedAt: now, UpdatedAt: now}, nil
		},
		createSessionFn: func(_ context.Context, gs *model.GameSession) error {
			roundNo = gs.RoundNo
			return nil
		},
		createPlayerStateFn: func(context.Context, *model.PlayerState) error { return nil },
		createDealerStateFn: func(context.Context, *model.DealerState) error { return nil },
		updateRoomFn:        func(context.Context, *model.Room) error { return nil },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
	if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err != nil {
		t.Fatalf("StartRoom: %v", err)
	}
	if roundNo != 5 {
		t.Fatalf("expected round_no 5 from previous latest, got %d", roundNo)
	}
}

func TestRoomUsecase_StartRoom_BlackjackOpensDealerTurn(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
	player := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
	var persisted *model.GameSession
	st := &authStoreStub{
		getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
		getRoomPlayerFn:    func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil },
		getLatestSessionFn: func(context.Context, string) (*model.GameSession, error) { return nil, repository.ErrNotFound },
		createSessionFn: func(_ context.Context, gs *model.GameSession) error {
			persisted = gs
			return nil
		},
		createPlayerStateFn: func(context.Context, *model.PlayerState) error { return nil },
		createDealerStateFn: func(context.Context, *model.DealerState) error { return nil },
		updateRoomFn:        func(context.Context, *model.Room) error { return nil },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{blackjack: true}, appendEngine{})
	_, sess, err := uc.StartRoom(context.Background(), "r1", "u1")
	if err != nil {
		t.Fatalf("StartRoom: %v", err)
	}
	if sess.Status != model.SessionStatusDealerTurn {
		t.Fatalf("expected dealer turn after blackjack start, got %s", sess.Status)
	}
	if persisted == nil || persisted.Status != model.SessionStatusDealerTurn {
		t.Fatalf("expected persisted session dealer turn, got %+v", persisted)
	}
}

func TestRoomUsecase_Hit_BustAndReplay(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
		Deck: []model.StoredCard{{Rank: "K", Suit: "S"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
	}
	playerState := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "Q", Suit: "H"}}}
	dealerState := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "5", Suit: "D"}}}

	updated := false
	st := &authStoreStub{
		getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
		getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) {
			return playerState, nil
		},
		getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealerState, nil },
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
			updated = true
			return true, nil
		},
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }

	uc := NewRoomUsecase(st, fixedEvaluator{value: 22}, appendEngine{})
	_, outSess, err := uc.Hit(context.Background(), "r1", "u1", 1, "a1")
	if err != nil {
		t.Fatalf("hit failed: %v", err)
	}
	if !updated || outSess.Status != model.SessionStatusDealerTurn || playerState.Status != model.PlayerStatusBust || outSess.Version != 2 {
		t.Fatalf("unexpected hit state: updated=%v sessStatus=%s playerStatus=%s version=%d", updated, outSess.Status, playerState.Status, outSess.Version)
	}

	// Replay with same action_id/payload should short-circuit without DB updates.
	updated = false
	sess.Version = 2
	sess.Status = model.SessionStatusPlayerTurn
	sess.TurnSeat = 1
	playerState.Status = model.PlayerStatusActive
	payload := "HIT:" + strconv.FormatInt(2, 10)
	hash := sha256.Sum256([]byte(payload))
	st.getActionLogByIDFn = func(context.Context, string, string, string) (*model.ActionLog, error) {
		return &model.ActionLog{
			SessionID:          "s1",
			ActorType:          model.ActorTypeUser,
			ActorUserID:        "u1",
			ActionID:           "a1",
			RequestType:        "HIT",
			RequestPayloadHash: hex.EncodeToString(hash[:]),
		}, nil
	}
	if _, _, err := uc.Hit(context.Background(), "r1", "u1", 2, "a1"); err != nil {
		t.Fatalf("replay hit failed: %v", err)
	}
	if updated {
		t.Fatal("expected replay to skip session update")
	}
}

func TestRoomUsecase_StartRoom_PersistenceErrors(t *testing.T) {
	now := time.Now().UTC()
	makeBaseStore := func() *authStoreStub {
		baseRoom := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
		player := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
		return &authStoreStub{
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return baseRoom, nil },
			getRoomPlayerFn:    func(context.Context, string, string) (*model.RoomPlayer, error) { return player, nil },
			getLatestSessionFn: func(context.Context, string) (*model.GameSession, error) { return nil, repository.ErrNotFound },
			createSessionFn:    func(context.Context, *model.GameSession) error { return nil },
			createPlayerStateFn: func(context.Context, *model.PlayerState) error {
				return nil
			},
			createDealerStateFn: func(context.Context, *model.DealerState) error { return nil },
			updateRoomFn:        func(context.Context, *model.Room) error { return nil },
		}
	}

	t.Run("create player state error", func(t *testing.T) {
		st := makeBaseStore()
		st.createPlayerStateFn = func(context.Context, *model.PlayerState) error { return errors.New("create player failed") }
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "create player failed" {
			t.Fatalf("expected create player failed, got %v", err)
		}
	})

	t.Run("update room error", func(t *testing.T) {
		st := makeBaseStore()
		st.updateRoomFn = func(context.Context, *model.Room) error { return errors.New("update room failed") }
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "update room failed" {
			t.Fatalf("expected update room failed, got %v", err)
		}
	})

	t.Run("create session error", func(t *testing.T) {
		st := makeBaseStore()
		st.createSessionFn = func(context.Context, *model.GameSession) error { return errors.New("create session failed") }
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "create session failed" {
			t.Fatalf("expected create session failed, got %v", err)
		}
	})

	t.Run("create dealer state error", func(t *testing.T) {
		st := makeBaseStore()
		st.createDealerStateFn = func(context.Context, *model.DealerState) error { return errors.New("create dealer failed") }
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "create dealer failed" {
			t.Fatalf("expected create dealer failed, got %v", err)
		}
	})
}

func TestRoomUsecase_Hit_PersistenceErrors(t *testing.T) {
	now := time.Now().UTC()
	makeBaseStore := func() *authStoreStub {
		sid := "s1"
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "K", Suit: "S"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		playerState := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "Q", Suit: "H"}}}
		dealerState := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "5", Suit: "D"}}}
		return &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) {
				return playerState, nil
			},
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealerState, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				return true, nil
			},
			updatePlayerStateFn: func(context.Context, *model.PlayerState) error { return nil },
			updateDealerStateFn: func(context.Context, *model.DealerState) error { return nil },
			updateRoomFn:        func(context.Context, *model.Room) error { return nil },
			createActionLogFn:   func(context.Context, *model.ActionLog) error { return nil },
		}
	}

	t.Run("update player state error", func(t *testing.T) {
		st := makeBaseStore()
		st.updatePlayerStateFn = func(context.Context, *model.PlayerState) error { return errors.New("update player failed") }
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 22}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "hp1"); err == nil || err.Error() != "update player failed" {
			t.Fatalf("expected update player failed, got %v", err)
		}
	})

	t.Run("update dealer state error", func(t *testing.T) {
		st := makeBaseStore()
		st.updateDealerStateFn = func(context.Context, *model.DealerState) error { return errors.New("update dealer failed") }
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 22}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "hp2"); err == nil || err.Error() != "update dealer failed" {
			t.Fatalf("expected update dealer failed, got %v", err)
		}
	})

	t.Run("snapshot save error on hit", func(t *testing.T) {
		st := makeBaseStore()
		st.createActionLogFn = func(context.Context, *model.ActionLog) error { return errors.New("save snapshot failed") }
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 22}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "hp3"); err == nil || err.Error() != "save snapshot failed" {
			t.Fatalf("expected save snapshot failed, got %v", err)
		}
	})

	t.Run("snapshot save error on stand", func(t *testing.T) {
		st := makeBaseStore()
		st.createActionLogFn = func(context.Context, *model.ActionLog) error { return errors.New("save snapshot failed stand") }
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 10}, appendEngine{})
		if _, _, err := uc.Stand(context.Background(), "r1", "u1", 1, "sp1"); err == nil || err.Error() != "save snapshot failed stand" {
			t.Fatalf("expected save snapshot failed stand, got %v", err)
		}
	})
}

func TestRoomUsecase_Hit_ValidationAndStateErrors(t *testing.T) {
	now := time.Now().UTC()

	t.Run("auth and input validation", func(t *testing.T) {
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "", 1, "a1"); !errors.Is(err, ErrUnauthorizedUser) {
			t.Fatalf("expected unauthorized, got %v", err)
		}
		if _, _, err := uc.Hit(context.Background(), "", "u1", 1, "a1"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for room id, got %v", err)
		}
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 0, "a1"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for version, got %v", err)
		}
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, ""); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for action id, got %v", err)
		}
	})

	t.Run("room and session retrieval errors", func(t *testing.T) {
		sid := "s1"
		st := &authStoreStub{
			getRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("room fetch failed") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a1"); err == nil || err.Error() != "room fetch failed" {
			t.Fatalf("expected room fetch failed, got %v", err)
		}

		st.getRoomFn = func(context.Context, string) (*model.Room, error) {
			return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: nil, CreatedAt: now, UpdatedAt: now}, nil
		}
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a1"); !errors.Is(err, ErrInvalidGameState) {
			t.Fatalf("expected invalid game state, got %v", err)
		}

		st.getRoomFn = func(context.Context, string) (*model.Room, error) {
			return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}, nil
		}
		st.getSessionFn = func(context.Context, string) (*model.GameSession, error) { return nil, errors.New("session fetch failed") }
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a1"); err == nil || err.Error() != "session fetch failed" {
			t.Fatalf("expected session fetch failed, got %v", err)
		}
	})

	t.Run("version and player/dealer errors", func(t *testing.T) {
		sid := "s1"
		sess := &model.GameSession{ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 2, TurnSeat: 1, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}, nil },
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a1"); !errors.Is(err, model.ErrVersionConflict) {
			t.Fatalf("expected version conflict, got %v", err)
		}

		sess.Version = 1
		st.getPlayerStateFn = func(context.Context, string, string) (*model.PlayerState, error) { return nil, repository.ErrNotFound }
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a1"); !errors.Is(err, ErrForbiddenAction) {
			t.Fatalf("expected forbidden, got %v", err)
		}

		st.getPlayerStateFn = func(context.Context, string, string) (*model.PlayerState, error) {
			return &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "9", Suit: "H"}}}, nil
		}
		st.getDealerStateFn = func(context.Context, string) (*model.DealerState, error) { return nil, errors.New("dealer fetch failed") }
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a1"); err == nil || err.Error() != "dealer fetch failed" {
			t.Fatalf("expected dealer fetch failed, got %v", err)
		}
	})

	t.Run("update session version conflict bubbles", func(t *testing.T) {
		sid := "s1"
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "6", Suit: "C"}}, CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}, nil },
			getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "9", Suit: "H"}}}, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "10", Suit: "D"}}}, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				return false, nil
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 12}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a1"); !errors.Is(err, model.ErrVersionConflict) {
			t.Fatalf("expected version conflict, got %v", err)
		}
	})

	t.Run("update session repository error in transaction", func(t *testing.T) {
		sid := "s1"
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "6", Suit: "C"}}, CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}, nil },
			getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "9", Suit: "H"}}}, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "10", Suit: "D"}}}, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				return false, errors.New("session update repo failed")
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 12}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a-hit-repo"); err == nil || err.Error() != "session update repo failed" {
			t.Fatalf("expected session update repo failed, got %v", err)
		}
	})
}

func TestRoomUsecase_VoteRematch_DefaultAndDenial(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	session := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusResetting, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	players := []*model.RoomPlayer{
		{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now},
		{RoomID: "r1", UserID: "u2", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now},
	}

	t.Run("default path increments session version", func(t *testing.T) {
		updated := false
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return session, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return session, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return players, nil },
			listRematchVotesFn:    func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				updated = true
				return true, nil
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})

		gotRoom, gotSess, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv1")
		if err != nil {
			t.Fatalf("vote rematch failed: %v", err)
		}
		if gotRoom == nil || gotSess == nil || !updated || gotSess.Version != 2 {
			t.Fatalf("unexpected rematch default result: room=%+v sess=%+v updated=%v", gotRoom, gotSess, updated)
		}
	})

	t.Run("explicit denial finalizes current session", func(t *testing.T) {
		localRoom := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
		localSess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusResetting, Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		roomUpdated := false
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return localRoom, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return localSess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return localSess, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return []*model.RoomPlayer{{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}}, nil
			},
			updateRoomFn:          func(context.Context, *model.Room) error { roomUpdated = true; return nil },
			listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) {
				return []*model.RematchVote{{SessionID: "s1", UserID: "u2", Agree: true}}, nil
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})

		gotRoom, gotSess, err := uc.VoteRematch(context.Background(), "r1", "u1", false, 1, "rv2")
		if err != nil {
			t.Fatalf("vote rematch denial failed: %v", err)
		}
		if gotRoom == nil || gotSess == nil || !roomUpdated || gotRoom.CurrentSessionID != nil {
			t.Fatalf("unexpected rematch denial result: room=%+v sess=%+v updated=%v", gotRoom, gotSess, roomUpdated)
		}
	})
}

func TestRoomUsecase_AutoStandDueSessions_BasicFlow(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
		TurnDeadlineAt: ptrTime(now.Add(-1 * time.Second)), CreatedAt: now, UpdatedAt: now,
	}
	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "9", Suit: "H"}}}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "10", Suit: "D"}}}

	updated := false
	st := &authStoreStub{
		listSessionsByStatusAndDeadlineBeforeFn: func(context.Context, model.SessionStatus, time.Time) ([]*model.GameSession, error) {
			return []*model.GameSession{sess}, nil
		},
		listSessionsByStatusFn:         func(context.Context, model.SessionStatus) ([]*model.GameSession, error) { return nil, nil },
		listResettingSessionsDueByFn:   func(context.Context, time.Time) ([]*model.GameSession, error) { return nil, nil },
		getSessionFn:                   func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getRoomFn:                      func(context.Context, string) (*model.Room, error) { return room, nil },
		listPlayerStatesFn:             func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
		getDealerStateFn:               func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		updateSessionIfVersionFn:       func(context.Context, *model.GameSession, int64) (bool, error) { updated = true; return true, nil },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})

	rooms, err := uc.AutoStandDueSessions(context.Background())
	if err != nil {
		t.Fatalf("auto stand failed: %v", err)
	}
	if len(rooms) != 1 || rooms[0] != "r1" || !updated {
		t.Fatalf("unexpected auto stand result: rooms=%v updated=%v", rooms, updated)
	}
}

func TestRoomUsecase_DealerTurn_VersionConflictAndNoPlayer(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusDealerTurn, Version: 1,
		Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
	}
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "D"}, {Rank: "7", Suit: "S"}}}

	t.Run("no players returns not found", func(t *testing.T) {
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return nil, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{value: 16}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("version conflict bubbles up", func(t *testing.T) {
		localSess := *sess
		st := &authStoreStub{
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return []*model.PlayerState{player}, nil
			},
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				return false, nil
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 16}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); !errors.Is(err, model.ErrVersionConflict) {
			t.Fatalf("expected version conflict, got %v", err)
		}
	})

	t.Run("update session error bubbles up", func(t *testing.T) {
		localSess := *sess
		st := &authStoreStub{
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return []*model.PlayerState{player}, nil
			},
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				return false, errors.New("update session failed")
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 16}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), "s1"); err == nil || err.Error() != "update session failed" {
			t.Fatalf("expected update session failed, got %v", err)
		}
	})
}

func TestRoomUsecase_ProcessRematchDeadline_Branches(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-2 * time.Second)
	sid := "s1"
	baseRoom := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}

	t.Run("non-resetting session no-op", func(t *testing.T) {
		sess := &model.GameSession{ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.processRematchDeadline(context.Background(), "s1"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("deadline reached and no eligible players finalizes failure", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 3,
			RematchDeadlineAt: &past, CreatedAt: now, UpdatedAt: now,
		}
		roomUpdated := false
		st := &authStoreStub{
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return baseRoom, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return []*model.RoomPlayer{{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerLeft, JoinedAt: now}}, nil
			},
			listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
			updateRoomFn:       func(context.Context, *model.Room) error { roomUpdated = true; return nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.processRematchDeadline(context.Background(), "s1"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if !roomUpdated || baseRoom.CurrentSessionID != nil {
			t.Fatalf("expected room finalization, updated=%v current_session=%v", roomUpdated, baseRoom.CurrentSessionID)
		}
	})

	t.Run("list rematch votes error bubbles", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 3,
			RematchDeadlineAt: &past, CreatedAt: now, UpdatedAt: now,
		}
		localSID := "s1"
		localRoom := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &localSID, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return localRoom, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return []*model.RoomPlayer{{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}}, nil
			},
			listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) {
				return nil, errors.New("list votes failed")
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.processRematchDeadline(context.Background(), "s1"); err == nil || err.Error() != "list votes failed" {
			t.Fatalf("expected list votes failed, got %v", err)
		}
	})

	t.Run("get session for update error bubbles", func(t *testing.T) {
		st := &authStoreStub{
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return nil, errors.New("lock deadline failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.processRematchDeadline(context.Background(), "s1"); err == nil || err.Error() != "lock deadline failed" {
			t.Fatalf("expected lock deadline failed, got %v", err)
		}
	})

	t.Run("get room error bubbles", func(t *testing.T) {
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 3,
			RematchDeadlineAt: &past, CreatedAt: now, UpdatedAt: now,
		}
		st := &authStoreStub{
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return nil, errors.New("room lookup failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.processRematchDeadline(context.Background(), "s1"); err == nil || err.Error() != "room lookup failed" {
			t.Fatalf("expected room lookup failed, got %v", err)
		}
	})
}

func TestRoomUsecase_DealerTurn_DrawSuccess(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusDealerTurn, Version: 5,
		Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
	}
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "D"}, {Rank: "6", Suit: "S"}}}

	dealerUpdated := false
	st := &authStoreStub{
		getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
		listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
			return []*model.PlayerState{player}, nil
		},
		getDealerStateFn:         func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
		updateDealerStateFn: func(context.Context, *model.DealerState) error {
			dealerUpdated = true
			return nil
		},
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{value: 16}, appendEngine{}).(*roomService)

	if err := uc.dealerTurn(context.Background(), "s1"); err != nil {
		t.Fatalf("dealerTurn failed: %v", err)
	}
	if !dealerUpdated || len(dealer.Hand) != 3 || sess.Version != 6 {
		t.Fatalf("unexpected dealer draw result: updated=%v hand=%d version=%d", dealerUpdated, len(dealer.Hand), sess.Version)
	}
}

func TestRoomUsecase_ProcessRematchDeadline_UnanimousSuccess(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-3 * time.Second)
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusResetting, Version: 4,
		RematchDeadlineAt: &past, CreatedAt: now, UpdatedAt: now,
	}

	updatedPrev := false
	createdNext := false
	updatedRoom := false
	st := &authStoreStub{
		getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
		listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
			return []*model.RoomPlayer{{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}}, nil
		},
		listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) {
			return []*model.RematchVote{{SessionID: "s1", UserID: "u1", Agree: true}}, nil
		},
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
			updatedPrev = true
			return true, nil
		},
		createSessionFn: func(context.Context, *model.GameSession) error { createdNext = true; return nil },
		updateRoomFn:    func(context.Context, *model.Room) error { updatedRoom = true; return nil },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)

	if err := uc.processRematchDeadline(context.Background(), "s1"); err != nil {
		t.Fatalf("processRematchDeadline failed: %v", err)
	}
	if !updatedPrev || !createdNext || !updatedRoom || room.CurrentSessionID == nil || *room.CurrentSessionID == "s1" {
		t.Fatalf("unexpected unanimous result: updatedPrev=%v createdNext=%v updatedRoom=%v currentSessionID=%v", updatedPrev, createdNext, updatedRoom, room.CurrentSessionID)
	}
}

func TestRoomUsecase_PlayerStand_HeuristicTriggersHitReplay(t *testing.T) {
	t.Setenv("BLACKJACK_PLAYER_TIMEOUT_POLICY", "heuristic")

	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
		TurnDeadlineAt: ptrTime(now.Add(-1 * time.Second)), Deck: []model.StoredCard{{Rank: "3", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
	}
	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "5", Suit: "H"}, {Rank: "3", Suit: "D"}}}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "10", Suit: "S"}}}

	actionID := "auto-heuristic-hit:s1:1"
	payload := "HIT:1"
	hash := sha256.Sum256([]byte(payload))
	updatedByStandPath := false
	st := &authStoreStub{
		getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
		listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
			return []*model.PlayerState{player}, nil
		},
		getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
		getActionLogByIDFn: func(context.Context, string, string, string) (*model.ActionLog, error) {
			return &model.ActionLog{
				SessionID:          "s1",
				ActorType:          model.ActorTypeUser,
				ActorUserID:        "u1",
				ActionID:           actionID,
				RequestType:        "HIT",
				RequestPayloadHash: hex.EncodeToString(hash[:]),
			}, nil
		},
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
			updatedByStandPath = true
			return true, nil
		},
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{value: 8}, appendEngine{}).(*roomService)

	if err := uc.playerStand(context.Background(), "s1"); err != nil {
		t.Fatalf("playerStand heuristic path failed: %v", err)
	}
	if updatedByStandPath {
		t.Fatal("expected heuristic branch to avoid AUTO_STAND update path")
	}
}

func TestRoomUsecase_VoteRematch_ForbiddenWhenNotEligible(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 1, CreatedAt: now, UpdatedAt: now}

	st := &authStoreStub{
		getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
		getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}, nil },
		getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
			return []*model.RoomPlayer{{RoomID: "r1", UserID: "u2", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now}}, nil
		},
		listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})

	if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv-forbidden"); !errors.Is(err, ErrForbiddenAction) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestRoomUsecase_VoteRematch_PersistenceErrors(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 1, CreatedAt: now, UpdatedAt: now}
	eligible := []*model.RoomPlayer{
		{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now},
		{RoomID: "r1", UserID: "u2", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now},
	}

	t.Run("upsert rematch vote error bubbles", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return eligible[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return eligible, nil },
			listRematchVotesFn:    func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
			upsertRematchVoteFn:   func(context.Context, *model.RematchVote) error { return errors.New("upsert failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv-upsert"); err == nil || err.Error() != "upsert failed" {
			t.Fatalf("expected upsert failed, got %v", err)
		}
	})

	t.Run("list room players in transaction error", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return eligible[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return nil, errors.New("list rp in vote failed") },
			listRematchVotesFn:    func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv-list-rp"); err == nil || err.Error() != "list rp in vote failed" {
			t.Fatalf("expected list rp in vote failed, got %v", err)
		}
	})

	t.Run("save action snapshot error bubbles", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return eligible[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return eligible, nil },
			listRematchVotesFn:    func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
			upsertRematchVoteFn:   func(context.Context, *model.RematchVote) error { return nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				return true, nil
			},
			createActionLogFn: func(context.Context, *model.ActionLog) error { return errors.New("snapshot save failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv-snapshot"); err == nil || err.Error() != "snapshot save failed" {
			t.Fatalf("expected snapshot save failed, got %v", err)
		}
	})

	t.Run("get session for update error bubbles", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return eligible[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return nil, errors.New("lock failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv-lock"); err == nil || err.Error() != "lock failed" {
			t.Fatalf("expected lock failed, got %v", err)
		}
	})

	t.Run("locked status not resetting returns invalid game state", func(t *testing.T) {
		locked := &model.GameSession{ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return eligible[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return locked, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv-status"); !errors.Is(err, ErrInvalidGameState) {
			t.Fatalf("expected invalid game state, got %v", err)
		}
	})

	t.Run("check version conflict bubbles", func(t *testing.T) {
		locked := &model.GameSession{ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 2, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return eligible[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return locked, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv-version"); !errors.Is(err, model.ErrVersionConflict) {
			t.Fatalf("expected version conflict, got %v", err)
		}
	})
}

func TestRoomUsecase_VoteRematch_ValidationAndLookupErrors(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 1, CreatedAt: now, UpdatedAt: now}

	t.Run("auth and input validation", func(t *testing.T) {
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "", true, 1, "rv1"); !errors.Is(err, ErrUnauthorizedUser) {
			t.Fatalf("expected unauthorized, got %v", err)
		}
		if _, _, err := uc.VoteRematch(context.Background(), "", "u1", true, 1, "rv1"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input, got %v", err)
		}
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 0, "rv1"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for version, got %v", err)
		}
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, ""); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for action id, got %v", err)
		}
	})

	t.Run("lookup errors and forbidden", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("room lookup failed") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv1"); err == nil || err.Error() != "room lookup failed" {
			t.Fatalf("expected room lookup failed, got %v", err)
		}

		st.getRoomFn = func(context.Context, string) (*model.Room, error) { return room, nil }
		st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, repository.ErrNotFound }
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv1"); !errors.Is(err, ErrForbiddenAction) {
			t.Fatalf("expected forbidden, got %v", err)
		}

		st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, errors.New("membership lookup failed") }
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv1"); err == nil || err.Error() != "membership lookup failed" {
			t.Fatalf("expected membership lookup failed, got %v", err)
		}

		st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) {
			return &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}, nil
		}
		st.getLatestSessionFn = func(context.Context, string) (*model.GameSession, error) { return nil, errors.New("latest session failed") }
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv1"); err == nil || err.Error() != "latest session failed" {
			t.Fatalf("expected latest session failed, got %v", err)
		}
	})

	t.Run("list votes error and replay path", func(t *testing.T) {
		players := []*model.RoomPlayer{{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}}
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return players, nil },
			listRematchVotesFn:    func(context.Context, string) ([]*model.RematchVote, error) { return nil, errors.New("list votes failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv1"); err == nil || err.Error() != "list votes failed" {
			t.Fatalf("expected list votes failed, got %v", err)
		}

		st.listRematchVotesFn = func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound }
		st.getActionLogByIDFn = func(context.Context, string, string, string) (*model.ActionLog, error) {
			payload := "REMATCH_VOTE:true:1"
			hash := sha256.Sum256([]byte(payload))
			return &model.ActionLog{
				SessionID:          "s1",
				ActorType:          model.ActorTypeUser,
				ActorUserID:        "u1",
				ActionID:           "rv-replay",
				RequestType:        "REMATCH_VOTE",
				RequestPayloadHash: hex.EncodeToString(hash[:]),
			}, nil
		}
		st.upsertRematchVoteFn = func(context.Context, *model.RematchVote) error { return errors.New("should not upsert on replay") }
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "rv-replay"); err != nil {
			t.Fatalf("expected replay success, got %v", err)
		}
	})
}

func TestRoomUsecase_ProcessRematchDeadline_CurrentSessionMismatchNoop(t *testing.T) {
	now := time.Now().UTC()
	past := now.Add(-2 * time.Second)
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", Status: model.SessionStatusResetting, Version: 2,
		RematchDeadlineAt: &past, CreatedAt: now, UpdatedAt: now,
	}
	other := "s2"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &other, CreatedAt: now, UpdatedAt: now}
	updatedRoom := false

	st := &authStoreStub{
		getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
		updateRoomFn:          func(context.Context, *model.Room) error { updatedRoom = true; return nil },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)

	if err := uc.processRematchDeadline(context.Background(), "s1"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if updatedRoom {
		t.Fatal("expected no-op when room current_session_id mismatches")
	}
}

func TestRoomUsecase_AutoStandDueSessions_IgnoresVersionConflict(t *testing.T) {
	now := time.Now().UTC()
	dsid := "dealer-s1"
	rsid := "rematch-s1"
	roomDealer := &model.Room{ID: "r-dealer", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &dsid, CreatedAt: now, UpdatedAt: now}
	roomRematch := &model.Room{ID: "r-rematch", HostUserID: "u2", Status: model.RoomStatusPlaying, CurrentSessionID: &rsid, CreatedAt: now, UpdatedAt: now}

	dealerSess := &model.GameSession{
		ID: dsid, RoomID: roomDealer.ID, Status: model.SessionStatusDealerTurn, Version: 1,
		Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
	}
	rematchSess := &model.GameSession{
		ID: rsid, RoomID: roomRematch.ID, Status: model.SessionStatusResetting, Version: 2,
		RematchDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
	}
	st := &authStoreStub{
		listSessionsByStatusAndDeadlineBeforeFn: func(context.Context, model.SessionStatus, time.Time) ([]*model.GameSession, error) {
			return nil, nil
		},
		listSessionsByStatusFn: func(context.Context, model.SessionStatus) ([]*model.GameSession, error) {
			return []*model.GameSession{dealerSess}, nil
		},
		listResettingSessionsDueByFn: func(context.Context, time.Time) ([]*model.GameSession, error) {
			return []*model.GameSession{rematchSess}, nil
		},
		getSessionFn: func(context.Context, string) (*model.GameSession, error) { return dealerSess, nil },
		getDealerStateFn: func(context.Context, string) (*model.DealerState, error) {
			return &model.DealerState{SessionID: dsid, Hand: []model.StoredCard{{Rank: "9", Suit: "D"}, {Rank: "6", Suit: "S"}}}, nil
		},
		listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
			return []*model.PlayerState{{SessionID: dsid, UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}}, nil
		},
		getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return rematchSess, nil },
		getRoomFn: func(_ context.Context, roomID string) (*model.Room, error) {
			if roomID == roomDealer.ID {
				return roomDealer, nil
			}
			return roomRematch, nil
		},
		listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
			return []*model.RoomPlayer{{RoomID: roomRematch.ID, UserID: "u2", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}}, nil
		},
		listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) {
			return []*model.RematchVote{{SessionID: rsid, UserID: "u2", Agree: true}}, nil
		},
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
			return false, nil
		},
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{value: 16}, appendEngine{})

	rooms, err := uc.AutoStandDueSessions(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(rooms) != 2 {
		t.Fatalf("expected 2 updated rooms, got %v", rooms)
	}
}

func TestRoomUsecase_AutoStandDueSessions_EarlyReturnErrors(t *testing.T) {
	t.Run("list player-turn sessions error", func(t *testing.T) {
		st := &authStoreStub{
			listSessionsByStatusAndDeadlineBeforeFn: func(context.Context, model.SessionStatus, time.Time) ([]*model.GameSession, error) {
				return nil, errors.New("list due failed")
			},
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.AutoStandDueSessions(context.Background()); err == nil || err.Error() != "list due failed" {
			t.Fatalf("expected list due error, got %v", err)
		}
	})

	t.Run("list dealer sessions error", func(t *testing.T) {
		st := &authStoreStub{
			listSessionsByStatusAndDeadlineBeforeFn: func(context.Context, model.SessionStatus, time.Time) ([]*model.GameSession, error) {
				return nil, nil
			},
			listSessionsByStatusFn: func(context.Context, model.SessionStatus) ([]*model.GameSession, error) {
				return nil, errors.New("list dealer failed")
			},
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.AutoStandDueSessions(context.Background()); err == nil || err.Error() != "list dealer failed" {
			t.Fatalf("expected list dealer error, got %v", err)
		}
	})

	t.Run("list rematch due error", func(t *testing.T) {
		st := &authStoreStub{
			listSessionsByStatusAndDeadlineBeforeFn: func(context.Context, model.SessionStatus, time.Time) ([]*model.GameSession, error) {
				return nil, nil
			},
			listSessionsByStatusFn: func(context.Context, model.SessionStatus) ([]*model.GameSession, error) {
				return nil, nil
			},
			listResettingSessionsDueByFn: func(context.Context, time.Time) ([]*model.GameSession, error) {
				return nil, errors.New("list rematch failed")
			},
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.AutoStandDueSessions(context.Background()); err == nil || err.Error() != "list rematch failed" {
			t.Fatalf("expected list rematch error, got %v", err)
		}
	})

	t.Run("playerStand unexpected error returns early", func(t *testing.T) {
		s := &model.GameSession{ID: "ps1", RoomID: "r-ps"}
		st := &authStoreStub{
			listSessionsByStatusAndDeadlineBeforeFn: func(context.Context, model.SessionStatus, time.Time) ([]*model.GameSession, error) {
				return []*model.GameSession{s}, nil
			},
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return nil, errors.New("player stand failed") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.AutoStandDueSessions(context.Background()); err == nil || err.Error() != "player stand failed" {
			t.Fatalf("expected player stand failed, got %v", err)
		}
	})

	t.Run("dealerTurn unexpected error returns early", func(t *testing.T) {
		ds := &model.GameSession{ID: "ds1", RoomID: "r-dealer", Status: model.SessionStatusDealerTurn, Version: 1}
		st := &authStoreStub{
			listSessionsByStatusAndDeadlineBeforeFn: func(context.Context, model.SessionStatus, time.Time) ([]*model.GameSession, error) {
				return nil, nil
			},
			listSessionsByStatusFn: func(context.Context, model.SessionStatus) ([]*model.GameSession, error) {
				return []*model.GameSession{ds}, nil
			},
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return ds, nil },
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return nil, errors.New("dealer turn failed") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.AutoStandDueSessions(context.Background()); err == nil || err.Error() != "dealer turn failed" {
			t.Fatalf("expected dealer turn failed, got %v", err)
		}
	})

	t.Run("processRematchDeadline unexpected error returns early", func(t *testing.T) {
		rs := &model.GameSession{ID: "rs1", RoomID: "r-rematch", Status: model.SessionStatusResetting}
		st := &authStoreStub{
			listSessionsByStatusAndDeadlineBeforeFn: func(context.Context, model.SessionStatus, time.Time) ([]*model.GameSession, error) {
				return nil, nil
			},
			listSessionsByStatusFn: func(context.Context, model.SessionStatus) ([]*model.GameSession, error) {
				return nil, nil
			},
			listResettingSessionsDueByFn: func(context.Context, time.Time) ([]*model.GameSession, error) {
				return []*model.GameSession{rs}, nil
			},
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) {
				return nil, errors.New("deadline process failed")
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.AutoStandDueSessions(context.Background()); err == nil || err.Error() != "deadline process failed" {
			t.Fatalf("expected deadline process failed, got %v", err)
		}
	})
}

func TestRoomUsecase_GetRoomState_NotFoundToleranceAndCanFlags(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 9, TurnSeat: 1, CreatedAt: now, UpdatedAt: now,
	}
	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "9", Suit: "H"}}}

	st := &authStoreStub{
		getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
		getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return nil, repository.ErrNotFound },
		listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
			return []*model.PlayerState{player}, nil
		},
	}
	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
	state, err := uc.GetRoomState(context.Background(), "r1", "u1")
	if err != nil {
		t.Fatalf("GetRoomState failed: %v", err)
	}
	if state.Dealer != nil || !state.CanHit || !state.CanStand || state.CanRematch {
		t.Fatalf("unexpected state: dealer=%+v canHit=%v canStand=%v canRematch=%v", state.Dealer, state.CanHit, state.CanStand, state.CanRematch)
	}

	t.Run("player states not found is tolerated", func(t *testing.T) {
		st2 := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "6", Suit: "S"}}}, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return nil, repository.ErrNotFound
			},
		}
		uc2 := NewRoomUsecase(st2, fixedEvaluator{}, appendEngine{})
		state2, err := uc2.GetRoomState(context.Background(), "r1", "u1")
		if err != nil {
			t.Fatalf("GetRoomState failed: %v", err)
		}
		if len(state2.Players) != 0 || state2.CanHit || state2.CanStand {
			t.Fatalf("unexpected fallback state: players=%v canHit=%v canStand=%v", state2.Players, state2.CanHit, state2.CanStand)
		}
	})

	t.Run("dealer state non-notfound error bubbles", func(t *testing.T) {
		st3 := &authStoreStub{
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) {
				return nil, errors.New("dealer fetch failed")
			},
		}
		uc3 := NewRoomUsecase(st3, fixedEvaluator{}, appendEngine{})
		if _, err := uc3.GetRoomState(context.Background(), "r1", "u1"); err == nil || err.Error() != "dealer fetch failed" {
			t.Fatalf("expected dealer fetch failed, got %v", err)
		}
	})

	t.Run("player states non-notfound error bubbles", func(t *testing.T) {
		st4 := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return nil, repository.ErrNotFound },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return nil, errors.New("players fetch failed")
			},
		}
		uc4 := NewRoomUsecase(st4, fixedEvaluator{}, appendEngine{})
		if _, err := uc4.GetRoomState(context.Background(), "r1", "u1"); err == nil || err.Error() != "players fetch failed" {
			t.Fatalf("expected players fetch failed, got %v", err)
		}
	})
}

func TestRoomUsecase_SuggestPlayerAction_ErrorBranches(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 4, TurnSeat: 1, CreatedAt: now, UpdatedAt: now,
	}
	basePlayer := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "9", Suit: "H"}, {Rank: "7", Suit: "D"}}}

	t.Run("dealer hand empty returns invalid state", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) {
				return &model.DealerState{SessionID: "s1", Hand: nil}, nil
			},
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return []*model.PlayerState{basePlayer}, nil
			},
		}
		uc := NewRoomUsecase(st, fixedEvaluator{value: 16}, appendEngine{})
		if _, err := uc.SuggestPlayerAction(context.Background(), "r1", "u1"); !errors.Is(err, ErrInvalidGameState) {
			t.Fatalf("expected invalid state, got %v", err)
		}
	})

	t.Run("player hand empty returns invalid state", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "6", Suit: "S"}}}, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return []*model.PlayerState{{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: nil}}, nil
			},
		}
		uc := NewRoomUsecase(st, fixedEvaluator{value: 16}, appendEngine{})
		if _, err := uc.SuggestPlayerAction(context.Background(), "r1", "u1"); !errors.Is(err, ErrInvalidGameState) {
			t.Fatalf("expected invalid state, got %v", err)
		}
	})

	t.Run("get room state dependency error bubbles", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) {
				return nil, errors.New("dealer failure")
			},
		}
		uc := NewRoomUsecase(st, fixedEvaluator{value: 16}, appendEngine{})
		if _, err := uc.SuggestPlayerAction(context.Background(), "r1", "u1"); err == nil || err.Error() != "dealer failure" {
			t.Fatalf("expected dealer failure, got %v", err)
		}
	})
}

func TestRoomUsecase_SuggestPlayerAction_SuccessHitAndStand(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 7, TurnSeat: 1, CreatedAt: now, UpdatedAt: now,
	}
	player := &model.PlayerState{
		SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive,
		Hand: []model.StoredCard{{Rank: "8", Suit: "H"}, {Rank: "3", Suit: "D"}},
	}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "10", Suit: "S"}}}

	makeStore := func() *authStoreStub {
		return &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return []*model.PlayerState{player}, nil
			},
		}
	}

	t.Run("recommend hit", func(t *testing.T) {
		uc := NewRoomUsecase(makeStore(), fixedEvaluator{value: 11}, appendEngine{})
		hint, err := uc.SuggestPlayerAction(context.Background(), "r1", "u1")
		if err != nil {
			t.Fatalf("SuggestPlayerAction failed: %v", err)
		}
		if hint.Recommendation != "HIT" || hint.SessionVersion != 7 {
			t.Fatalf("unexpected hint: %+v", hint)
		}
	})

	t.Run("recommend stand", func(t *testing.T) {
		uc := NewRoomUsecase(makeStore(), fixedEvaluator{value: 17}, appendEngine{})
		hint, err := uc.SuggestPlayerAction(context.Background(), "r1", "u1")
		if err != nil {
			t.Fatalf("SuggestPlayerAction failed: %v", err)
		}
		if hint.Recommendation != "STAND" || hint.SessionVersion != 7 {
			t.Fatalf("unexpected hint: %+v", hint)
		}
	})
}

func TestRoomUsecase_MarkConnectedAndDisconnected_Branches(t *testing.T) {
	now := time.Now().UTC()
	active := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
	left := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerLeft, JoinedAt: now}

	t.Run("mark connected invalid input and no-op statuses", func(t *testing.T) {
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{})
		if err := uc.MarkConnected(context.Background(), "", "u1"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input, got %v", err)
		}
		if err := uc.MarkConnected(context.Background(), "r1", ""); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for empty user, got %v", err)
		}

		st := &authStoreStub{getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return active, nil }}
		uc2 := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if err := uc2.MarkConnected(context.Background(), "r1", "u1"); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}

		st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return left, nil }
		if err := uc2.MarkConnected(context.Background(), "r1", "u1"); err != nil {
			t.Fatalf("expected nil for left, got %v", err)
		}

		disc := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerDisconnected, JoinedAt: now}
		st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return disc, nil }
		st.updateRoomPlayerFn = func(context.Context, *model.RoomPlayer) error { return errors.New("update connected failed") }
		if err := uc2.MarkConnected(context.Background(), "r1", "u1"); err == nil || err.Error() != "update connected failed" {
			t.Fatalf("expected update connected failed, got %v", err)
		}
	})

	t.Run("mark connected get room player error propagates", func(t *testing.T) {
		st := &authStoreStub{
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, errors.New("get player for connect failed") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if err := uc.MarkConnected(context.Background(), "r1", "u1"); err == nil || err.Error() != "get player for connect failed" {
			t.Fatalf("expected get player for connect failed, got %v", err)
		}
	})

	t.Run("mark disconnected invalid input", func(t *testing.T) {
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{})
		if err := uc.MarkDisconnected(context.Background(), "", "u1"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for room id, got %v", err)
		}
		if err := uc.MarkDisconnected(context.Background(), "r1", ""); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input for user id, got %v", err)
		}
	})

	t.Run("mark disconnected not found and transition", func(t *testing.T) {
		st := &authStoreStub{
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, repository.ErrNotFound },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if err := uc.MarkDisconnected(context.Background(), "r1", "u1"); err != nil {
			t.Fatalf("expected nil for not found, got %v", err)
		}

		st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, errors.New("get player failed") }
		if err := uc.MarkDisconnected(context.Background(), "r1", "u1"); err == nil || err.Error() != "get player failed" {
			t.Fatalf("expected get player failed, got %v", err)
		}

		p := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
		updated := false
		st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return p, nil }
		st.updateRoomPlayerFn = func(context.Context, *model.RoomPlayer) error { updated = true; return nil }
		if err := uc.MarkDisconnected(context.Background(), "r1", "u1"); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
		if !updated || p.Status != model.RoomPlayerDisconnected {
			t.Fatalf("expected disconnected update, updated=%v status=%s", updated, p.Status)
		}

		p.Status = model.RoomPlayerActive
		st.updateRoomPlayerFn = func(context.Context, *model.RoomPlayer) error { return errors.New("update disconnected failed") }
		if err := uc.MarkDisconnected(context.Background(), "r1", "u1"); err == nil || err.Error() != "update disconnected failed" {
			t.Fatalf("expected update disconnected failed, got %v", err)
		}
	})

	t.Run("mark disconnected no-op for left and disconnected", func(t *testing.T) {
		st := &authStoreStub{}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return left, nil }
		if err := uc.MarkDisconnected(context.Background(), "r1", "u1"); err != nil {
			t.Fatalf("expected nil for left player, got %v", err)
		}
		disc := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerDisconnected, JoinedAt: now}
		st.getRoomPlayerFn = func(context.Context, string, string) (*model.RoomPlayer, error) { return disc, nil }
		if err := uc.MarkDisconnected(context.Background(), "r1", "u1"); err != nil {
			t.Fatalf("expected nil for already disconnected, got %v", err)
		}
	})
}

func TestRoomUsecase_PlayerStand_AdditionalErrorBranches(t *testing.T) {
	now := time.Now().UTC()
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
		TurnDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
	}
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: ptrString("s1"), CreatedAt: now, UpdatedAt: now}

	t.Run("empty players returns not found", func(t *testing.T) {
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return nil, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); !errors.Is(err, repository.ErrNotFound) {
			t.Fatalf("expected not found, got %v", err)
		}
	})

	t.Run("dealer state error propagates", func(t *testing.T) {
		st := &authStoreStub{
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return []*model.PlayerState{{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "8", Suit: "H"}}}}, nil
			},
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return nil, errors.New("dealer missing") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "dealer missing" {
			t.Fatalf("expected dealer error, got %v", err)
		}
	})
}

func TestRoomUsecase_PlayerStand_TransactionBranches(t *testing.T) {
	now := time.Now().UTC()
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 3, TurnSeat: 1,
		TurnDeadlineAt: ptrTime(now.Add(-time.Second)), CreatedAt: now, UpdatedAt: now,
	}
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: ptrString("s1"), CreatedAt: now, UpdatedAt: now}
	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "8", Suit: "H"}}}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "10", Suit: "S"}}}

	makeBase := func() *authStoreStub {
		localSess := *sess
		localRoom := *room
		localPlayer := *player
		localDealer := *dealer
		st := &authStoreStub{
			getSessionFn:       func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getRoomFn:          func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{&localPlayer}, nil },
			getDealerStateFn:   func(context.Context, string) (*model.DealerState, error) { return &localDealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				return true, nil
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		return st
	}

	t.Run("version conflict on update session", func(t *testing.T) {
		st := makeBase()
		st.updateSessionIfVersionFn = func(context.Context, *model.GameSession, int64) (bool, error) { return false, nil }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); !errors.Is(err, model.ErrVersionConflict) {
			t.Fatalf("expected version conflict, got %v", err)
		}
	})

	t.Run("update player/dealer/room and snapshot errors", func(t *testing.T) {
		st := makeBase()
		st.updatePlayerStateFn = func(context.Context, *model.PlayerState) error { return errors.New("update player stand failed") }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "update player stand failed" {
			t.Fatalf("expected update player stand failed, got %v", err)
		}

		st = makeBase()
		st.updateDealerStateFn = func(context.Context, *model.DealerState) error { return errors.New("update dealer stand failed") }
		uc = NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "update dealer stand failed" {
			t.Fatalf("expected update dealer stand failed, got %v", err)
		}

		st = makeBase()
		st.updateRoomFn = func(context.Context, *model.Room) error { return errors.New("update room stand failed") }
		uc = NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "update room stand failed" {
			t.Fatalf("expected update room stand failed, got %v", err)
		}

		st = makeBase()
		st.createActionLogFn = func(context.Context, *model.ActionLog) error { return errors.New("auto stand snapshot failed") }
		uc = NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.playerStand(context.Background(), "s1"); err == nil || err.Error() != "auto stand snapshot failed" {
			t.Fatalf("expected auto stand snapshot failed, got %v", err)
		}
	})
}

func TestRoomUsecase_RematchUnanimousSuccessTx_PersistenceErrors(t *testing.T) {
	now := time.Now().UTC()
	prev := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusResetting, Version: 4, CreatedAt: now, UpdatedAt: now,
	}
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: ptrString("s1"), CreatedAt: now, UpdatedAt: now}

	makeBase := func() *authStoreStub {
		st := &authStoreStub{
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			createSessionFn:          func(context.Context, *model.GameSession) error { return nil },
			createPlayerStateFn:      func(context.Context, *model.PlayerState) error { return nil },
			createDealerStateFn:      func(context.Context, *model.DealerState) error { return nil },
			updateRoomFn:             func(context.Context, *model.Room) error { return nil },
		}
		return st
	}

	t.Run("version conflict and create errors", func(t *testing.T) {
		st := makeBase()
		st.updateSessionIfVersionFn = func(context.Context, *model.GameSession, int64) (bool, error) { return false, nil }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		prevLocal := *prev
		roomLocal := *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomLocal, &prevLocal, "u1", now, 4); !errors.Is(err, model.ErrVersionConflict) {
			t.Fatalf("expected version conflict, got %v", err)
		}

		st = makeBase()
		st.createSessionFn = func(context.Context, *model.GameSession) error { return errors.New("create rematch session failed") }
		uc = NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		prevLocal = *prev
		roomLocal = *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomLocal, &prevLocal, "u1", now, 4); err == nil || err.Error() != "create rematch session failed" {
			t.Fatalf("expected create rematch session failed, got %v", err)
		}
	})

	t.Run("create player/dealer and update room errors", func(t *testing.T) {
		st := makeBase()
		st.createPlayerStateFn = func(context.Context, *model.PlayerState) error { return errors.New("create rematch player failed") }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		prevLocal := *prev
		roomLocal := *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomLocal, &prevLocal, "u1", now, 4); err == nil || err.Error() != "create rematch player failed" {
			t.Fatalf("expected create rematch player failed, got %v", err)
		}

		st = makeBase()
		st.createDealerStateFn = func(context.Context, *model.DealerState) error { return errors.New("create rematch dealer failed") }
		uc = NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		prevLocal = *prev
		roomLocal = *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomLocal, &prevLocal, "u1", now, 4); err == nil || err.Error() != "create rematch dealer failed" {
			t.Fatalf("expected create rematch dealer failed, got %v", err)
		}

		st = makeBase()
		st.updateRoomFn = func(context.Context, *model.Room) error { return errors.New("update rematch room failed") }
		uc = NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		prevLocal = *prev
		roomLocal = *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomLocal, &prevLocal, "u1", now, 4); err == nil || err.Error() != "update rematch room failed" {
			t.Fatalf("expected update rematch room failed, got %v", err)
		}
	})

	t.Run("update previous session repository error", func(t *testing.T) {
		st := makeBase()
		st.updateSessionIfVersionFn = func(context.Context, *model.GameSession, int64) (bool, error) {
			return false, errors.New("prev session update repo failed")
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)
		prevLocal := *prev
		roomLocal := *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomLocal, &prevLocal, "u1", now, 4); err == nil || err.Error() != "prev session update repo failed" {
			t.Fatalf("expected prev session update repo failed, got %v", err)
		}
	})

	t.Run("initial deal failure on rematch", func(t *testing.T) {
		st := makeBase()
		uc := NewRoomUsecase(st, fixedEvaluator{}, failingEngine{applyErr: errors.New("rematch initial deal failed")}).(*roomService)
		prevLocal := *prev
		roomLocal := *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomLocal, &prevLocal, "u1", now, 4); err == nil || err.Error() != "rematch initial deal failed" {
			t.Fatalf("expected rematch initial deal failed, got %v", err)
		}
	})

	t.Run("blackjack after rematch initial deal advances to dealer turn", func(t *testing.T) {
		st := makeBase()
		uc := NewRoomUsecase(st, fixedEvaluator{blackjack: true}, appendEngine{}).(*roomService)
		prevLocal := *prev
		roomLocal := *room
		next, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomLocal, &prevLocal, "u1", now, 4)
		if err != nil {
			t.Fatalf("rematch unanimous success: %v", err)
		}
		if next.Status != model.SessionStatusDealerTurn {
			t.Fatalf("expected dealer turn after blackjack, got %s", next.Status)
		}
	})
}

func TestRoomUsecase_FinalizeRematchFailureTx_Errors(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{}).(*roomService)

	t.Run("list room players error propagates", func(t *testing.T) {
		st := &authStoreStub{
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return nil, errors.New("list players for finalize failed")
			},
		}
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
		if err := uc.finalizeRematchFailureTx(context.Background(), st, room); err == nil || err.Error() != "list players for finalize failed" {
			t.Fatalf("expected list players for finalize failed, got %v", err)
		}
	})

	t.Run("recalculate status error propagates", func(t *testing.T) {
		st := &authStoreStub{
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return []*model.RoomPlayer{
					{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now},
					{RoomID: "r1", UserID: "u2", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now},
				}, nil
			},
		}
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
		if err := uc.finalizeRematchFailureTx(context.Background(), st, room); !errors.Is(err, model.ErrRoomFull) {
			t.Fatalf("expected ErrRoomFull from RecalculateStatus, got %v", err)
		}
	})

	t.Run("update room error propagates", func(t *testing.T) {
		st := &authStoreStub{
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				return []*model.RoomPlayer{
					{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now},
				}, nil
			},
			updateRoomFn: func(context.Context, *model.Room) error { return errors.New("finalize update room failed") },
		}
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
		if err := uc.finalizeRematchFailureTx(context.Background(), st, room); err == nil || err.Error() != "finalize update room failed" {
			t.Fatalf("expected finalize update room failed, got %v", err)
		}
	})
}

func TestRoomUsecase_ResetRoomForDebug_Branches(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}

	t.Run("auth and input validation", func(t *testing.T) {
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{})
		if _, err := uc.ResetRoomForDebug(context.Background(), "r1", ""); !errors.Is(err, ErrUnauthorizedUser) {
			t.Fatalf("expected unauthorized, got %v", err)
		}
		if _, err := uc.ResetRoomForDebug(context.Background(), "", "u1"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected invalid input, got %v", err)
		}
	})

	t.Run("get room error inside transaction", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("reset get room failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.ResetRoomForDebug(context.Background(), "r1", "u1"); err == nil || err.Error() != "reset get room failed" {
			t.Fatalf("expected reset get room failed, got %v", err)
		}
	})

	t.Run("forbidden and delete error propagation", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn: func(context.Context, string) (*model.Room, error) {
				return &model.Room{ID: "r1", HostUserID: "other", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}, nil
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.ResetRoomForDebug(context.Background(), "r1", "u1"); !errors.Is(err, ErrForbiddenAction) {
			t.Fatalf("expected forbidden, got %v", err)
		}

		st.getRoomFn = func(context.Context, string) (*model.Room, error) { return room, nil }
		st.deleteGameSessionsByRoomIDFn = func(context.Context, string) error { return errors.New("delete session failed") }
		if _, err := uc.ResetRoomForDebug(context.Background(), "r1", "u1"); err == nil || err.Error() != "delete session failed" {
			t.Fatalf("expected delete session error, got %v", err)
		}

		st.deleteGameSessionsByRoomIDFn = func(context.Context, string) error { return nil }
		st.deleteRoomPlayersByRoomIDFn = func(context.Context, string) error { return errors.New("delete players failed") }
		if _, err := uc.ResetRoomForDebug(context.Background(), "r1", "u1"); err == nil || err.Error() != "delete players failed" {
			t.Fatalf("expected delete players error, got %v", err)
		}

		updateCalls := 0
		st.deleteRoomPlayersByRoomIDFn = func(context.Context, string) error { return nil }
		st.updateRoomFn = func(context.Context, *model.Room) error {
			updateCalls++
			if updateCalls == 1 {
				return errors.New("first update failed")
			}
			return nil
		}
		if _, err := uc.ResetRoomForDebug(context.Background(), "r1", "u1"); err == nil || err.Error() != "first update failed" {
			t.Fatalf("expected first update failed, got %v", err)
		}

		updateCalls = 0
		st.deleteRoomPlayersByRoomIDFn = func(context.Context, string) error { return nil }
		st.updateRoomFn = func(context.Context, *model.Room) error {
			updateCalls++
			if updateCalls == 2 {
				return errors.New("second update failed")
			}
			return nil
		}
		if _, err := uc.ResetRoomForDebug(context.Background(), "r1", "u1"); err == nil || err.Error() != "second update failed" {
			t.Fatalf("expected second update failed, got %v", err)
		}
	})
}

func TestRoomUsecase_GetRoomHistory_NonHostBranches(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CreatedAt: now, UpdatedAt: now}
	logs := []*model.RoundLog{{SessionID: "s1", RoundNo: 1, ResultPayload: `{"ok":true}`, CreatedAt: now}}

	t.Run("not found membership is forbidden", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, repository.ErrNotFound },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.GetRoomHistory(context.Background(), "r1", "u2"); !errors.Is(err, ErrForbiddenAction) {
			t.Fatalf("expected forbidden, got %v", err)
		}
	})

	t.Run("left membership is forbidden", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn: func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) {
				return &model.RoomPlayer{RoomID: "r1", UserID: "u2", SeatNo: 1, Status: model.RoomPlayerLeft, JoinedAt: now}, nil
			},
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.GetRoomHistory(context.Background(), "r1", "u2"); !errors.Is(err, ErrForbiddenAction) {
			t.Fatalf("expected forbidden, got %v", err)
		}
	})

	t.Run("active membership can read history", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn: func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) {
				return &model.RoomPlayer{RoomID: "r1", UserID: "u2", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}, nil
			},
			listRoundLogsByRoomIDFn: func(context.Context, string) ([]*model.RoundLog, error) { return logs, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		got, err := uc.GetRoomHistory(context.Background(), "r1", "u2")
		if err != nil || len(got) != 1 {
			t.Fatalf("unexpected history result: len=%d err=%v", len(got), err)
		}
	})
}

func TestRoomUsecase_GetRoomHistory_ValidationAndRepositoryErrors(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CreatedAt: now, UpdatedAt: now}

	uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{})
	if _, err := uc.GetRoomHistory(context.Background(), "r1", ""); !errors.Is(err, ErrUnauthorizedUser) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
	if _, err := uc.GetRoomHistory(context.Background(), "", "u1"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}

	t.Run("get room error", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("history room load failed") },
		}
		uc2 := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc2.GetRoomHistory(context.Background(), "r1", "u1"); err == nil || err.Error() != "history room load failed" {
			t.Fatalf("expected history room load failed, got %v", err)
		}
	})

	t.Run("non-host get room player error propagates", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn: func(context.Context, string, string) (*model.RoomPlayer, error) { return nil, errors.New("membership load failed") },
		}
		uc2 := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc2.GetRoomHistory(context.Background(), "r1", "u2"); err == nil || err.Error() != "membership load failed" {
			t.Fatalf("expected membership load failed, got %v", err)
		}
	})

	t.Run("host list round logs error", func(t *testing.T) {
		st := &authStoreStub{
			getRoomFn:               func(context.Context, string) (*model.Room, error) { return room, nil },
			listRoundLogsByRoomIDFn: func(context.Context, string) ([]*model.RoundLog, error) { return nil, errors.New("list logs failed") },
		}
		uc2 := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc2.GetRoomHistory(context.Background(), "r1", "u1"); err == nil || err.Error() != "list logs failed" {
			t.Fatalf("expected list logs failed, got %v", err)
		}
	})
}

func TestRoomUsecase_InitialDeal_ErrorBranches(t *testing.T) {
	now := time.Now().UTC()
	sess, err := model.NewGameSession("s1", "r1", 1, now)
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	p, _ := model.NewPlayerState("s1", "u1", 1)
	d, _ := model.NewDealerState("s1")

	t.Run("apply player hit error", func(t *testing.T) {
		local := *sess
		local.SetDeck([]model.StoredCard{{Rank: "A", Suit: "S"}, {Rank: "K", Suit: "H"}, {Rank: "Q", Suit: "D"}, {Rank: "J", Suit: "C"}})
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, failingEngine{applyErr: errors.New("apply failed")}).(*roomService)
		if err := uc.initialDeal(&local, p, d); err == nil || err.Error() != "apply failed" {
			t.Fatalf("expected apply failed, got %v", err)
		}
	})

	t.Run("deck exhausted during initial deal", func(t *testing.T) {
		local := *sess
		local.SetDeck([]model.StoredCard{{Rank: "A", Suit: "S"}, {Rank: "K", Suit: "H"}, {Rank: "Q", Suit: "D"}})
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{}, appendEngine{}).(*roomService)
		if err := uc.initialDeal(&local, p, d); !errors.Is(err, model.ErrDeckExhausted) {
			t.Fatalf("expected deck exhausted, got %v", err)
		}
	})
}

func TestRoomUsecase_DealerResult_ErrorBranches(t *testing.T) {
	now := time.Now().UTC()
	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "10", Suit: "S"}}}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "H"}, {Rank: "7", Suit: "D"}}}

	t.Run("invalid transition from player turn", func(t *testing.T) {
		sess := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, CreatedAt: now, UpdatedAt: now}
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{value: 17}, appendEngine{}).(*roomService)
		if _, err := uc.dealerresult(sess, player, dealer, now); !errors.Is(err, model.ErrInvalidTransition) {
			t.Fatalf("expected invalid transition, got %v", err)
		}
	})

	t.Run("resolve outcome error bubbles", func(t *testing.T) {
		sess := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusDealerTurn, Version: 1, CreatedAt: now, UpdatedAt: now}
		uc := NewRoomUsecase(&authStoreStub{}, fixedEvaluator{value: 17}, failingEngine{resolveErr: errors.New("resolve failed")}).(*roomService)
		if _, err := uc.dealerresult(sess, player, dealer, now); err == nil || err.Error() != "resolve failed" {
			t.Fatalf("expected resolve failed, got %v", err)
		}
	})
}

func TestRoomUsecase_DealerTurn_PersistenceErrors(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	baseSession := &model.GameSession{
		ID: sid, RoomID: "r1", RoundNo: 1, Status: model.SessionStatusDealerTurn, Version: 1,
		Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
	}
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	player := &model.PlayerState{SessionID: sid, UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "S"}}}
	dealer := &model.DealerState{SessionID: sid, Hand: []model.StoredCard{{Rank: "9", Suit: "H"}, {Rank: "8", Suit: "D"}}}

	t.Run("create round log error bubbles", func(t *testing.T) {
		local := *baseSession
		st := &authStoreStub{
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return &local, nil },
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return []*model.PlayerState{player}, nil
			},
			getDealerStateFn:         func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			createRoundLogFn:         func(context.Context, *model.RoundLog) error { return errors.New("create round log failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 17}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), sid); err == nil || err.Error() != "create round log failed" {
			t.Fatalf("expected create round log failed, got %v", err)
		}
	})

	t.Run("update room error bubbles", func(t *testing.T) {
		local := *baseSession
		st := &authStoreStub{
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return &local, nil },
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) {
				return []*model.PlayerState{player}, nil
			},
			getDealerStateFn:         func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			createRoundLogFn:         func(context.Context, *model.RoundLog) error { return nil },
			updateRoomFn:             func(context.Context, *model.Room) error { return errors.New("update room failed") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 17}, appendEngine{}).(*roomService)
		if err := uc.dealerTurn(context.Background(), sid); err == nil || err.Error() != "update room failed" {
			t.Fatalf("expected update room failed, got %v", err)
		}
	})
}

func TestRoomUsecase_Stand_Success(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "D"}}}
	updated := false
	st := &authStoreStub{
		getRoomFn:       func(context.Context, string) (*model.Room, error) { return room, nil },
		getSessionFn:    func(context.Context, string) (*model.GameSession, error) { return sess, nil },
		getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
		getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
			updated = true
			return true, nil
		},
		updatePlayerStateFn: func(context.Context, *model.PlayerState) error { return nil },
		updateDealerStateFn: func(context.Context, *model.DealerState) error { return nil },
		updateRoomFn:        func(context.Context, *model.Room) error { return nil },
		createActionLogFn:   func(context.Context, *model.ActionLog) error { return nil },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
	uc := NewRoomUsecase(st, fixedEvaluator{value: 10}, appendEngine{})
	_, out, err := uc.Stand(context.Background(), "r1", "u1", 1, "stand-ok-1")
	if err != nil {
		t.Fatalf("Stand: %v", err)
	}
	if !updated || out.Status != model.SessionStatusDealerTurn || out.Version != 2 || player.Status != model.PlayerStatusStand {
		t.Fatalf("unexpected stand result: updated=%v status=%s version=%d player=%s", updated, out.Status, out.Version, player.Status)
	}
}

func TestRoomUsecase_PlayerTurn_IdempotencyAndStateErrors(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	baseSess := func() *model.GameSession {
		return &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
	}
	basePlayer := func() *model.PlayerState {
		return &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "9", Suit: "H"}}}
	}
	dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "10", Suit: "D"}}}

	t.Run("get player state non-notfound error", func(t *testing.T) {
		sess := baseSess()
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return nil, errors.New("player lookup failed") },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a-pl"); err == nil || err.Error() != "player lookup failed" {
			t.Fatalf("expected player lookup failed, got %v", err)
		}
	})

	t.Run("action log lookup error", func(t *testing.T) {
		sess := baseSess()
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return basePlayer(), nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			getActionLogByIDFn: func(context.Context, string, string, string) (*model.ActionLog, error) {
				return nil, errors.New("action log lookup failed")
			},
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a-log-err"); err == nil || err.Error() != "action log lookup failed" {
			t.Fatalf("expected action log lookup failed, got %v", err)
		}
	})

	t.Run("duplicate action id with different payload hash", func(t *testing.T) {
		sess := baseSess()
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return basePlayer(), nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			getActionLogByIDFn: func(context.Context, string, string, string) (*model.ActionLog, error) {
				return &model.ActionLog{
					SessionID:          "s1",
					ActorType:          model.ActorTypeUser,
					ActorUserID:        "u1",
					ActionID:           "dup-id",
					RequestType:        "HIT",
					RequestPayloadHash: "wrong-hash-not-matching",
				}, nil
			},
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "dup-id"); !errors.Is(err, model.ErrDuplicateAction) {
			t.Fatalf("expected ErrDuplicateAction, got %v", err)
		}
	})

	t.Run("not your turn wrong seat", func(t *testing.T) {
		sess := baseSess()
		sess.TurnSeat = 2
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return basePlayer(), nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a-wrong-seat"); !errors.Is(err, model.ErrNotYourTurn) {
			t.Fatalf("expected ErrNotYourTurn, got %v", err)
		}
	})

	t.Run("hit deck exhausted", func(t *testing.T) {
		sess := baseSess()
		sess.SetDeck([]model.StoredCard{})
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return basePlayer(), nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a-deck"); !errors.Is(err, model.ErrDeckExhausted) {
			t.Fatalf("expected ErrDeckExhausted, got %v", err)
		}
	})

	t.Run("hit apply engine error", func(t *testing.T) {
		sess := baseSess()
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return basePlayer(), nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, failingEngine{applyErr: errors.New("apply hit failed")})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "a-apply"); err == nil || err.Error() != "apply hit failed" {
			t.Fatalf("expected apply hit failed, got %v", err)
		}
	})
}

func ptrTime(v time.Time) *time.Time { return &v }

