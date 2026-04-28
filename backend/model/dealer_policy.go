package model

type DealerAction string

const (
	DealerActionHit   DealerAction = "HIT"
	DealerActionStand DealerAction = "STAND"
)

type HandEvaluator interface {
	Value(hand []StoredCard) int
	IsBlackjack(hand []StoredCard) bool
	IsBust(hand []StoredCard) bool
	IsSoft(hand []StoredCard) bool
}

func NextDealerAction(ev HandEvaluator, dealer []StoredCard) (action DealerAction, terminal bool) {

	if ev.IsBust(dealer) {
		return DealerActionStand, true
	}

	v := ev.Value(dealer)

	if v == 17 && ev.IsSoft(dealer) {
		return DealerActionStand, false
	}
	if v >= 17 {
		return DealerActionStand, false
	}
	return DealerActionHit, false
}
