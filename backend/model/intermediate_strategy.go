package model



func RecommendHitOrStand(ev HandEvaluator, player []StoredCard, dealerUp StoredCard) bool {
	if len(player) == 0 || ev.IsBust(player) {
		return false
	}

	dv := dealerUpValue(dealerUp)
	if ev.IsSoft(player) {
		return softHitOrStand(ev, player, dv)
	}
	return hardHitOrStand(ev, player, dv)
}

func dealerUpValue(c StoredCard) int {
	switch c.Rank {
	case "A":
		return 11
	case "K", "Q", "J", "10":
		return 10
	case "9":
		return 9
	case "8":
		return 8
	case "7":
		return 7
	case "6":
		return 6
	case "5":
		return 5
	case "4":
		return 4
	case "3":
		return 3
	case "2":
		return 2
	default:
		return 10
	}
}

func softHitOrStand(ev HandEvaluator, player []StoredCard, dealerValue int) bool {
	v := ev.Value(player)
	if v >= 19 {
		return false
	}
	if v == 18 {

		return dealerValue >= 9 || dealerValue == 11
	}
	return true
}

func hardHitOrStand(ev HandEvaluator, player []StoredCard, dealerValue int) bool {
	v := ev.Value(player)
	switch {
	case v >= 17:
		return false
	case v == 16 || v == 15 || v == 14 || v == 13:
		return !(dealerValue >= 2 && dealerValue <= 6)
	case v == 12:
		return !(dealerValue >= 4 && dealerValue <= 6)
	case v == 11:
		return true
	case v == 10:

		return dealerValue >= 10 || dealerValue == 11
	default:
		return true
	}
}
