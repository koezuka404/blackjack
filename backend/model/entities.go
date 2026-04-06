package model

import (
	"fmt"
	"time"
)

type StoredCard struct {
	Rank string
	Suit string
}

type Room struct {
	ID               string
	HostUserID       string
	Status           RoomStatus
	CurrentSessionID *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func NewRoom(id, hostUserID string, at time.Time) (*Room, error) {
	if id == "" || hostUserID == "" {
		return nil, fmt.Errorf("room id and host user id are required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return &Room{
		ID:         id,
		HostUserID: hostUserID,
		Status:     RoomStatusWaiting,
		CreatedAt:  at,
		UpdatedAt:  at,
	}, nil
}

func (r *Room) Touch(at time.Time) {
	if !at.IsZero() {
		r.UpdatedAt = at
	}
}

func (r *Room) RecalculateStatus(activeHumanPlayers int, hasActiveSession bool) error {
	if activeHumanPlayers < 0 {
		return fmt.Errorf("activeHumanPlayers must be >= 0")
	}
	if activeHumanPlayers > 1 {
		return ErrRoomFull
	}
	if hasActiveSession {
		r.Status = RoomStatusPlaying
		return nil
	}
	if activeHumanPlayers == 1 {
		r.Status = RoomStatusReady
		return nil
	}
	r.Status = RoomStatusWaiting
	return nil
}

type GameSession struct {
	ID                string
	RoomID            string
	RoundNo           int
	Status            SessionStatus
	Version           int64
	TurnSeat          int
	Deck              []StoredCard
	DrawIndex         int
	TurnDeadlineAt    *time.Time
	ResultSnapshot    *string
	RematchDeadlineAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func NewGameSession(id, roomID string, roundNo int, at time.Time) (*GameSession, error) {
	if id == "" || roomID == "" {
		return nil, fmt.Errorf("session id and room id are required")
	}
	if roundNo <= 0 {
		return nil, fmt.Errorf("roundNo must be > 0")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return &GameSession{
		ID:        id,
		RoomID:    roomID,
		RoundNo:   roundNo,
		Status:    SessionStatusDealing,
		Version:   1,
		TurnSeat:  1,
		DrawIndex: 0,
		CreatedAt: at,
		UpdatedAt: at,
	}, nil
}

func (s *GameSession) Touch(at time.Time) {
	if !at.IsZero() {
		s.UpdatedAt = at
	}
}

func (s *GameSession) SetTurnDeadline(at *time.Time) {
	s.TurnDeadlineAt = at
}

func (s *GameSession) CheckVersion(expected int64) error {
	if expected <= 0 {
		return ErrInvalidVersion
	}
	if expected != s.Version {
		return ErrVersionConflict
	}
	return nil
}

func (s *GameSession) IncrementVersion() {
	s.Version++
}

func (s *GameSession) TransitionTo(next SessionStatus) error {
	if !next.IsValid() {
		return ErrInvalidStatus
	}

	switch s.Status {
	case SessionStatusDealing:
		if next != SessionStatusPlayerTurn {
			return ErrInvalidTransition
		}
	case SessionStatusPlayerTurn:
		if next != SessionStatusDealerTurn {
			return ErrInvalidTransition
		}
	case SessionStatusDealerTurn:
		if next != SessionStatusResult {
			return ErrInvalidTransition
		}
	case SessionStatusResult:
		if next != SessionStatusResetting {
			return ErrInvalidTransition
		}
	case SessionStatusResetting:
		return ErrInvalidTransition
	default:
		return ErrInvalidStatus
	}

	s.Status = next
	return nil
}

type PlayerState struct {
	SessionID  string
	UserID     string
	SeatNo     int
	Hand       []StoredCard
	Status     PlayerStatus
	Outcome    *Outcome
	FinalScore *int
}

func NewPlayerState(sessionID, userID string, seatNo int) (*PlayerState, error) {
	if sessionID == "" || userID == "" {
		return nil, fmt.Errorf("session id and user id are required")
	}
	if seatNo != 1 {
		return nil, ErrInvalidSeat
	}
	return &PlayerState{
		SessionID: sessionID,
		UserID:    userID,
		SeatNo:    seatNo,
		Status:    PlayerStatusActive,
		Hand:      make([]StoredCard, 0, 5),
	}, nil
}

func (p *PlayerState) AppendCard(c StoredCard) {
	p.Hand = append(p.Hand, c)
}

func (p *PlayerState) SetStatus(next PlayerStatus) error {
	if !next.IsValid() {
		return ErrInvalidPlayerStatus
	}
	p.Status = next
	return nil
}

func (p *PlayerState) CanAct() bool {
	return p.Status == PlayerStatusActive
}

func (p *PlayerState) SetOutcome(score int, o Outcome) error {
	if !o.IsValid() {
		return ErrInvalidStatus
	}
	p.Outcome = &o
	p.FinalScore = &score
	return nil
}

type DealerState struct {
	SessionID  string
	Hand       []StoredCard
	HoleHidden bool
	FinalScore *int
}

func NewDealerState(sessionID string) (*DealerState, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	return &DealerState{
		SessionID:  sessionID,
		HoleHidden: true,
		Hand:       make([]StoredCard, 0, 5),
	}, nil
}

func (d *DealerState) AppendCard(c StoredCard) {
	d.Hand = append(d.Hand, c)
}

func (d *DealerState) RevealHole() {
	d.HoleHidden = false
}

func (d *DealerState) SetFinalScore(v int) {
	d.FinalScore = &v
}

type ActionLog struct {
	SessionID          string
	ActorType          ActorType
	ActorUserID        string
	TargetUserID       string
	ActionID           string
	RequestType        string
	RequestPayloadHash string
	ResponseSnapshot   string
}

func (a ActionLog) Validate() error {
	if a.SessionID == "" || a.ActionID == "" || a.RequestType == "" || a.RequestPayloadHash == "" {
		return fmt.Errorf("missing required action log fields")
	}
	if !a.ActorType.IsValid() {
		return fmt.Errorf("invalid actor type")
	}
	if a.ActorType == ActorTypeUser && a.ActorUserID == "" {
		return fmt.Errorf("actor user id is required for USER actor")
	}
	return nil
}

type RematchVote struct {
	SessionID string
	UserID    string
	Agree     bool
}

func (v RematchVote) Validate() error {
	if v.SessionID == "" || v.UserID == "" {
		return fmt.Errorf("session id and user id are required")
	}
	return nil
}
