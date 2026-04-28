package repository

import (
	"time"
)



type RoomRecord struct {
	ID               string    `gorm:"type:uuid;primaryKey"`
	HostUserID       string    `gorm:"type:uuid;not null;column:host_user_id"`
	Status           string    `gorm:"type:varchar(32);not null"`
	CurrentSessionID *string   `gorm:"type:uuid;column:current_session_id"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

func (RoomRecord) TableName() string { return "rooms" }

type RoomPlayerRecord struct {
	RoomID   string     `gorm:"type:uuid;not null;column:room_id;primaryKey;uniqueIndex:ux_room_seat"`
	UserID   string     `gorm:"type:uuid;not null;column:user_id;primaryKey"`
	SeatNo   int        `gorm:"not null;column:seat_no;uniqueIndex:ux_room_seat"`
	Status   string     `gorm:"type:varchar(32);not null"`
	JoinedAt time.Time  `gorm:"not null;column:joined_at"`
	LeftAt   *time.Time `gorm:"column:left_at"`
}

func (RoomPlayerRecord) TableName() string { return "room_players" }

type GameSessionRecord struct {
	ID                string     `gorm:"type:uuid;primaryKey"`
	RoomID            string     `gorm:"type:uuid;not null;index;column:room_id;uniqueIndex:ux_room_round"`
	RoundNo           int        `gorm:"not null;column:round_no;uniqueIndex:ux_room_round"`
	Status            string     `gorm:"type:varchar(32);not null"`
	Version           int64      `gorm:"not null;default:1"`
	Deck              []byte     `gorm:"type:jsonb;not null"`
	DrawIndex         int        `gorm:"not null;column:draw_index"`
	TurnSeat          int        `gorm:"not null;column:turn_seat"`
	TurnDeadlineAt    *time.Time `gorm:"column:turn_deadline_at"`
	ResultSnapshot    []byte     `gorm:"type:jsonb;column:result_snapshot"`
	RematchDeadlineAt *time.Time `gorm:"column:rematch_deadline_at"`
	CreatedAt         time.Time  `gorm:"not null;column:created_at"`
	UpdatedAt         time.Time  `gorm:"not null;column:updated_at"`
}

func (GameSessionRecord) TableName() string { return "game_sessions" }

type PlayerStateRecord struct {
	SessionID  string  `gorm:"type:uuid;not null;column:session_id;primaryKey;uniqueIndex:ux_player_session_seat"`
	UserID     string  `gorm:"type:uuid;not null;column:user_id;primaryKey"`
	SeatNo     int     `gorm:"not null;column:seat_no;uniqueIndex:ux_player_session_seat"`
	Hand       []byte  `gorm:"type:jsonb;not null"`
	Status     string  `gorm:"type:varchar(32);not null"`
	Outcome    *string `gorm:"type:varchar(16);column:outcome"`
	FinalScore *int    `gorm:"column:final_score"`
}

func (PlayerStateRecord) TableName() string { return "player_states" }

type DealerStateRecord struct {
	SessionID  string `gorm:"type:uuid;primaryKey;column:session_id"`
	Hand       []byte `gorm:"type:jsonb;not null"`
	HoleHidden bool   `gorm:"not null;column:hole_hidden"`
	FinalScore *int   `gorm:"column:final_score"`
}

func (DealerStateRecord) TableName() string { return "dealer_states" }

type ActionLogRecord struct {
	ID                 uint   `gorm:"primaryKey;autoIncrement"`
	SessionID          string `gorm:"type:uuid;not null;index;column:session_id;uniqueIndex:ux_action_idempotency"`
	ActorType          string `gorm:"type:varchar(16);not null;column:actor_type"`
	ActorUserID        string `gorm:"type:uuid;column:actor_user_id;uniqueIndex:ux_action_idempotency"`
	TargetUserID       string `gorm:"type:uuid;column:target_user_id"`
	ActionID           string `gorm:"type:varchar(128);not null;column:action_id;uniqueIndex:ux_action_idempotency"`
	RequestType        string `gorm:"type:varchar(64);not null;column:request_type"`
	RequestPayloadHash string `gorm:"type:varchar(128);not null;column:request_payload_hash"`
	ResponseSnapshot   []byte `gorm:"type:jsonb;column:response_snapshot"`
}

func (ActionLogRecord) TableName() string { return "action_logs" }

type RematchVoteRecord struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	SessionID string    `gorm:"type:uuid;not null;uniqueIndex:ux_rematch_session_user;column:session_id"`
	UserID    string    `gorm:"type:uuid;not null;uniqueIndex:ux_rematch_session_user;column:user_id"`
	Agree     bool      `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null;column:created_at"`
	UpdatedAt time.Time `gorm:"not null;column:updated_at"`
}

func (RematchVoteRecord) TableName() string { return "rematch_votes" }

type RoundLogRecord struct {
	ID            uint      `gorm:"primaryKey;autoIncrement"`
	SessionID     string    `gorm:"type:uuid;not null;index;column:session_id;uniqueIndex:ux_round_session_no"`
	RoundNo       int       `gorm:"not null;column:round_no;uniqueIndex:ux_round_session_no"`
	ResultPayload []byte    `gorm:"type:jsonb;not null;column:result_payload"`
	CreatedAt     time.Time `gorm:"not null;column:created_at"`
}

func (RoundLogRecord) TableName() string { return "round_logs" }

type UserRecord struct {
	ID           string    `gorm:"type:uuid;primaryKey"`
	Username     string    `gorm:"type:varchar(100);not null;uniqueIndex"`
	PasswordHash string    `gorm:"type:varchar(255);not null;column:password_hash"`
	CreatedAt    time.Time `gorm:"not null;column:created_at"`
	UpdatedAt    time.Time `gorm:"not null;column:updated_at"`
}

func (UserRecord) TableName() string { return "users" }

type SessionRecord struct {
	ID        string    `gorm:"type:varchar(128);primaryKey"`
	UserID    string    `gorm:"type:uuid;not null;index;column:user_id"`
	ExpiresAt time.Time `gorm:"not null;index;column:expires_at"`
	CreatedAt time.Time `gorm:"not null;column:created_at"`
}

func (SessionRecord) TableName() string { return "sessions" }
