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

func TestCoverage_StartRoom_BlackjackTransitionToDealerFails(t *testing.T) {
	now := time.Now().UTC()
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	localRoom := model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
	localPlayer := model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
	st := &authStoreStub{
		getRoomFn:          func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
		getRoomPlayerFn:    func(context.Context, string, string) (*model.RoomPlayer, error) { return &localPlayer, nil },
		getLatestSessionFn: func(context.Context, string) (*model.GameSession, error) { return nil, repository.ErrNotFound },
	}
	st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }

	def := gameSessionTransition
	gameSessionTransition = func(s *model.GameSession, next model.SessionStatus) error {
		if next == model.SessionStatusDealerTurn {
			return errors.New("bj dealer transition")
		}
		return def(s, next)
	}
	uc := NewRoomUsecase(st, fixedEvaluator{blackjack: true}, appendEngine{})
	if _, _, err := uc.StartRoom(context.Background(), "r1", "u1"); err == nil || err.Error() != "bj dealer transition" {
		t.Fatalf("got %v", err)
	}
}

func TestCoverage_PlayerTurnBranches(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("DrawCard error on hit", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "5", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "7", Suit: "D"}}}
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{value: 10}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "h-draw"); !errors.Is(err, model.ErrDeckExhausted) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("ApplyPlayerHit error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "5", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "7", Suit: "D"}}}
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{value: 10}, failingEngine{applyErr: errors.New("hit apply")})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "h-apply"); err == nil || err.Error() != "hit apply" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("hit blackjack path", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
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
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "h-bj"); err != nil {
			t.Fatalf("hit bj: %v", err)
		}
	})

	t.Run("hit then still player turn sets deadline", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
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
		uc := NewRoomUsecase(st, fixedEvaluator{value: 7, blackjack: false}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "h-continue"); err != nil {
			t.Fatalf("hit continue: %v", err)
		}
		if sess.Status != model.SessionStatusPlayerTurn || sess.TurnDeadlineAt == nil {
			t.Fatalf("expected player turn with deadline, got %+v", sess)
		}
	})

	t.Run("stand transition and setStatus errors", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "7", Suit: "D"}}}
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		defPS := playerStateSetStatus
		playerStateSetStatus = func(*model.PlayerState, model.PlayerStatus) error { return errors.New("stand set") }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.Stand(context.Background(), "r1", "u1", 1, "s1"); err == nil || err.Error() != "stand set" {
			t.Fatalf("got %v", err)
		}
		playerStateSetStatus = defPS
		defGT := gameSessionTransition
		gameSessionTransition = func(*model.GameSession, model.SessionStatus) error { return errors.New("stand tx") }
		if _, _, err := uc.Stand(context.Background(), "r1", "u1", 1, "s2"); err == nil || err.Error() != "stand tx" {
			t.Fatalf("got %v", err)
		}
		gameSessionTransition = defGT
	})

	t.Run("room recalc error on hit", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "8", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "7", Suit: "D"}}}
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		roomRecalculateStatus = func(*model.Room, int, bool) error { return errors.New("pt recalc") }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 10}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "h-rec"); err == nil || err.Error() != "pt recalc" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("update room error on hit", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "8", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "7", Suit: "D"}}}
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			updateRoomFn:             func(context.Context, *model.Room) error { return errors.New("pt room") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 10}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "h-room"); err == nil || err.Error() != "pt room" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("marshal error on hit", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "2", Suit: "C"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "8", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "7", Suit: "D"}}}
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		marshalGameJSON = func(any) ([]byte, error) { return nil, errors.New("pt marshal") }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 10}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "h-m"); err == nil || err.Error() != "pt marshal" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_GetRoomStateAndSuggestErrors(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{ID: "s1", RoomID: "r1", Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1, CreatedAt: now, UpdatedAt: now}

	t.Run("GetRoomState propagates GetRoom error", func(t *testing.T) {
		st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("grs") }}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.GetRoomState(context.Background(), "r1", "u1"); err == nil || err.Error() != "grs" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("GetRoomState nil session short path", func(t *testing.T) {
		waiting := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return waiting, nil }}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		state, err := uc.GetRoomState(context.Background(), "r1", "u1")
		if err != nil || state.Session != nil {
			t.Fatalf("got err=%v session=%v", err, state.Session)
		}
	})

	t.Run("SuggestPlayerAction GetRoomState error", func(t *testing.T) {
		st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("suggest gr") }}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.SuggestPlayerAction(context.Background(), "r1", "u1"); err == nil || err.Error() != "suggest gr" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("SuggestPlayerAction invalid when no session", func(t *testing.T) {
		waiting := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}
		st := &authStoreStub{getRoomFn: func(context.Context, string) (*model.Room, error) { return waiting, nil }}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.SuggestPlayerAction(context.Background(), "r1", "u1"); !errors.Is(err, ErrInvalidGameState) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("SuggestPlayerAction invalid when cannot hit", func(t *testing.T) {
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}}
		st := &authStoreStub{
			getRoomFn:    func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn: func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) {
				return &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "6", Suit: "S"}}}, nil
			},
			listPlayerStatesFn: func(context.Context, string) ([]*model.PlayerState, error) { return []*model.PlayerState{player}, nil },
		}
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, err := uc.SuggestPlayerAction(context.Background(), "r1", "u1"); !errors.Is(err, ErrInvalidGameState) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_RematchUnanimousModelInjections(t *testing.T) {
	now := time.Now().UTC()
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)
	prev := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusResetting, Version: 4, CreatedAt: now, UpdatedAt: now}
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: ptrString("s1"), CreatedAt: now, UpdatedAt: now}
	st := &authStoreStub{
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
		createSessionFn:          func(context.Context, *model.GameSession) error { return nil },
		createPlayerStateFn:      func(context.Context, *model.PlayerState) error { return nil },
		createDealerStateFn:      func(context.Context, *model.DealerState) error { return nil },
		updateRoomFn:             func(context.Context, *model.Room) error { return nil },
	}
	uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{}).(*roomService)

	t.Run("NewGameSession", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		newGameSessionUC = func(string, string, int, time.Time) (*model.GameSession, error) { return nil, errors.New("r-ngs") }
		prevL := *prev
		roomL := *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomL, &prevL, "u1", now, 4); err == nil || err.Error() != "r-ngs" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("NewDealerState", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		newDealerStateUC = func(string) (*model.DealerState, error) { return nil, errors.New("r-nds") }
		prevL := *prev
		roomL := *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomL, &prevL, "u1", now, 4); err == nil || err.Error() != "r-nds" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("NewPlayerState", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		newPlayerStateUC = func(string, string, int) (*model.PlayerState, error) { return nil, errors.New("r-nps") }
		prevL := *prev
		roomL := *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomL, &prevL, "u1", now, 4); err == nil || err.Error() != "r-nps" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("Transition to player turn", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		gameSessionTransition = func(*model.GameSession, model.SessionStatus) error { return errors.New("r-tpt") }
		prevL := *prev
		roomL := *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomL, &prevL, "u1", now, 4); err == nil || err.Error() != "r-tpt" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("rematch blackjack dealer transition error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		def := gameSessionTransition
		gameSessionTransition = func(s *model.GameSession, next model.SessionStatus) error {
			if next == model.SessionStatusDealerTurn && s.Status == model.SessionStatusPlayerTurn {
				return errors.New("r-bj-dealer")
			}
			return def(s, next)
		}
		prevL := *prev
		roomL := *room
		ucBJ := NewRoomUsecase(st, fixedEvaluator{blackjack: true}, appendEngine{}).(*roomService)
		if _, err := ucBJ.rematchUnanimousSuccessTx(context.Background(), st, &roomL, &prevL, "u1", now, 4); err == nil || err.Error() != "r-bj-dealer" {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("room recalc error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		roomRecalculateStatus = func(*model.Room, int, bool) error { return errors.New("r-room") }
		prevL := *prev
		roomL := *room
		if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomL, &prevL, "u1", now, 4); err == nil || err.Error() != "r-room" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_VoteRematchUnanimousReplayAndBranches(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	session := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusResetting, Version: 1, CreatedAt: now, UpdatedAt: now}
	players := []*model.RoomPlayer{
		{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now},
		{RoomID: "r1", UserID: "u2", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now},
	}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("replay short-circuits transaction body", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		payload := "REMATCH_VOTE:" + strconv.FormatBool(true) + ":" + strconv.FormatInt(1, 10)
		hash := sha256.Sum256([]byte(payload))
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return room, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return session, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return session, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return players, nil },
			listRematchVotesFn:   func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
			getActionLogByIDFn: func(context.Context, string, string, string) (*model.ActionLog, error) {
				return &model.ActionLog{RequestPayloadHash: hex.EncodeToString(hash[:]), ResponseSnapshot: "{}"}, nil
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "replay-v"); err != nil {
			t.Fatalf("replay vote: %v", err)
		}
	})

	t.Run("unanimous rematch success in vote", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		localSess := *session
		localRoom := *room
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return players, nil },
			listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) {
				return []*model.RematchVote{{SessionID: "s1", UserID: "u2", Agree: true}}, nil
			},
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			createSessionFn:          func(context.Context, *model.GameSession) error { return nil },
			createPlayerStateFn:      func(context.Context, *model.PlayerState) error { return nil },
			createDealerStateFn:      func(context.Context, *model.DealerState) error { return nil },
			updateRoomFn:             func(context.Context, *model.Room) error { return nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "uni-v"); err != nil {
			t.Fatalf("unanimous: %v", err)
		}
	})

	t.Run("default branch version conflict", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		localSess := *session
		localRoom := *room
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return players, nil },
			listRematchVotesFn:    func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return false, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "def-vc"); !errors.Is(err, model.ErrVersionConflict) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("marshal snapshot error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		localSess := *session
		localRoom := *room
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return players, nil },
			listRematchVotesFn:    func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		marshalGameJSON = func(any) ([]byte, error) { return nil, errors.New("vote marshal") }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "m-v"); err == nil || err.Error() != "vote marshal" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("EnsureActionIdempotency error in vote tx", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		localSess := *session
		localRoom := *room
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return players, nil },
			getActionLogByIDFn: func(context.Context, string, string, string) (*model.ActionLog, error) {
				return nil, errors.New("idem err")
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "idem-fail"); err == nil || err.Error() != "idem err" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("unanimous path rematchUnanimousSuccessTx error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		localSess := *session
		localRoom := *room
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return players, nil },
			listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) {
				return []*model.RematchVote{{SessionID: "s1", UserID: "u2", Agree: true}}, nil
			},
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
			createSessionFn:          func(context.Context, *model.GameSession) error { return errors.New("uni inner") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "uni-err"); err == nil || err.Error() != "uni inner" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("denial path finalize error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		localSess := *session
		localRoom := *room
		listCalls := 0
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			listRoomPlayersFn: func(context.Context, string) ([]*model.RoomPlayer, error) {
				listCalls++
				if listCalls == 1 {
					return players, nil
				}
				return []*model.RoomPlayer{{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}}, nil
			},
			listRematchVotesFn: func(context.Context, string) ([]*model.RematchVote, error) {
				return []*model.RematchVote{{SessionID: "s1", UserID: "u2", Agree: true}}, nil
			},
			updateRoomFn: func(context.Context, *model.Room) error { return errors.New("deny finalize") },
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", false, 1, "deny-v"); err == nil || err.Error() != "deny finalize" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("default branch update session repo error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		localSess := *session
		localRoom := *room
		st := &authStoreStub{
			getRoomFn:             func(context.Context, string) (*model.Room, error) { return &localRoom, nil },
			getRoomPlayerFn:       func(context.Context, string, string) (*model.RoomPlayer, error) { return players[0], nil },
			getLatestSessionFn:    func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			getSessionForUpdateFn: func(context.Context, string) (*model.GameSession, error) { return &localSess, nil },
			listRoomPlayersFn:     func(context.Context, string) ([]*model.RoomPlayer, error) { return players, nil },
			listRematchVotesFn:    func(context.Context, string) ([]*model.RematchVote, error) { return nil, repository.ErrNotFound },
			updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) {
				return false, errors.New("upd sess err")
			},
		}
		st.transactionFn = func(ctx context.Context, fn func(txStore repository.Store) error) error { return fn(st) }
		uc := NewRoomUsecase(st, fixedEvaluator{}, appendEngine{})
		if _, _, err := uc.VoteRematch(context.Background(), "r1", "u1", true, 1, "def-repo"); err == nil || err.Error() != "upd sess err" {
			t.Fatalf("got %v", err)
		}
	})
}

