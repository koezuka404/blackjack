package model

import (
	"fmt"
	"time"
)

type RoomPlayerStatus string

const (
	RoomPlayerActive       RoomPlayerStatus = "ACTIVE"
	RoomPlayerDisconnected RoomPlayerStatus = "DISCONNECTED"
	RoomPlayerLeft         RoomPlayerStatus = "LEFT"
)

func (s RoomPlayerStatus) IsValid() bool {
	switch s {
	case RoomPlayerActive, RoomPlayerDisconnected, RoomPlayerLeft:
		return true
	default:
		return false
	}
}

type RoomPlayer struct {
	RoomID   string
	UserID   string
	SeatNo   int
	Status   RoomPlayerStatus
	JoinedAt time.Time
	LeftAt   *time.Time
}

func NewRoomPlayer(roomID, userID string, seatNo int, joinedAt time.Time) (*RoomPlayer, error) {
	if roomID == "" || userID == "" {
		return nil, fmt.Errorf("room id and user id are required")
	}
	if seatNo != 1 {
		return nil, ErrInvalidSeat
	}
	if joinedAt.IsZero() {
		return nil, fmt.Errorf("joinedAt is required")
	}
	return &RoomPlayer{
		RoomID:   roomID,
		UserID:   userID,
		SeatNo:   seatNo,
		Status:   RoomPlayerActive,
		JoinedAt: joinedAt,
	}, nil
}

func (p *RoomPlayer) SetStatus(next RoomPlayerStatus) error {
	if !next.IsValid() {
		return ErrInvalidStatus
	}
	p.Status = next
	return nil
}

func (p *RoomPlayer) MarkLeft(at time.Time) {
	p.Status = RoomPlayerLeft
	p.LeftAt = &at
}
