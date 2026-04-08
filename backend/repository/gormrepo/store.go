package gormrepo

import (
	"context"
	"errors"
	"time"

	"blackjack/backend/db"
	"blackjack/backend/model"
	"blackjack/backend/repository"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

type Store struct {
	db *gorm.DB
}

func New(g *gorm.DB) *Store {
	return &Store{db: g}
}

func (s *Store) Transaction(ctx context.Context, fn func(txStore repository.Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Store{db: tx})
	})
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return repository.ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return repository.ErrAlreadyExists
	}
	return err
}

var _ repository.Store = (*Store)(nil)

func (s *Store) CreateUser(ctx context.Context, user *model.User) error {
	row, err := db.UserRecordFromDomain(user)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return mapErr(err)
	}
	return nil
}

func (s *Store) CreateRoom(ctx context.Context, room *model.Room) error {
	row, err := db.RoomRecordFromDomain(room)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *Store) UpdateRoom(ctx context.Context, room *model.Room) error {
	row, err := db.RoomRecordFromDomain(room)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *Store) UpdateSessionIfVersion(ctx context.Context, session *model.GameSession, expectedVersion int64) (bool, error) {
	row, err := db.GameSessionRecordFromDomain(session)
	if err != nil {
		return false, err
	}
	tx := s.db.WithContext(ctx).
		Model(&db.GameSessionRecord{}).
		Where("id = ? AND version = ?", row.ID, expectedVersion).
		Updates(map[string]any{
			"status":              row.Status,
			"version":             row.Version,
			"deck":                row.Deck,
			"draw_index":          row.DrawIndex,
			"turn_seat":           row.TurnSeat,
			"turn_deadline_at":    row.TurnDeadlineAt,
			"result_snapshot":     row.ResultSnapshot,
			"rematch_deadline_at": row.RematchDeadlineAt,
			"updated_at":          row.UpdatedAt,
		})
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected == 1, nil
}

func (s *Store) GetRoom(ctx context.Context, id string) (*model.Room, error) {
	var rec db.RoomRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", id).Error; err != nil {
		return nil, mapErr(err)
	}
	return db.RoomRecordToDomain(&rec)
}

func (s *Store) ListRoomsByUserID(ctx context.Context, userID string) ([]*model.Room, error) {
	var rows []db.RoomRecord
	if err := s.db.WithContext(ctx).
		Where("host_user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, mapErr(err)
	}
	out := make([]*model.Room, 0, len(rows))
	for i := range rows {
		item, err := db.RoomRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) CreateSession(ctx context.Context, session *model.GameSession) error {
	row, err := db.GameSessionRecordFromDomain(session)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *Store) UpdateSession(ctx context.Context, session *model.GameSession) error {
	row, err := db.GameSessionRecordFromDomain(session)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *Store) GetSession(ctx context.Context, id string) (*model.GameSession, error) {
	var rec db.GameSessionRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", id).Error; err != nil {
		return nil, mapErr(err)
	}
	return db.GameSessionRecordToDomain(&rec)
}

func (s *Store) GetLatestSessionByRoomID(ctx context.Context, roomID string) (*model.GameSession, error) {
	var rec db.GameSessionRecord
	err := s.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("created_at DESC").
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return db.GameSessionRecordToDomain(&rec)
}

func (s *Store) CreateRoomPlayer(ctx context.Context, p *model.RoomPlayer) error {
	row, err := db.RoomPlayerRecordFromDomain(p)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *Store) UpdateRoomPlayer(ctx context.Context, p *model.RoomPlayer) error {
	row, err := db.RoomPlayerRecordFromDomain(p)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *Store) GetRoomPlayer(ctx context.Context, roomID, userID string) (*model.RoomPlayer, error) {
	var rec db.RoomPlayerRecord
	err := s.db.WithContext(ctx).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return db.RoomPlayerRecordToDomain(&rec)
}

