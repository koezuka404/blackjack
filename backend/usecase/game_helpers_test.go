package usecase

import (
	"testing"

	"blackjack/backend/model"
)

func TestSelectNextHost(t *testing.T) {
	players := []*model.RoomPlayer{
		{UserID: "u3", SeatNo: 3, Status: model.RoomPlayerActive},
		{UserID: "u2", SeatNo: 2, Status: model.RoomPlayerDisconnected},
		{UserID: "u1", SeatNo: 1, Status: model.RoomPlayerActive},
	}
	next := selectNextHost(players, "u1")
	if next == nil || next.UserID != "u2" {
		t.Fatalf("unexpected next host: %+v", next)
	}
	if got := selectNextHost(players, "u3"); got == nil || got.UserID != "u1" {
		t.Fatalf("unexpected next host for leaving u3: %+v", got)
	}
	if got := selectNextHost([]*model.RoomPlayer{{UserID: "u1", SeatNo: 1, Status: model.RoomPlayerLeft}}, "u1"); got != nil {
		t.Fatalf("expected nil next host, got: %+v", got)
	}
}

func TestRematchHelpers(t *testing.T) {
	players := []*model.RoomPlayer{
		{UserID: "u1", Status: model.RoomPlayerActive},
		{UserID: "u2", Status: model.RoomPlayerDisconnected},
		{UserID: "u3", Status: model.RoomPlayerLeft},
	}
	eligible := rematchEligibleUserIDs(players)
	if len(eligible) != 2 || eligible[0] != "u1" || eligible[1] != "u2" {
		t.Fatalf("unexpected eligible users: %+v", eligible)
	}

	votes := []*model.RematchVote{
		{UserID: "u1", Agree: true},
	}
	agreeMap := rematchAgreeMapAtDeadline(eligible, votes)
	if !agreeMap["u1"] || agreeMap["u2"] {
		t.Fatalf("unexpected agree map: %+v", agreeMap)
	}

	if !hasExplicitRematchDenial(eligible, agreeMap) {
		t.Fatal("expected explicit denial due to u2=false")
	}
	if hasExplicitRematchDenial([]string{"u1"}, map[string]bool{"u1": true}) {
		t.Fatal("did not expect denial when everyone agrees")
	}
}

func TestNewShuffledDeck_Basic(t *testing.T) {
	deck := newShuffledDeck()
	if len(deck) != 52 {
		t.Fatalf("unexpected deck length: %d", len(deck))
	}
	seen := make(map[string]bool, 52)
	for _, c := range deck {
		key := c.Rank + c.Suit
		if seen[key] {
			t.Fatalf("duplicate card found: %s", key)
		}
		seen[key] = true
	}
	if len(seen) != 52 {
		t.Fatalf("expected 52 unique cards, got %d", len(seen))
	}
}

