package dto

import (
	"time"

	"blackjack/backend/model"
)

func RoomFromDomain(r *model.Room) RoomJSON {
	if r == nil {
		return RoomJSON{}
	}
	return RoomJSON{
		ID:     r.ID,
		Status: string(r.Status),
	}
}

func SessionFromDomain(s *model.GameSession, formatTime func(time.Time) string) SessionJSON {
	if s == nil {
		return SessionJSON{}
	}
	out := SessionJSON{
		ID:       s.ID,
		Status:   string(s.Status),
		Version:  s.Version,
		RoundNo:  s.RoundNo,
		TurnSeat: s.TurnSeat,
	}
	if s.TurnDeadlineAt != nil && formatTime != nil {
		t := formatTime(*s.TurnDeadlineAt)
		out.TurnDeadlineAt = &t
	}
	if s.RematchDeadlineAt != nil && formatTime != nil {
		t := formatTime(*s.RematchDeadlineAt)
		out.RematchDeadlineAt = &t
	}
	return out
}
