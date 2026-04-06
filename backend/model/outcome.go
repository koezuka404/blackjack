package model

func ResolveRoundOutcome(ev HandEvaluator, playerHand, dealerHand []StoredCard) (Outcome, error) {
	pBust := ev.IsBust(playerHand)
	dBust := ev.IsBust(dealerHand)
	pBJ := ev.IsBlackjack(playerHand)
	dBJ := ev.IsBlackjack(dealerHand)
	pVal := ev.Value(playerHand)
	dVal := ev.Value(dealerHand)

	if pBust {
		return OutcomeLose, nil
	}
	if dBust {
		return OutcomeWin, nil
	}
	if pBJ && !dBJ {
		return OutcomeWin, nil
	}
	if dBJ && !pBJ {
		return OutcomeLose, nil
	}
	if pBJ && dBJ {
		return OutcomePush, nil
	}
	if pVal > dVal {
		return OutcomeWin, nil
	}
	if pVal < dVal {
		return OutcomeLose, nil
	}
	return OutcomePush, nil
}
