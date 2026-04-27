package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"blackjack/backend/model"
)

func TestMarshalStoredCardsHook_FromDomainErrors(t *testing.T) {
	prev := marshalStoredCardsHook
	t.Cleanup(func() { marshalStoredCardsHook = prev })
	marshalStoredCardsHook = func([]model.StoredCard) ([]byte, error) {
		return nil, errors.New("marshal boom")
	}

	ctx := context.Background()
	s := newSQLiteStore(t)
	now := time.Now().UTC()
	deck := []model.StoredCard{{Rank: "A", Suit: "S"}}
	sess := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, Deck: deck, TurnSeat: 1, CreatedAt: now, UpdatedAt: now}
	_ = s.CreateRoom(ctx, &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now})

	if err := s.CreateSession(ctx, sess); err == nil {
		t.Fatal("expected CreateSession marshal error")
	}
	if err := s.UpdateSession(ctx, sess); err == nil {
		t.Fatal("expected UpdateSession marshal error")
	}
	ok, err := s.UpdateSessionIfVersion(ctx, sess, 1)
	if err == nil || ok {
		t.Fatalf("expected UpdateSessionIfVersion marshal error, ok=%v err=%v", ok, err)
	}
	ps := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: deck}
	if err := s.CreatePlayerState(ctx, ps); err == nil {
		t.Fatal("expected CreatePlayerState marshal error")
	}
	if err := s.UpdatePlayerState(ctx, ps); err == nil {
		t.Fatal("expected UpdatePlayerState marshal error")
	}
	ds := &model.DealerState{SessionID: "s1", Hand: deck}
	if err := s.CreateDealerState(ctx, ds); err == nil {
		t.Fatal("expected CreateDealerState marshal error")
	}
	if err := s.UpdateDealerState(ctx, ds); err == nil {
		t.Fatal("expected UpdateDealerState marshal error")
	}
}

func TestStore_CreateRoundLog_InvalidID(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	now := time.Now().UTC()
	err := s.CreateRoundLog(ctx, &model.RoundLog{ID: "not-int", SessionID: "s1", RoundNo: 1, ResultPayload: `{}`, CreatedAt: now})
	if err == nil {
		t.Fatal("expected invalid round log id error")
	}
}

func TestStore_NotFoundPaths(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	if _, err := s.GetSessionForUpdate(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("GetSessionForUpdate: want ErrNotFound got %v", err)
	}
	if _, err := s.GetLatestSessionByRoomID(ctx, "no-room"); err != ErrNotFound {
		t.Fatalf("GetLatestSessionByRoomID: want ErrNotFound got %v", err)
	}
	if _, err := s.GetDealerState(ctx, "no-session"); err != ErrNotFound {
		t.Fatalf("GetDealerState: want ErrNotFound got %v", err)
	}
}

