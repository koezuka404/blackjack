package blackjackadapter

import (
	"blackjack/backend/model"
)

type HandEvaluator struct{}

func NewHandEvaluator() *HandEvaluator {
	return &HandEvaluator{}
}

func (h *HandEvaluator) Value(hand []model.StoredCard) int {
	bh := toBlackjackHand(hand)
	return bh.Value()
}

func (h *HandEvaluator) IsBlackjack(hand []model.StoredCard) bool {
	bh := toBlackjackHand(hand)
	return len(bh) == 2 && bh.Value() == 21
}

func (h *HandEvaluator) IsBust(hand []model.StoredCard) bool {
	bh := toBlackjackHand(hand)
	return bh.Value() > 21
}

func (h *HandEvaluator) IsSoft(hand []model.StoredCard) bool {
	bh := toBlackjackHand(hand)
	hard := hardValue(hand)
	return bh.HasAce() && bh.Value() > hard
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

var _ model.HandEvaluator = (*HandEvaluator)(nil)
