package dto

import (
	"testing"
	"time"

	"blackjack/backend/model"
)

func TestRoomHistoryItemFromDomain(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("JST", 9*3600))
	got := RoomHistoryItemFromDomain("s1", 2, "{}", at)
	if got.SessionID != "s1" || got.RoundNo != 2 || got.ResultPayload != "{}" {
		t.Fatalf("unexpected payload: %+v", got)
	}
	if got.CreatedAt != "2026-01-01T18:04:05Z" {
		t.Fatalf("unexpected created_at: %s", got.CreatedAt)
	}
}

func TestRoomFromDomain(t *testing.T) {
	if got := RoomFromDomain(nil); got != (RoomJSON{}) {
		t.Fatalf("nil room should map to zero value: %+v", got)
	}
	r := &model.Room{ID: "r1", Status: model.RoomStatusReady}
	got := RoomFromDomain(r)
	if got.ID != "r1" || got.Status != "READY" {
		t.Fatalf("unexpected room json: %+v", got)
	}
}

func TestSessionFromDomain(t *testing.T) {
	format := func(tt time.Time) string { return tt.UTC().Format(time.RFC3339) }
	if got := SessionFromDomain(nil, format); got != (SessionJSON{}) {
		t.Fatalf("nil session should map to zero value: %+v", got)
	}

	td := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	rd := td.Add(30 * time.Second)
	s := &model.GameSession{
		ID:                "ss1",
		Status:            model.SessionStatusPlayerTurn,
		Version:           7,
		RoundNo:           3,
		TurnSeat:          1,
		TurnDeadlineAt:    &td,
		RematchDeadlineAt: &rd,
	}
	got := SessionFromDomain(s, format)
	if got.ID != "ss1" || got.Status != "PLAYER_TURN" || got.Version != 7 || got.RoundNo != 3 || got.TurnSeat != 1 {
		t.Fatalf("unexpected base mapping: %+v", got)
	}
	if got.TurnDeadlineAt == nil || *got.TurnDeadlineAt != td.Format(time.RFC3339) {
		t.Fatalf("unexpected turn deadline: %+v", got.TurnDeadlineAt)
	}
	if got.RematchDeadlineAt == nil || *got.RematchDeadlineAt != rd.Format(time.RFC3339) {
		t.Fatalf("unexpected rematch deadline: %+v", got.RematchDeadlineAt)
	}

	gotNoFormat := SessionFromDomain(s, nil)
	if gotNoFormat.TurnDeadlineAt != nil || gotNoFormat.RematchDeadlineAt != nil {
		t.Fatalf("deadlines should be omitted when format function is nil: %+v", gotNoFormat)
	}
}
