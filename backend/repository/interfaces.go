package repository

import (
	"context"
	"time"

	"blackjack/backend/model"
)

type RoomRepository interface {
	CreateRoom(ctx context.Context, room *model.Room) error
	UpdateRoom(ctx context.Context, room *model.Room) error
	GetRoom(ctx context.Context, id string) (*model.Room, error)
	ListRoomsByUserID(ctx context.Context, userID string) ([]*model.Room, error)
	// DeleteRoomPlayersByRoomID removes all rows in room_players for the room (debug reset / admin).
	DeleteRoomPlayersByRoomID(ctx context.Context, roomID string) error
}

type GameSessionRepository interface {
	CreateSession(ctx context.Context, session *model.GameSession) error
	UpdateSession(ctx context.Context, session *model.GameSession) error
	UpdateSessionIfVersion(ctx context.Context, session *model.GameSession, expectedVersion int64) (bool, error)
	GetSession(ctx context.Context, id string) (*model.GameSession, error)
	// GetSessionForUpdate fetches a session row with FOR UPDATE lock in an active transaction.
	GetSessionForUpdate(ctx context.Context, id string) (*model.GameSession, error)
	GetLatestSessionByRoomID(ctx context.Context, roomID string) (*model.GameSession, error)
	ListSessionsByStatusAndDeadlineBefore(ctx context.Context, status model.SessionStatus, deadline time.Time) ([]*model.GameSession, error)
	// ListResettingSessionsDueBy returns RESETTING sessions whose rematch_deadline_at is set and <= deadline (Phase 2 / §9.3.11).
	ListResettingSessionsDueBy(ctx context.Context, deadline time.Time) ([]*model.GameSession, error)
	ListSessionsByStatus(ctx context.Context, status model.SessionStatus) ([]*model.GameSession, error)
	// DeleteGameSessionsByRoomID removes all game_sessions (and cascaded children) for the room.
	DeleteGameSessionsByRoomID(ctx context.Context, roomID string) error
}

type RoomPlayerRepository interface {
	CreateRoomPlayer(ctx context.Context, p *model.RoomPlayer) error
	UpdateRoomPlayer(ctx context.Context, p *model.RoomPlayer) error
	GetRoomPlayer(ctx context.Context, roomID, userID string) (*model.RoomPlayer, error)
	ListRoomPlayersByRoomID(ctx context.Context, roomID string) ([]*model.RoomPlayer, error)
}

type PlayerStateRepository interface {
	CreatePlayerState(ctx context.Context, p *model.PlayerState) error
	UpdatePlayerState(ctx context.Context, p *model.PlayerState) error
	GetPlayerState(ctx context.Context, sessionID, userID string) (*model.PlayerState, error)
	ListPlayerStatesBySessionID(ctx context.Context, sessionID string) ([]*model.PlayerState, error)
}

type DealerStateRepository interface {
	CreateDealerState(ctx context.Context, d *model.DealerState) error
	UpdateDealerState(ctx context.Context, d *model.DealerState) error
	GetDealerState(ctx context.Context, sessionID string) (*model.DealerState, error)
}

type AuthRepository interface {
	CreateUser(ctx context.Context, user *model.User) error
	GetUserByUsername(ctx context.Context, username string) (*model.User, error)
	GetUserByID(ctx context.Context, userID string) (*model.User, error)
	UpsertSession(ctx context.Context, session *model.Session) error
	GetAuthSession(ctx context.Context, sessionID string) (*model.Session, error)
	DeleteSession(ctx context.Context, sessionID string) error
	DeleteSessionsByUserID(ctx context.Context, userID string) error
	DeleteExpiredSessions(ctx context.Context) error
}

type ActionLogRepository interface {
	CreateActionLog(ctx context.Context, actionLog *model.ActionLog) error
	GetActionLogByActionID(ctx context.Context, sessionID, actorUserID, actionID string) (*model.ActionLog, error)
}

type RematchVoteRepository interface {
	UpsertRematchVote(ctx context.Context, vote *model.RematchVote) error
	ListRematchVotes(ctx context.Context, sessionID string) ([]*model.RematchVote, error)
}

type RoundLogRepository interface {
	CreateRoundLog(ctx context.Context, log *model.RoundLog) error
	GetRoundLog(ctx context.Context, sessionID string, roundNo int) (*model.RoundLog, error)
	ListRoundLogsByRoomID(ctx context.Context, roomID string) ([]*model.RoundLog, error)
}

type Store interface {
	RoomRepository
	GameSessionRepository
	RoomPlayerRepository
	PlayerStateRepository
	DealerStateRepository
	AuthRepository
	ActionLogRepository
	RematchVoteRepository
	RoundLogRepository
	Transaction(ctx context.Context, fn func(txStore Store) error) error
}
