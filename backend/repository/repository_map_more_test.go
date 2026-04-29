package repository

import (
	"testing"
	"time"

	"blackjack/backend/model"
)

func TestTableNames(t *testing.T) {
	if (RoomRecord{}).TableName() != "rooms" ||
		(RoomPlayerRecord{}).TableName() != "room_players" ||
		(GameSessionRecord{}).TableName() != "game_sessions" ||
		(PlayerStateRecord{}).TableName() != "player_states" ||
		(DealerStateRecord{}).TableName() != "dealer_states" ||
		(ActionLogRecord{}).TableName() != "action_logs" ||
		(RematchVoteRecord{}).TableName() != "rematch_votes" ||
		(RoundLogRecord{}).TableName() != "round_logs" ||
		(UserRecord{}).TableName() != "users" ||
		(SessionRecord{}).TableName() != "sessions" {
		t.Fatal("table names mismatch")
	}
}

func TestMapFromToDomain_AllRecords(t *testing.T) {
	now := time.Now().UTC()
	outcome := model.OutcomeWin
	score := 21
	currentSessionID := "s1"
	rs := `{"ok":true}`
	leftAt := now

	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CurrentSessionID: &currentSessionID, CreatedAt: now, UpdatedAt: now}
	roomRec := roomRecordFromDomain(room)
	if _, err := roomRecordToDomain(roomRec); err != nil {
		t.Fatalf("room roundtrip: %v", err)
	}

	sess := &model.GameSession{
		ID:                "s1",
		RoomID:            "r1",
		RoundNo:           1,
		Status:            model.SessionStatusPlayerTurn,
		Version:           2,
		TurnSeat:          1,
		Deck:              []model.StoredCard{{Rank: "A", Suit: "S"}},
		DrawIndex:         0,
		ResultSnapshot:    &rs,
		RematchDeadlineAt: &now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	sessRec, _ := gameSessionRecordFromDomain(sess)
	if _, err := gameSessionRecordToDomain(sessRec); err != nil {
		t.Fatalf("session roundtrip: %v", err)
	}

	ps := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Hand: []model.StoredCard{{Rank: "10", Suit: "H"}}, Status: model.PlayerStatusActive, Outcome: &outcome, FinalScore: &score}
	psRec, _ := playerStateRecordFromDomain(ps)
	if _, err := playerStateRecordToDomain(psRec); err != nil {
		t.Fatalf("player roundtrip: %v", err)
	}

	ds := &model.DealerState{SessionID: "s1", Hand: []model.StoredCard{{Rank: "9", Suit: "D"}}, HoleHidden: true, FinalScore: &score}
	dsRec, _ := dealerStateRecordFromDomain(ds)
	if _, err := dealerStateRecordToDomain(dsRec); err != nil {
		t.Fatalf("dealer roundtrip: %v", err)
	}

	ur := &model.User{ID: "u1", Username: "alice", Email: "alice@example.com", PasswordHash: "h", CreatedAt: now, UpdatedAt: now}
	urRec := userRecordFromDomain(ur)
	if _, err := userRecordToDomain(urRec); err != nil {
		t.Fatalf("user roundtrip: %v", err)
	}

	as := &model.Session{ID: "token", UserID: "u1", ExpiresAt: now.Add(time.Hour), CreatedAt: now}
	asRec := authSessionRecordFromDomain(as)
	if _, err := authSessionRecordToDomain(asRec); err != nil {
		t.Fatalf("auth session roundtrip: %v", err)
	}

	rp := &model.RoomPlayer{RoomID: "r1", UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive, JoinedAt: now, LeftAt: &leftAt}
	rpRec := roomPlayerRecordFromDomain(rp)
	if _, err := roomPlayerRecordToDomain(rpRec); err != nil {
		t.Fatalf("room player roundtrip: %v", err)
	}

	al := &model.ActionLog{SessionID: "s1", ActorType: model.ActorTypeUser, ActorUserID: "u1", TargetUserID: "u1", ActionID: "a1", RequestType: "HIT", RequestPayloadHash: "h", ResponseSnapshot: `{"ok":1}`}
	alRec := actionLogRecordFromDomain(al)
	if _, err := actionLogRecordToDomain(alRec); err != nil {
		t.Fatalf("action log roundtrip: %v", err)
	}

	rv := &model.RematchVote{SessionID: "s1", UserID: "u1", Agree: true}
	rvRec := rematchVoteRecordFromDomain(rv)
	if rematchVoteRecordToDomain(rvRec) == nil {
		t.Fatal("expected rematch vote domain")
	}

	rl := &model.RoundLog{ID: "1", SessionID: "s1", RoundNo: 1, ResultPayload: `{"w":"u1"}`, CreatedAt: now}
	rlRec, _ := roundLogRecordFromDomain(rl)
	if roundLogRecordToDomain(rlRec) == nil {
		t.Fatal("expected round log domain")
	}
}

func TestMapNilInputs(t *testing.T) {
	if v := roomRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil room record")
	}
	if v, _ := roomRecordToDomain(nil); v != nil {
		t.Fatal("expected nil room domain")
	}
	if v, _ := gameSessionRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil session record")
	}
	if v, _ := gameSessionRecordToDomain(nil); v != nil {
		t.Fatal("expected nil session domain")
	}
	if v, _ := playerStateRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil player record")
	}
	if v, _ := playerStateRecordToDomain(nil); v != nil {
		t.Fatal("expected nil player domain")
	}
	if v, _ := dealerStateRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil dealer record")
	}
	if v, _ := dealerStateRecordToDomain(nil); v != nil {
		t.Fatal("expected nil dealer domain")
	}
	if v := userRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil user record")
	}
	if v, _ := userRecordToDomain(nil); v != nil {
		t.Fatal("expected nil user domain")
	}
	if v := authSessionRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil auth session record")
	}
	if v, _ := authSessionRecordToDomain(nil); v != nil {
		t.Fatal("expected nil auth session domain")
	}
	if v := roomPlayerRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil room player record")
	}
	if v, _ := roomPlayerRecordToDomain(nil); v != nil {
		t.Fatal("expected nil room player domain")
	}
	if v := actionLogRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil action log record")
	}
	if v, _ := actionLogRecordToDomain(nil); v != nil {
		t.Fatal("expected nil action log domain")
	}
	if v := rematchVoteRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil rematch vote record")
	}
	if v := rematchVoteRecordToDomain(nil); v != nil {
		t.Fatal("expected nil rematch vote domain")
	}
	if v, _ := roundLogRecordFromDomain(nil); v != nil {
		t.Fatal("expected nil round log record")
	}
	if v := roundLogRecordToDomain(nil); v != nil {
		t.Fatal("expected nil round log domain")
	}
}
