package model

import "testing"

type evaluatorStub struct {
	value int
	bust  bool
	soft  bool
}

func (e evaluatorStub) Value(_ []StoredCard) int       { return e.value }
func (e evaluatorStub) IsBlackjack(_ []StoredCard) bool { return false }
func (e evaluatorStub) IsBust(_ []StoredCard) bool      { return e.bust }
func (e evaluatorStub) IsSoft(_ []StoredCard) bool      { return e.soft }

func TestNextDealerAction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		ev           evaluatorStub
		wantAction   DealerAction
		wantTerminal bool
	}{
		{
			name:         "hard 16 hits",
			ev:           evaluatorStub{value: 16, bust: false, soft: false},
			wantAction:   DealerActionHit,
			wantTerminal: false,
		},
		{
			name:         "hard 17 stands",
			ev:           evaluatorStub{value: 17, bust: false, soft: false},
			wantAction:   DealerActionStand,
			wantTerminal: false,
		},
		{
			name:         "soft 17 stands",
			ev:           evaluatorStub{value: 17, bust: false, soft: true},
			wantAction:   DealerActionStand,
			wantTerminal: false,
		},
		{
			name:         "bust becomes terminal",
			ev:           evaluatorStub{value: 22, bust: true, soft: false},
			wantAction:   DealerActionStand,
			wantTerminal: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotAction, gotTerminal := NextDealerAction(tc.ev, nil)
			if gotAction != tc.wantAction {
				t.Fatalf("action mismatch: got=%s want=%s", gotAction, tc.wantAction)
			}
			if gotTerminal != tc.wantTerminal {
				t.Fatalf("terminal mismatch: got=%v want=%v", gotTerminal, tc.wantTerminal)
			}
		})
	}
}