func (s *Store) ListRoomPlayersByRoomID(ctx context.Context, roomID string) ([]*model.RoomPlayer, error) {
	var rows []db.RoomPlayerRecord
	if err := s.db.WithContext(ctx).Where("room_id = ?", roomID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*model.RoomPlayer, 0, len(rows))
	for i := range rows {
		d, err := db.RoomPlayerRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func (s *Store) CreatePlayerState(ctx context.Context, p *model.PlayerState) error {
	row, err := db.PlayerStateRecordFromDomain(p)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *Store) UpdatePlayerState(ctx context.Context, p *model.PlayerState) error {
	row, err := db.PlayerStateRecordFromDomain(p)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *Store) GetPlayerState(ctx context.Context, sessionID, userID string) (*model.PlayerState, error) {
	var rec db.PlayerStateRecord
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return db.PlayerStateRecordToDomain(&rec)
}

func (s *Store) CreateDealerState(ctx context.Context, d *model.DealerState) error {
	row, err := db.DealerStateRecordFromDomain(d)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *Store) UpdateDealerState(ctx context.Context, d *model.DealerState) error {
	row, err := db.DealerStateRecordFromDomain(d)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *Store) GetDealerState(ctx context.Context, sessionID string) (*model.DealerState, error) {
	var rec db.DealerStateRecord
	if err := s.db.WithContext(ctx).First(&rec, "session_id = ?", sessionID).Error; err != nil {
		return nil, mapErr(err)
	}
	return db.DealerStateRecordToDomain(&rec)
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var rec db.UserRecord
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&rec).Error; err != nil {
		return nil, mapErr(err)
	}
	return db.UserRecordToDomain(&rec)
}

func (s *Store) GetUserByID(ctx context.Context, userID string) (*model.User, error) {
	var rec db.UserRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", userID).Error; err != nil {
		return nil, mapErr(err)
	}
	return db.UserRecordToDomain(&rec)
}

func (s *Store) UpsertSession(ctx context.Context, session *model.Session) error {
	row, err := db.SessionRecordFromDomain(session)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *Store) GetAuthSession(ctx context.Context, sessionID string) (*model.Session, error) {
	var rec db.SessionRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", sessionID).Error; err != nil {
		return nil, mapErr(err)
	}
	return db.SessionRecordToDomain(&rec)
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	return s.db.WithContext(ctx).Delete(&db.SessionRecord{}, "id = ?", sessionID).Error
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	return s.db.WithContext(ctx).
		Delete(&db.SessionRecord{}, "expires_at <= ?", time.Now().UTC()).
		Error
}

func (s *Store) CreateActionLog(ctx context.Context, actionLog *model.ActionLog) error {
	row, err := db.ActionLogRecordFromDomain(actionLog)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *Store) GetActionLogByActionID(ctx context.Context, sessionID, actorUserID, actionID string) (*model.ActionLog, error) {
	var rec db.ActionLogRecord
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND actor_user_id = ? AND action_id = ?", sessionID, actorUserID, actionID).
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return db.ActionLogRecordToDomain(&rec)
}

func (s *Store) UpsertRematchVote(ctx context.Context, vote *model.RematchVote) error {
	row, err := db.RematchVoteRecordFromDomain(vote)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ?", row.SessionID, row.UserID).
		Assign(map[string]any{
			"agree":      row.Agree,
			"updated_at": time.Now().UTC(),
		}).
		FirstOrCreate(row).Error
}

func (s *Store) ListRematchVotes(ctx context.Context, sessionID string) ([]*model.RematchVote, error) {
	var rows []db.RematchVoteRecord
	if err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*model.RematchVote, 0, len(rows))
	for i := range rows {
		v, err := db.RematchVoteRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Store) CreateRoundLog(ctx context.Context, logItem *model.RoundLog) error {
	row, err := db.RoundLogRecordFromDomain(logItem)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *Store) GetRoundLog(ctx context.Context, sessionID string, roundNo int) (*model.RoundLog, error) {
	var rec db.RoundLogRecord
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND round_no = ?", sessionID, roundNo).
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return db.RoundLogRecordToDomain(&rec)
}

func (s *Store) ListRoundLogsByRoomID(ctx context.Context, roomID string) ([]*model.RoundLog, error) {
	var rows []db.RoundLogRecord
	err := s.db.WithContext(ctx).
		Table("round_logs").
		Joins("JOIN game_sessions ON game_sessions.id = round_logs.session_id").
		Where("game_sessions.room_id = ?", roomID).
		Order("round_logs.created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]*model.RoundLog, 0, len(rows))
	for i := range rows {
		item, err := db.RoundLogRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}
