package model_test

import (
	"testing"

	"blackjack/backend/adapter/blackjackadapter"
	"blackjack/backend/model"
)

func c(rank, suit string) model.StoredCard {
	return model.StoredCard{Rank: rank, Suit: suit}
}

func TestBlackjackAndBustEvaluation(t *testing.T) {
	ev := blackjackadapter.NewHandEvaluator()
	tests := []struct {
		name        string
		hand        []model.StoredCard
		wantBJ      bool
		wantBust    bool
		wantValue   int
	}{
		{
			name:      "two-card blackjack",
			hand:      []model.StoredCard{c("A", "S"), c("10", "H")},
			wantBJ:    true,
			wantBust:  false,
			wantValue: 21,
		},
		{
			name:      "three-card 21 is not blackjack",
			hand:      []model.StoredCard{c("7", "S"), c("7", "H"), c("7", "D")},
			wantBJ:    false,
			wantBust:  false,
			wantValue: 21,
		},
		{
			name:      "bust over 21",
			hand:      []model.StoredCard{c("K", "S"), c("Q", "H"), c("2", "D")},
			wantBJ:    false,
			wantBust:  true,
			wantValue: 22,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ev.IsBlackjack(tt.hand); got != tt.wantBJ {
				t.Fatalf("IsBlackjack() = %v, want %v", got, tt.wantBJ)
			}
			if got := ev.IsBust(tt.hand); got != tt.wantBust {
				t.Fatalf("IsBust() = %v, want %v", got, tt.wantBust)
			}
			if got := ev.Value(tt.hand); got != tt.wantValue {
				t.Fatalf("Value() = %d, want %d", got, tt.wantValue)
			}
		})
	}
}

func TestDealerSoft17Stand(t *testing.T) {
	ev := blackjackadapter.NewHandEvaluator()
	action, terminal := model.NextDealerAction(ev, []model.StoredCard{c("A", "S"), c("6", "H")})
	if action != model.DealerActionStand {
		t.Fatalf("action = %s, want %s", action, model.DealerActionStand)
	}
	if terminal {
		t.Fatalf("terminal = true, want false")
	}
}

func TestResolveRoundOutcomePriority(t *testing.T) {
	ev := blackjackadapter.NewHandEvaluator()
	tests := []struct {
		name   string
		player []model.StoredCard
		dealer []model.StoredCard
		want   model.Outcome
	}{
		{
			name:   "player bust loses",
			player: []model.StoredCard{c("K", "S"), c("Q", "H"), c("2", "D")},
			dealer: []model.StoredCard{c("9", "C"), c("7", "D")},
			want:   model.OutcomeLose,
		},
		{
			name:   "dealer bust player wins",
			player: []model.StoredCard{c("9", "S"), c("7", "H")},
			dealer: []model.StoredCard{c("K", "C"), c("Q", "D"), c("2", "H")},
			want:   model.OutcomeWin,
		},
		{
			name:   "dealer blackjack beats non-blackjack 21",
			player: []model.StoredCard{c("7", "S"), c("7", "H"), c("7", "D")},
			dealer: []model.StoredCard{c("A", "C"), c("K", "D")},
			want:   model.OutcomeLose,
		},
		{
			name:   "both blackjack is push",
			player: []model.StoredCard{c("A", "S"), c("K", "H")},
			dealer: []model.StoredCard{c("A", "C"), c("10", "D")},
			want:   model.OutcomePush,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := model.ResolveRoundOutcome(ev, tt.player, tt.dealer)
			if err != nil {
				t.Fatalf("ResolveRoundOutcome() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveRoundOutcome() = %s, want %s", got, tt.want)
			}
		})
	}
}
