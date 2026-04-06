package model

import "fmt"

type RoomStatus string

const (
	RoomStatusWaiting RoomStatus = "WAITING"
	RoomStatusReady   RoomStatus = "READY"
	RoomStatusPlaying RoomStatus = "PLAYING"
)

func (s RoomStatus) IsValid() bool {
	switch s {
	case RoomStatusWaiting, RoomStatusReady, RoomStatusPlaying:
		return true
	default:
		return false
	}
}

type SessionStatus string

const (
	SessionStatusDealing    SessionStatus = "DEALING"
	SessionStatusPlayerTurn SessionStatus = "PLAYER_TURN"
	SessionStatusDealerTurn SessionStatus = "DEALER_TURN"
	SessionStatusResult     SessionStatus = "RESULT"
	SessionStatusResetting  SessionStatus = "RESETTING"
)

func (s SessionStatus) IsValid() bool {
	switch s {
	case SessionStatusDealing, SessionStatusPlayerTurn, SessionStatusDealerTurn, SessionStatusResult, SessionStatusResetting:
		return true
	default:
		return false
	}
}

type PlayerStatus string

const (
	PlayerStatusActive       PlayerStatus = "ACTIVE"
	PlayerStatusStand        PlayerStatus = "STAND"
	PlayerStatusBust         PlayerStatus = "BUST"
	PlayerStatusBlackjack    PlayerStatus = "BLACKJACK"
	PlayerStatusDisconnected PlayerStatus = "DISCONNECTED"
	PlayerStatusLeft         PlayerStatus = "LEFT"
)

func (s PlayerStatus) IsValid() bool {
	switch s {
	case PlayerStatusActive, PlayerStatusStand, PlayerStatusBust, PlayerStatusBlackjack, PlayerStatusDisconnected, PlayerStatusLeft:
		return true
	default:
		return false
	}
}

type Outcome string

const (
	OutcomeWin  Outcome = "WIN"
	OutcomeLose Outcome = "LOSE"
	OutcomePush Outcome = "PUSH"
	OutcomeBust Outcome = "BUST"
)

func (o Outcome) IsValid() bool {
	switch o {
	case OutcomeWin, OutcomeLose, OutcomePush, OutcomeBust:
		return true
	default:
		return false
	}
}

type ActorType string

const (
	ActorTypeUser   ActorType = "USER"
	ActorTypeSystem ActorType = "SYSTEM"
)

func (a ActorType) IsValid() bool {
	return a == ActorTypeUser || a == ActorTypeSystem
}

var (
	ErrInvalidStatus       = fmt.Errorf("invalid status")
	ErrInvalidTransition   = fmt.Errorf("invalid transition")
	ErrInvalidVersion      = fmt.Errorf("invalid version")
	ErrVersionConflict     = fmt.Errorf("version_conflict")
	ErrRoomFull            = fmt.Errorf("room_full")
	ErrInvalidSeat         = fmt.Errorf("invalid seat")
	ErrInvalidPlayerStatus = fmt.Errorf("invalid player status")
	ErrInvalidDeck         = fmt.Errorf("invalid deck")
	ErrDeckExhausted       = fmt.Errorf("deck exhausted")
	ErrNotPlayerTurn       = fmt.Errorf("not player turn")
	ErrNotYourTurn         = fmt.Errorf("not your turn")
	ErrCannotJoin          = fmt.Errorf("cannot join room in current state")
	ErrForbiddenStart      = fmt.Errorf("forbidden start")
)
