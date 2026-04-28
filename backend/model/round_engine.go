package model



type RoundEngine interface {

	ApplyPlayerHit(hand []StoredCard, draw StoredCard) ([]StoredCard, error)

	ResolveOutcome(ev HandEvaluator, playerHand, dealerHand []StoredCard) (Outcome, error)
}
