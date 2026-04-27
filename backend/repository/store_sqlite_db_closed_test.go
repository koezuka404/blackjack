package repository

import (
	"context"
	"testing"
	"time"

	"blackjack/backend/model"
)

func TestStore_ClosedDB_PropagatesErrors(t *testing.T) {
	ctx := context.Background()
	s, gdb := newSQLiteStoreWithDB(t)
	now := time.Now().UTC()

	deck := []model.StoredCard{{Rank: "A", Suit: "S"}}
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, Deck: deck, TurnSeat: 1, CreatedAt: now, UpdatedAt: now}
	if err := s.CreateRoom(ctx, room); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePlayerState(ctx, &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: deck}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDealerState(ctx, &model.DealerState{SessionID: "s1", Hand: deck}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateActionLog(ctx, &model.ActionLog{SessionID: "s1", ActorType: model.ActorTypeUser, ActorUserID: "u1", TargetUserID: "u1", ActionID: "a1", RequestType: "HIT", RequestPayloadHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertRematchVote(ctx, &model.RematchVote{SessionID: "s1", UserID: "u1", Agree: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRoundLog(ctx, &model.RoundLog{SessionID: "s1", RoundNo: 1, ResultPayload: `{}`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateUser(ctx, &model.User{ID: "u1", Username: "a", PasswordHash: "h", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertSession(ctx, &model.Session{ID: "tok", UserID: "u1", ExpiresAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRoomPlayer(ctx, &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}); err != nil {
		t.Fatal(err)
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}

	expectErr := func(label string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected error after DB close", label)
		}
	}

	expectErr("CreateRoom", s.CreateRoom(ctx, room))
	expectErr("UpdateRoom", s.UpdateRoom(ctx, room))
	expectErr("CreateRoomPlayer", s.CreateRoomPlayer(ctx, &model.RoomPlayer{RoomID: "r1", UserID: "u2", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now}))
	expectErr("UpdateRoomPlayer", s.UpdateRoomPlayer(ctx, &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}))
	expectErr("CreateSession", s.CreateSession(ctx, sess))
	expectErr("UpdateSession", s.UpdateSession(ctx, sess))
	ok, err := s.UpdateSessionIfVersion(ctx, sess, 1)
	if err == nil || ok {
		t.Fatalf("UpdateSessionIfVersion: want error and ok=false, got ok=%v err=%v", ok, err)
	}
	expectErr("CreatePlayerState", s.CreatePlayerState(ctx, &model.PlayerState{SessionID: "s1", UserID: "u9", SeatNo: 9, Status: model.PlayerStatusActive, Hand: deck}))
	expectErr("UpdatePlayerState", s.UpdatePlayerState(ctx, &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusStand, Hand: deck}))
	expectErr("CreateDealerState", s.CreateDealerState(ctx, &model.DealerState{SessionID: "s9", Hand: deck}))
	expectErr("UpdateDealerState", s.UpdateDealerState(ctx, &model.DealerState{SessionID: "s1", Hand: deck}))
	expectErr("CreateActionLog", s.CreateActionLog(ctx, &model.ActionLog{SessionID: "s1", ActorType: model.ActorTypeUser, ActorUserID: "u1", TargetUserID: "u1", ActionID: "a2", RequestType: "HIT", RequestPayloadHash: "h"}))
	expectErr("UpsertRematchVote", s.UpsertRematchVote(ctx, &model.RematchVote{SessionID: "s1", UserID: "u2", Agree: false}))
	expectErr("CreateRoundLog", s.CreateRoundLog(ctx, &model.RoundLog{SessionID: "s1", RoundNo: 9, ResultPayload: `{}`, CreatedAt: now}))
	expectErr("CreateUser", s.CreateUser(ctx, &model.User{ID: "u2", Username: "b", PasswordHash: "h", CreatedAt: now, UpdatedAt: now}))
	expectErr("UpsertSession", s.UpsertSession(ctx, &model.Session{ID: "t2", UserID: "u1", ExpiresAt: now, CreatedAt: now}))
}

func TestStore_ListQueries_DBClosed_FindErrors(t *testing.T) {
	ctx := context.Background()
	s, gdb := newSQLiteStoreWithDB(t)
	sqlDB, err := gdb.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().UTC()
	if _, err := s.ListRoomsByUserID(ctx, "u1"); err == nil {
		t.Fatal("expected ListRoomsByUserID error")
	}
	if _, err := s.ListRoomPlayersByRoomID(ctx, "r1"); err == nil {
		t.Fatal("expected ListRoomPlayersByRoomID error")
	}
	if _, err := s.ListSessionsByStatusAndDeadlineBefore(ctx, model.SessionStatusPlayerTurn, deadline); err == nil {
		t.Fatal("expected ListSessionsByStatusAndDeadlineBefore error")
	}
	if _, err := s.ListResettingSessionsDueBy(ctx, deadline); err == nil {
		t.Fatal("expected ListResettingSessionsDueBy error")
	}
	if _, err := s.ListSessionsByStatus(ctx, model.SessionStatusPlayerTurn); err == nil {
		t.Fatal("expected ListSessionsByStatus error")
	}
	if _, err := s.ListPlayerStatesBySessionID(ctx, "s1"); err == nil {
		t.Fatal("expected ListPlayerStatesBySessionID error")
	}
	if _, err := s.ListRematchVotes(ctx, "s1"); err == nil {
		t.Fatal("expected ListRematchVotes error")
	}
	if _, err := s.ListRoundLogsByRoomID(ctx, "r1"); err == nil {
		t.Fatal("expected ListRoundLogsByRoomID error")
	}
}
