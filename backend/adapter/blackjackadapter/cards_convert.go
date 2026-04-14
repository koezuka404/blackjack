package blackjackadapter

import (
	"blackjack/backend/model"

	bj "github.com/ethanefung/blackjack"
	"github.com/ethanefung/cards"
)

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

func fromBlackjackHand(h bj.Hand) []model.StoredCard {
	out := make([]model.StoredCard, 0, len(h))
	for _, c := range h {
		out = append(out, cardToStored(c))
	}
	return out
}

func cardToStored(c cards.Card) model.StoredCard {
	return model.StoredCard{
		Rank: rankToString(c.Rank),
		Suit: suitToString(c.Suit),
	}
}

func rankToString(r cards.Rank) string {
	switch r {
	case cards.Ace:
		return "A"
	case cards.Two:
		return "2"
	case cards.Three:
		return "3"
	case cards.Four:
		return "4"
	case cards.Five:
		return "5"
	case cards.Six:
		return "6"
	case cards.Seven:
		return "7"
	case cards.Eight:
		return "8"
	case cards.Nine:
		return "9"
	case cards.Ten:
		return "10"
	case cards.Jack:
		return "J"
	case cards.Queen:
		return "Q"
	case cards.King:
		return "K"
	default:
		return "2"
	}
}

func suitToString(s cards.Suit) string {
	switch s {
	case cards.Spades:
		return "S"
	case cards.Hearts:
		return "H"
	case cards.Diamonds:
		return "D"
	case cards.Clubs:
		return "C"
	default:
		return "S"
	}
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
