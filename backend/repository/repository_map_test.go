package repository

import (
	"errors"
	"testing"
	"time"

	"blackjack/backend/model"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestMapErr(t *testing.T) {
	if got := mapErr(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := mapErr(gorm.ErrRecordNotFound); !errors.Is(got, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", got)
	}
	if got := mapErr(&pgconn.PgError{Code: "23505"}); !errors.Is(got, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", got)
	}
	src := errors.New("boom")
	if got := mapErr(src); !errors.Is(got, src) {
		t.Fatalf("expected original error, got %v", got)
	}
}

func TestStoredCardsMarshalRoundtrip(t *testing.T) {
	b, err := marshalStoredCards(nil)
	if err != nil {
		t.Fatalf("marshal nil failed: %v", err)
	}
	if string(b) != "[]" {
		t.Fatalf("unexpected nil marshal: %s", string(b))
	}
	if out, err := unmarshalStoredCards(nil); err != nil || out != nil {
		t.Fatalf("unexpected nil unmarshal: out=%v err=%v", out, err)
	}
	_, err = unmarshalStoredCards([]byte("{bad json"))
	if err == nil {
		t.Fatal("expected json unmarshal error")
	}
}

func TestRoomRecordToDomain_ValidatesStatus(t *testing.T) {
	_, err := roomRecordToDomain(&RoomRecord{Status: "INVALID"})
	if err == nil {
		t.Fatal("expected invalid room status error")
	}
}

func TestGameSessionRecordToDomain_ValidatesStatus(t *testing.T) {
	_, err := gameSessionRecordToDomain(&GameSessionRecord{Status: "INVALID", Deck: []byte("[]")})
	if err == nil {
		t.Fatal("expected invalid session status error")
	}
}

func TestPlayerStateRecordToDomain_ValidatesStatusAndOutcome(t *testing.T) {
	_, err := playerStateRecordToDomain(&PlayerStateRecord{Status: "INVALID", Hand: []byte("[]")})
	if err == nil {
		t.Fatal("expected invalid player status error")
	}
	_, err = playerStateRecordToDomain(&PlayerStateRecord{
		Status:  string(model.PlayerStatusActive),
		Hand:    []byte("[]"),
		Outcome: ptr("INVALID"),
	})
	if err == nil {
		t.Fatal("expected invalid outcome error")
	}
}

func TestRoomPlayerAndActionLogRecordToDomain_Validations(t *testing.T) {
	_, err := roomPlayerRecordToDomain(&RoomPlayerRecord{Status: "INVALID"})
	if err == nil {
		t.Fatal("expected invalid room player status error")
	}
	_, err = actionLogRecordToDomain(&ActionLogRecord{ActorType: "INVALID"})
	if err == nil {
		t.Fatal("expected invalid actor type error")
	}
}

func TestRoundLogRecordFromDomain_InvalidID(t *testing.T) {
	_, err := roundLogRecordFromDomain(&model.RoundLog{ID: "not-number"})
	if err == nil {
		t.Fatal("expected parse error for invalid round log id")
	}
}

func TestBasicRecordRoundTrips(t *testing.T) {
	now := time.Now().UTC()
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}
	roomRec, err := roomRecordFromDomain(room)
	if err != nil {
		t.Fatalf("roomRecordFromDomain failed: %v", err)
	}
	if _, err := roomRecordToDomain(roomRec); err != nil {
		t.Fatalf("roomRecordToDomain failed: %v", err)
	}

	player := &model.PlayerState{SessionID: "s1", UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "A", Suit: "S"}}}
	playerRec, err := playerStateRecordFromDomain(player)
	if err != nil {
		t.Fatalf("playerStateRecordFromDomain failed: %v", err)
	}
	if _, err := playerStateRecordToDomain(playerRec); err != nil {
		t.Fatalf("playerStateRecordToDomain failed: %v", err)
	}

	roundRec, err := roundLogRecordFromDomain(&model.RoundLog{
		ID:            "1",
		SessionID:     "s1",
		RoundNo:       1,
		ResultPayload: `{"result":"WIN"}`,
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("roundLogRecordFromDomain failed: %v", err)
	}
	if _, err := roundLogRecordToDomain(roundRec); err != nil {
		t.Fatalf("roundLogRecordToDomain failed: %v", err)
	}
}

func ptr(v string) *string { return &v }

