package model

func (s *GameSession) SetDeck(cards []StoredCard) {
	if cards == nil {
		s.Deck = nil
	} else {
		s.Deck = append([]StoredCard(nil), cards...)
	}
	s.DrawIndex = 0
}

func (s *GameSession) DrawCard() (StoredCard, error) {
	if s.DrawIndex < 0 || s.DrawIndex > len(s.Deck) {
		return StoredCard{}, ErrInvalidDeck
	}
	if s.DrawIndex >= len(s.Deck) {
		return StoredCard{}, ErrDeckExhausted
	}
	c := s.Deck[s.DrawIndex]
	s.DrawIndex++
	return c, nil
}

func (s *GameSession) RemainingDeckCards() int {
	if s.DrawIndex < 0 || s.DrawIndex > len(s.Deck) {
		return 0
	}
	return len(s.Deck) - s.DrawIndex
}