func TestCoverage_PlayerTurnBustHookErrors(t *testing.T) {
	now := time.Now().UTC()
	sid := "s1"
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: &sid, CreatedAt: now, UpdatedAt: now}
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)

	t.Run("bust SetStatus error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "K", Suit: "S"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "Q", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "5", Suit: "D"}}}
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		playerStateSetStatus = func(*model.PlayerState, model.PlayerStatus) error { return errors.New("bust set") }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 22}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "bust-set"); err == nil || err.Error() != "bust set" {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("bust Transition error", func(t *testing.T) {
		restoreUsecaseHooks(baseline)
		sess := &model.GameSession{
			ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1,
			Deck: []model.StoredCard{{Rank: "K", Suit: "S"}}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
		}
		player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "Q", Suit: "H"}}}
		dealer := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "5", Suit: "D"}}}
		st := &authStoreStub{
			getRoomFn:        func(context.Context, string) (*model.Room, error) { return room, nil },
			getSessionFn:     func(context.Context, string) (*model.GameSession, error) { return sess, nil },
			getPlayerStateFn: func(context.Context, string, string) (*model.PlayerState, error) { return player, nil },
			getDealerStateFn: func(context.Context, string) (*model.DealerState, error) { return dealer, nil },
		}
		def := gameSessionTransition
		gameSessionTransition = func(*model.GameSession, model.SessionStatus) error { return errors.New("bust tx") }
		uc := NewRoomUsecase(st, fixedEvaluator{value: 22}, appendEngine{})
		if _, _, err := uc.Hit(context.Background(), "r1", "u1", 1, "bust-tx"); err == nil || err.Error() != "bust tx" {
			t.Fatalf("got %v", err)
		}
		gameSessionTransition = def
	})
}

