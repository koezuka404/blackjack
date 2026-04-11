package blackjackadapter

import (
	"blackjack/backend/model"

	bj "github.com/ethanefung/blackjack"
	"github.com/ethanefung/cards"
)

type HandEvaluator struct{}

func NewHandEvaluator() *HandEvaluator {
	return &HandEvaluator{}
}

func (h *HandEvaluator) Value(hand []model.StoredCard) int {
	bh := toBlackjackHand(hand)
	return (&bh).Value()
}

func (h *HandEvaluator) IsBlackjack(hand []model.StoredCard) bool {
	bh := toBlackjackHand(hand)
	return len(bh) == 2 && (&bh).Value() == 21
}

func (h *HandEvaluator) IsBust(hand []model.StoredCard) bool {
	bh := toBlackjackHand(hand)
	return (&bh).Value() > 21
}

func (h *HandEvaluator) IsSoft(hand []model.StoredCard) bool {
	bh := toBlackjackHand(hand)
	hard := hardValue(hand)
	return (&bh).HasAce() && (&bh).Value() > hard
}

func toBlackjackHand(hand []model.StoredCard) bj.Hand {
	out := make(bj.Hand, 0, len(hand))
	for _, c := range hand {
		out = append(out, cards.Card{
			Suit: toSuit(c.Suit),
			Rank: toRank(c.Rank),
		})
	}
	return out
}

func hardValue(hand []model.StoredCard) int {
	total := 0
	for _, c := range hand {
		switch c.Rank {
		case "A":
			total += 1
		case "K", "Q", "J", "10":
			total += 10
		case "9":
			total += 9
		case "8":
			total += 8
		case "7":
			total += 7
		case "6":
			total += 6
		case "5":
			total += 5
		case "4":
			total += 4
		case "3":
			total += 3
		case "2":
			total += 2
		}
	}
	return total
}

func toSuit(v string) cards.Suit {
	switch v {
	case "S":
		return cards.Spades
	case "H":
		return cards.Hearts
	case "D":
		return cards.Diamonds
	case "C":
		return cards.Clubs
	default:
		return cards.Suitless
	}
}

func toRank(v string) cards.Rank {
	switch v {
	case "A":
		return cards.Ace
	case "2":
		return cards.Two
	case "3":
		return cards.Three
	case "4":
		return cards.Four
	case "5":
		return cards.Five
	case "6":
		return cards.Six
	case "7":
		return cards.Seven
	case "8":
		return cards.Eight
	case "9":
		return cards.Nine
	case "10":
		return cards.Ten
	case "J":
		return cards.Jack
	case "Q":
		return cards.Queen
	case "K":
		return cards.King
	default:
		return cards.Rankless
	}
}
