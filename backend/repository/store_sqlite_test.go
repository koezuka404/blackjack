package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"blackjack/backend/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSQLiteStoreWithDB(t *testing.T) (Store, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:repo_%d?mode=memory&cache=shared", time.Now().UnixNano())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := gdb.AutoMigrate(
		&RoomRecord{}, &RoomPlayerRecord{}, &GameSessionRecord{}, &PlayerStateRecord{}, &DealerStateRecord{},
		&ActionLogRecord{}, &RematchVoteRecord{}, &RoundLogRecord{}, &UserRecord{}, &SessionRecord{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return NewPostgreSQLStore(gdb), gdb
}

func newSQLiteStore(t *testing.T) Store {
	s, _ := newSQLiteStoreWithDB(t)
	return s
}

func TestStore_SQLite_CoversCRUD(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	now := time.Now().UTC()


	err := s.Transaction(ctx, func(tx Store) error {
		return tx.CreateRoom(ctx, &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now})
	})
	if err != nil {
		t.Fatalf("transaction create room: %v", err)
	}
	if _, err := s.GetRoom(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	room, err := s.GetRoom(ctx, "r1")
	if err != nil {
		t.Fatalf("get room: %v", err)
	}
	room.Status = model.RoomStatusReady
	if err := s.UpdateRoom(ctx, room); err != nil {
		t.Fatalf("update room: %v", err)
	}
	if rooms, err := s.ListRoomsByUserID(ctx, "u1"); err != nil || len(rooms) != 1 {
		t.Fatalf("list rooms: len=%d err=%v", len(rooms), err)
	}
	if n, err := s.CountRooms(ctx); err != nil || n != 1 {
		t.Fatalf("count rooms: n=%d err=%v", n, err)
	}


	rp := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}
	if err := s.CreateRoomPlayer(ctx, rp); err != nil {
		t.Fatalf("create room player: %v", err)
	}
	if _, err := s.GetRoomPlayer(ctx, "r1", "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for room player, got %v", err)
	}
	gotRP, err := s.GetRoomPlayer(ctx, "r1", "u1")
	if err != nil {
		t.Fatalf("get room player: %v", err)
	}
	gotRP.Status = model.RoomPlayerDisconnected
	if err := s.UpdateRoomPlayer(ctx, gotRP); err != nil {
		t.Fatalf("update room player: %v", err)
	}
	if rps, err := s.ListRoomPlayersByRoomID(ctx, "r1"); err != nil || len(rps) != 1 {
		t.Fatalf("list room players: len=%d err=%v", len(rps), err)
	}


	deck := []model.StoredCard{{Rank: "A", Suit: "S"}, {Rank: "10", Suit: "H"}}
	sess := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, Deck: deck, TurnSeat: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := s.GetSession(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for session, got %v", err)
	}
	gotSess, err := s.GetSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	gotSess.Status = model.SessionStatusDealerTurn
	if err := s.UpdateSession(ctx, gotSess); err != nil {
		t.Fatalf("update session: %v", err)
	}
	ok, err := s.UpdateSessionIfVersion(ctx, gotSess, 999)
	if err != nil || ok {
		t.Fatalf("update if version mismatch: ok=%v err=%v", ok, err)
	}
	ok, err = s.UpdateSessionIfVersion(ctx, gotSess, gotSess.Version)
	if err != nil || !ok {
		t.Fatalf("update if version match: ok=%v err=%v", ok, err)
	}
	if _, err := s.GetSessionForUpdate(ctx, "s1"); err != nil {
		t.Fatalf("get session for update: %v", err)
	}
	if _, err := s.GetLatestSessionByRoomID(ctx, "r1"); err != nil {
		t.Fatalf("get latest session: %v", err)
	}
	td := now.Add(-time.Second)
	gotSess.TurnDeadlineAt = &td
	gotSess.Status = model.SessionStatusPlayerTurn
	if err := s.UpdateSession(ctx, gotSess); err != nil {
		t.Fatalf("update session deadline: %v", err)
	}
	if list, err := s.ListSessionsByStatusAndDeadlineBefore(ctx, model.SessionStatusPlayerTurn, now); err != nil || len(list) == 0 {
		t.Fatalf("list sessions by deadline: len=%d err=%v", len(list), err)
	}
	rd := now.Add(-time.Second)
	gotSess.Status = model.SessionStatusResetting
	gotSess.RematchDeadlineAt = &rd
	if err := s.UpdateSession(ctx, gotSess); err != nil {
		t.Fatalf("update session rematch deadline: %v", err)
	}
	if list, err := s.ListResettingSessionsDueBy(ctx, now); err != nil || len(list) == 0 {
		t.Fatalf("list resetting due: len=%d err=%v", len(list), err)
	}
	if list, err := s.ListSessionsByStatus(ctx, model.SessionStatusResetting); err != nil || len(list) == 0 {
		t.Fatalf("list sessions by status: len=%d err=%v", len(list), err)
	}
	if n, err := s.CountSessions(ctx); err != nil || n == 0 {
		t.Fatalf("count sessions: n=%d err=%v", n, err)
	}


	ps := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "9", Suit: "C"}}}
	if err := s.CreatePlayerState(ctx, ps); err != nil {
		t.Fatalf("create player state: %v", err)
	}
	if _, err := s.GetPlayerState(ctx, "s1", "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound player state, got %v", err)
	}
	gotPS, err := s.GetPlayerState(ctx, "s1", "u1")
	if err != nil {
		t.Fatalf("get player state: %v", err)
	}
	gotPS.Status = model.PlayerStatusStand
	if err := s.UpdatePlayerState(ctx, gotPS); err != nil {
		t.Fatalf("update player state: %v", err)
	}
	if list, err := s.ListPlayerStatesBySessionID(ctx, "s1"); err != nil || len(list) != 1 {
		t.Fatalf("list player states: len=%d err=%v", len(list), err)
	}

	ds := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "8", Suit: "D"}}, HoleHidden: true}
	if err := s.CreateDealerState(ctx, ds); err != nil {
		t.Fatalf("create dealer state: %v", err)
	}
	gotDS, err := s.GetDealerState(ctx, "s1")
	if err != nil {
		t.Fatalf("get dealer state: %v", err)
	}
	gotDS.HoleHidden = false
	if err := s.UpdateDealerState(ctx, gotDS); err != nil {
		t.Fatalf("update dealer state: %v", err)
	}

	al := &model.ActionLog{SessionID: "s1", ActorType: model.ActorTypeUser, ActorUserID: "u1", TargetUserID: "u1", ActionID: "a1", RequestType: "HIT", RequestPayloadHash: "h"}
	if err := s.CreateActionLog(ctx, al); err != nil {
		t.Fatalf("create action log: %v", err)
	}
	if _, err := s.GetActionLogByActionID(ctx, "s1", "u1", "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound action log, got %v", err)
	}
	if _, err := s.GetActionLogByActionID(ctx, "s1", "u1", "a1"); err != nil {
		t.Fatalf("get action log: %v", err)
	}

	rv := &model.RematchVote{SessionID: "s1", UserID: "u1", Agree: true}
	if err := s.UpsertRematchVote(ctx, rv); err != nil {
		t.Fatalf("upsert rematch vote: %v", err)
	}
	if list, err := s.ListRematchVotes(ctx, "s1"); err != nil || len(list) != 1 {
		t.Fatalf("list rematch votes: len=%d err=%v", len(list), err)
	}

	rl := &model.RoundLog{SessionID: "s1", RoundNo: 1, ResultPayload: `{"result":"WIN"}`, CreatedAt: now}
	if err := s.CreateRoundLog(ctx, rl); err != nil {
		t.Fatalf("create round log: %v", err)
	}
	if _, err := s.GetRoundLog(ctx, "s1", 999); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound round log, got %v", err)
	}
	if _, err := s.GetRoundLog(ctx, "s1", 1); err != nil {
		t.Fatalf("get round log: %v", err)
	}
	if list, err := s.ListRoundLogsByRoomID(ctx, "r1"); err != nil || len(list) != 1 {
		t.Fatalf("list round logs by room: len=%d err=%v", len(list), err)
	}


	u := &model.User{ID: "u1", Username: "alice", Email: "alice@example.com", PasswordHash: "hash", CreatedAt: now, UpdatedAt: now}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := s.CreateUser(ctx, u); err == nil {
		t.Fatal("expected duplicate user creation error")
	}
	if _, err := s.GetUserByUsername(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound user by username, got %v", err)
	}
	if _, err := s.GetUserByUsername(ctx, "alice"); err != nil {
		t.Fatalf("get user by username: %v", err)
	}
	if _, err := s.GetUserByEmail(ctx, "alice@example.com"); err != nil {
		t.Fatalf("get user by email: %v", err)
	}
	if _, err := s.GetUserByEmail(ctx, "missing@example.com"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound user by email, got %v", err)
	}
	if _, err := s.GetUserByID(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound user by id, got %v", err)
	}
	if _, err := s.GetUserByID(ctx, "u1"); err != nil {
		t.Fatalf("get user by id: %v", err)
	}

	authS := &model.Session{ID: "token1", UserID: "u1", ExpiresAt: now.Add(-time.Hour), CreatedAt: now}
	if err := s.UpsertSession(ctx, authS); err != nil {
		t.Fatalf("upsert session: %v", err)
	}
	if _, err := s.GetAuthSession(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound auth session, got %v", err)
	}
	if _, err := s.GetAuthSession(ctx, "token1"); err != nil {
		t.Fatalf("get auth session: %v", err)
	}
	if err := s.DeleteSession(ctx, "token1"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	_ = s.UpsertSession(ctx, &model.Session{ID: "token2", UserID: "u1", ExpiresAt: now.Add(time.Hour), CreatedAt: now})
	if err := s.DeleteSessionsByUserID(ctx, "u1"); err != nil {
		t.Fatalf("delete sessions by user: %v", err)
	}
	_ = s.UpsertSession(ctx, &model.Session{ID: "token3", UserID: "u1", ExpiresAt: now.Add(-time.Hour), CreatedAt: now})
	if err := s.DeleteExpiredSessions(ctx); err != nil {
		t.Fatalf("delete expired sessions: %v", err)
	}


	if err := s.DeleteRoomPlayersByRoomID(ctx, "r1"); err != nil {
		t.Fatalf("delete room players by room: %v", err)
	}
	if err := s.DeleteGameSessionsByRoomID(ctx, "r1"); err != nil {
		t.Fatalf("delete sessions by room: %v", err)
	}
}
