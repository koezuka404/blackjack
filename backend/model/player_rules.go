package model

func (p *PlayerState) AssertCanHitOrStand(sess *GameSession, actorUserID string) error {
	if sess == nil {
		return ErrInvalidStatus
	}
	if sess.Status != SessionStatusPlayerTurn {
		return ErrNotPlayerTurn
	}
	if sess.TurnSeat != p.SeatNo {
		return ErrNotYourTurn
	}
	if p.UserID != actorUserID {
		return ErrNotYourTurn
	}
	if !p.CanAct() {
		return ErrInvalidPlayerStatus
	}
	return nil
}
