package blackjackadapter

import (
	"blackjack/backend/model"

	"github.com/ethanefung/cards"
)

// libRoundEngine は github.com/ethanefung/blackjack の Hand（[]cards.Card）へ変換して Draw し、正本の StoredCard 列へ戻す。
type libRoundEngine struct{}

// NewRoundEngine は RoundEngine のデフォルト実装を返す。
func NewRoundEngine() model.RoundEngine {
	return &libRoundEngine{}
}

func (*libRoundEngine) ApplyPlayerHit(hand []model.StoredCard, draw model.StoredCard) ([]model.StoredCard, error) {
	h := toBlackjackHand(hand)
	c := cards.Card{Suit: toSuit(draw.Suit), Rank: toRank(draw.Rank)}
	h.Draw(c)
	return fromBlackjackHand(h), nil
}

func (*libRoundEngine) ResolveOutcome(ev model.HandEvaluator, playerHand, dealerHand []model.StoredCard) (model.Outcome, error) {
	return model.ResolveRoundOutcome(ev, playerHand, dealerHand)
}

var _ model.RoundEngine = (*libRoundEngine)(nil)