func TestStore_CorruptDB_MappingErrors(t *testing.T) {
	ctx := context.Background()
	s, gdb := newSQLiteStoreWithDB(t)
	now := time.Now().UTC()

	if err := s.CreateRoom(ctx, &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRoom(ctx, &model.Room{ID: "r2", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now.Add(time.Minute), UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE rooms SET status = ? WHERE id = ?`, "BAD", "r2").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListRoomsByUserID(ctx, "u1"); err == nil {
		t.Fatal("expected ListRoomsByUserID mapping error")
	}
	if _, err := s.GetRoom(ctx, "r2"); err == nil {
		t.Fatal("expected GetRoom mapping error")
	}

	if err := s.CreateRoomPlayer(ctx, &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateRoomPlayer(ctx, &model.RoomPlayer{RoomID: "r1", UserID: "u2", SeatNo: 2, Status: model.RoomPlayerActive, JoinedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE room_players SET status = ? WHERE room_id = ? AND user_id = ?`, "BAD", "r1", "u2").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListRoomPlayersByRoomID(ctx, "r1"); err == nil {
		t.Fatal("expected ListRoomPlayersByRoomID mapping error")
	}
	if _, err := s.GetRoomPlayer(ctx, "r1", "u2"); err == nil {
		t.Fatal("expected GetRoomPlayer mapping error")
	}

	deckOK := []model.StoredCard{{Rank: "A", Suit: "S"}}
	sess1 := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, Deck: deckOK, TurnSeat: 1, CreatedAt: now, UpdatedAt: now}
	sess2 := &model.GameSession{ID: "s2", RoomID: "r1", RoundNo: 2, Status: model.SessionStatusPlayerTurn, Version: 1, Deck: deckOK, TurnSeat: 1, CreatedAt: now.Add(time.Hour), UpdatedAt: now.Add(time.Hour)}
	if err := s.CreateSession(ctx, sess1); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, sess2); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE game_sessions SET deck = ? WHERE id = ?`, "not-json", "s2").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSession(ctx, "s2"); err == nil {
		t.Fatal("expected GetSession mapping error")
	}
	if _, err := s.GetSessionForUpdate(ctx, "s2"); err == nil {
		t.Fatal("expected GetSessionForUpdate mapping error")
	}
	if _, err := s.GetLatestSessionByRoomID(ctx, "r1"); err == nil {
		t.Fatal("expected GetLatestSessionByRoomID mapping error")
	}
	if _, err := s.ListSessionsByStatus(ctx, model.SessionStatusPlayerTurn); err == nil {
		t.Fatal("expected ListSessionsByStatus mapping error")
	}

	td := now.Add(-time.Hour)
	if err := gdb.Model(&GameSessionRecord{}).Where("id = ?", "s1").Updates(map[string]any{
		"turn_deadline_at": td,
		"deck":             []byte(`[]`),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE game_sessions SET deck = ? WHERE id = ?`, "not-json", "s1").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListSessionsByStatusAndDeadlineBefore(ctx, model.SessionStatusPlayerTurn, now); err == nil {
		t.Fatal("expected ListSessionsByStatusAndDeadlineBefore mapping error")
	}

	if err := gdb.Model(&GameSessionRecord{}).Where("id = ?", "s1").Updates(map[string]any{
		"status":              string(model.SessionStatusResetting),
		"turn_deadline_at":    nil,
		"rematch_deadline_at": now,
		"deck":                []byte(`[]`),
	}).Error; err != nil {
		t.Fatal(err)
	}
	sess3 := &model.GameSession{ID: "s3", RoomID: "r1", RoundNo: 3, Status: model.SessionStatusResetting, Version: 1, Deck: deckOK, TurnSeat: 1, RematchDeadlineAt: ptrTime(now), CreatedAt: now, UpdatedAt: now}
	if err := s.CreateSession(ctx, sess3); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE game_sessions SET deck = ? WHERE id = ?`, "not-json", "s3").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListResettingSessionsDueBy(ctx, now.Add(time.Hour)); err == nil {
		t.Fatal("expected ListResettingSessionsDueBy mapping error")
	}

	if err := gdb.Model(&GameSessionRecord{}).Where("id = ?", "s1").Updates(map[string]any{
		"status": string(model.SessionStatusPlayerTurn),
		"deck":   []byte(`[]`),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePlayerState(ctx, &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: deckOK}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePlayerState(ctx, &model.PlayerState{SessionID: "s1", UserID: "u3", SeatNo: 3, Status: model.PlayerStatusActive, Hand: deckOK}); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE player_states SET hand = ? WHERE session_id = ? AND user_id = ?`, "not-json", "s1", "u3").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPlayerState(ctx, "s1", "u3"); err == nil {
		t.Fatal("expected GetPlayerState mapping error")
	}
	if _, err := s.ListPlayerStatesBySessionID(ctx, "s1"); err == nil {
		t.Fatal("expected ListPlayerStatesBySessionID mapping error")
	}

	if err := s.CreateDealerState(ctx, &model.DealerState{SessionID: "s1", Hand: deckOK}); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE dealer_states SET hand = ? WHERE session_id = ?`, "not-json", "s1").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDealerState(ctx, "s1"); err == nil {
		t.Fatal("expected GetDealerState mapping error")
	}

	if err := s.CreateActionLog(ctx, &model.ActionLog{SessionID: "s1", ActorType: model.ActorTypeUser, ActorUserID: "u1", TargetUserID: "u1", ActionID: "a1", RequestType: "HIT", RequestPayloadHash: "h"}); err != nil {
		t.Fatal(err)
	}
	if err := gdb.Exec(`UPDATE action_logs SET actor_type = ? WHERE action_id = ?`, "BAD", "a1").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetActionLogByActionID(ctx, "s1", "u1", "a1"); err == nil {
		t.Fatal("expected GetActionLogByActionId mapping error")
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
