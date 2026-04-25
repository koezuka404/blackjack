package blackjackadapter

import (
	"testing"

	"blackjack/backend/model"

	bj "github.com/ethanefung/blackjack"
	"github.com/ethanefung/cards"
)

func TestCardConversionHelpers(t *testing.T) {
	for _, rank := range []cards.Rank{
		cards.Ace, cards.Two, cards.Three, cards.Four, cards.Five, cards.Six,
		cards.Seven, cards.Eight, cards.Nine, cards.Ten, cards.Jack, cards.Queen, cards.King,
	} {
		if got := toRank(rankToString(rank)); got != rank {
			t.Fatalf("rank roundtrip failed: %v -> %v", rank, got)
		}
	}

	for _, suit := range []cards.Suit{cards.Spades, cards.Hearts, cards.Diamonds, cards.Clubs} {
		if got := toSuit(suitToString(suit)); got != suit {
			t.Fatalf("suit roundtrip failed: %v -> %v", suit, got)
		}
	}

	if got := rankToString(cards.Rankless); got != "2" {
		t.Fatalf("unexpected fallback rank string: %s", got)
	}
	if got := suitToString(cards.Suitless); got != "S" {
		t.Fatalf("unexpected fallback suit string: %s", got)
	}
	if got := toRank("??"); got != cards.Rankless {
		t.Fatalf("unexpected unknown rank mapping: %v", got)
	}
	if got := toSuit("??"); got != cards.Suitless {
		t.Fatalf("unexpected unknown suit mapping: %v", got)
	}
}

func TestBlackjackHandConversions(t *testing.T) {
	stored := []model.StoredCard{
		{Rank: "A", Suit: "S"},
		{Rank: "10", Suit: "H"},
	}
	h := toBlackjackHand(stored)
	back := fromBlackjackHand(h)
	if len(back) != 2 {
		t.Fatalf("unexpected converted length: %d", len(back))
	}
	if back[0].Rank != "A" || back[1].Rank != "10" {
		t.Fatalf("unexpected converted cards: %+v", back)
	}

	storedCard := cardToStored(cards.Card{Suit: cards.Diamonds, Rank: cards.King})
	if storedCard.Rank != "K" || storedCard.Suit != "D" {
		t.Fatalf("unexpected stored card: %+v", storedCard)
	}
}

func TestRoundEngineApplyAndOutcome(t *testing.T) {
	engine := NewRoundEngine()
	hand, err := engine.ApplyPlayerHit(
		[]model.StoredCard{{Rank: "9", Suit: "S"}},
		model.StoredCard{Rank: "2", Suit: "H"},
	)
	if err != nil {
		t.Fatalf("apply hit failed: %v", err)
	}
	if len(hand) != 2 {
		t.Fatalf("unexpected hand size: %d", len(hand))
	}

	ev := NewHandEvaluator()
	outcome, err := engine.ResolveOutcome(
		ev,
		[]model.StoredCard{{Rank: "K", Suit: "S"}, {Rank: "9", Suit: "H"}},
		[]model.StoredCard{{Rank: "10", Suit: "C"}, {Rank: "7", Suit: "D"}},
	)
	if err != nil {
		t.Fatalf("resolve outcome failed: %v", err)
	}
	if outcome != model.OutcomeWin {
		t.Fatalf("unexpected outcome: %s", outcome)
	}
}

func TestHandEvaluatorHardValueBranches(t *testing.T) {
	ev := NewHandEvaluator()
	hand := []model.StoredCard{
		{Rank: "A", Suit: "S"},
		{Rank: "K", Suit: "H"},
		{Rank: "Q", Suit: "D"},
		{Rank: "J", Suit: "C"},
		{Rank: "10", Suit: "S"},
		{Rank: "9", Suit: "H"},
		{Rank: "8", Suit: "D"},
		{Rank: "7", Suit: "C"},
		{Rank: "6", Suit: "S"},
		{Rank: "5", Suit: "H"},
		{Rank: "4", Suit: "D"},
		{Rank: "3", Suit: "C"},
		{Rank: "2", Suit: "S"},
	}
	if hard := hardValue(hand); hard != 85 {
		t.Fatalf("unexpected hard value: %d", hard)
	}
	if !ev.IsSoft([]model.StoredCard{{Rank: "A", Suit: "S"}, {Rank: "6", Suit: "H"}}) {
		t.Fatal("A+6 should be soft")
	}
}

func TestFromBlackjackHand_WithLibraryType(t *testing.T) {
	h := bj.Hand{
		cards.Card{Suit: cards.Spades, Rank: cards.Ace},
		cards.Card{Suit: cards.Clubs, Rank: cards.Nine},
	}
	out := fromBlackjackHand(h)
	if len(out) != 2 {
		t.Fatalf("unexpected output: %+v", out)
	}
}

