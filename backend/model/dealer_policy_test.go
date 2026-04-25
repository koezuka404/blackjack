package model

import "testing"

type dealerEvalStub struct {
	value int
	bust  bool
	soft  bool
}

func (d dealerEvalStub) Value([]StoredCard) int         { return d.value }
func (d dealerEvalStub) IsBlackjack([]StoredCard) bool  { return false }
func (d dealerEvalStub) IsBust([]StoredCard) bool       { return d.bust }
func (d dealerEvalStub) IsSoft([]StoredCard) bool       { return d.soft }

func TestNextDealerAction(t *testing.T) {
	tests := []struct {
		name     string
		ev       dealerEvalStub
		want     DealerAction
		terminal bool
	}{
		{name: "bust is terminal stand", ev: dealerEvalStub{bust: true, value: 25}, want: DealerActionStand, terminal: true},
		{name: "soft17 stands", ev: dealerEvalStub{value: 17, soft: true}, want: DealerActionStand, terminal: false},
		{name: "hard17 stands", ev: dealerEvalStub{value: 17, soft: false}, want: DealerActionStand, terminal: false},
		{name: "below17 hits", ev: dealerEvalStub{value: 16}, want: DealerActionHit, terminal: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, term := NextDealerAction(tt.ev, nil)
			if got != tt.want || term != tt.terminal {
				t.Fatalf("got=(%s,%v) want=(%s,%v)", got, term, tt.want, tt.terminal)
			}
		})
	}
}