func TestCoverage_RematchBlackjackSetStatusError(t *testing.T) {
	now := time.Now().UTC()
	baseline := captureUsecaseHooks()
	defer restoreUsecaseHooks(baseline)
	prev := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusResetting, Version: 4, CreatedAt: now, UpdatedAt: now}
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CurrentSessionID: ptrString("s1"), CreatedAt: now, UpdatedAt: now}
	st := &authStoreStub{
		updateSessionIfVersionFn: func(context.Context, *model.GameSession, int64) (bool, error) { return true, nil },
		createSessionFn:          func(context.Context, *model.GameSession) error { return nil },
		createPlayerStateFn:      func(context.Context, *model.PlayerState) error { return nil },
		createDealerStateFn:      func(context.Context, *model.DealerState) error { return nil },
		updateRoomFn:             func(context.Context, *model.Room) error { return nil },
	}
	def := playerStateSetStatus
	playerStateSetStatus = func(p *model.PlayerState, st model.PlayerStatus) error {
		if st == model.PlayerStatusBlackjack {
			return errors.New("rem bj set")
		}
		return def(p, st)
	}
	uc := NewRoomUsecase(st, fixedEvaluator{blackjack: true}, appendEngine{}).(*roomService)
	prevL := *prev
	roomL := *room
	if _, err := uc.rematchUnanimousSuccessTx(context.Background(), st, &roomL, &prevL, "u1", now, 4); err == nil || err.Error() != "rem bj set" {
		t.Fatalf("got %v", err)
	}
}
