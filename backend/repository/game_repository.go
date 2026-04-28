package repository

import (
	"context"
	"errors"
	"time"

	"blackjack/backend/model"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)


type pgStore struct {
	db *gorm.DB
}


func NewPostgreSQLStore(g *gorm.DB) Store {
	return &pgStore{db: g}
}


func (s *pgStore) Transaction(ctx context.Context, fn func(txStore Store) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&pgStore{db: tx})
	})
}


func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAlreadyExists
	}
	return err
}

var _ Store = (*pgStore)(nil)

func (s *pgStore) UpdateSessionIfVersion(ctx context.Context, session *model.GameSession, expectedVersion int64) (bool, error) {
	row, err := gameSessionRecordFromDomain(session)
	if err != nil {
		return false, err
	}
	tx := s.db.WithContext(ctx).
		Model(&GameSessionRecord{}).
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

func (s *pgStore) CreateSession(ctx context.Context, session *model.GameSession) error {
	row, err := gameSessionRecordFromDomain(session)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *pgStore) UpdateSession(ctx context.Context, session *model.GameSession) error {
	row, err := gameSessionRecordFromDomain(session)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *pgStore) GetSession(ctx context.Context, id string) (*model.GameSession, error) {
	var rec GameSessionRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", id).Error; err != nil {
		return nil, mapErr(err)
	}
	return gameSessionRecordToDomain(&rec)
}

func (s *pgStore) GetSessionForUpdate(ctx context.Context, id string) (*model.GameSession, error) {
	var rec GameSessionRecord
	if err := s.db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&rec, "id = ?", id).Error; err != nil {
		return nil, mapErr(err)
	}
	return gameSessionRecordToDomain(&rec)
}

func (s *pgStore) GetLatestSessionByRoomID(ctx context.Context, roomID string) (*model.GameSession, error) {
	var rec GameSessionRecord
	err := s.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("created_at DESC").
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return gameSessionRecordToDomain(&rec)
}

func (s *pgStore) ListSessionsByStatusAndDeadlineBefore(ctx context.Context, status model.SessionStatus, deadline time.Time) ([]*model.GameSession, error) {
	var rows []GameSessionRecord
	err := s.db.WithContext(ctx).
		Where("status = ? AND turn_deadline_at IS NOT NULL AND turn_deadline_at <= ?", string(status), deadline).
		Order("turn_deadline_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]*model.GameSession, 0, len(rows))
	for i := range rows {
		item, err := gameSessionRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *pgStore) ListResettingSessionsDueBy(ctx context.Context, deadline time.Time) ([]*model.GameSession, error) {
	var rows []GameSessionRecord
	err := s.db.WithContext(ctx).
		Where("status = ? AND rematch_deadline_at IS NOT NULL AND rematch_deadline_at <= ?", string(model.SessionStatusResetting), deadline).
		Order("rematch_deadline_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]*model.GameSession, 0, len(rows))
	for i := range rows {
		item, err := gameSessionRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *pgStore) ListSessionsByStatus(ctx context.Context, status model.SessionStatus) ([]*model.GameSession, error) {
	var rows []GameSessionRecord
	err := s.db.WithContext(ctx).
		Where("status = ?", string(status)).
		Order("updated_at ASC").
		Find(&rows).Error
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]*model.GameSession, 0, len(rows))
	for i := range rows {
		item, err := gameSessionRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *pgStore) CountSessions(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&GameSessionRecord{}).Count(&n).Error
	return n, err
}

func (s *pgStore) DeleteGameSessionsByRoomID(ctx context.Context, roomID string) error {
	return s.db.WithContext(ctx).Where("room_id = ?", roomID).Delete(&GameSessionRecord{}).Error
}

func (s *pgStore) CreatePlayerState(ctx context.Context, p *model.PlayerState) error {
	row, err := playerStateRecordFromDomain(p)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *pgStore) UpdatePlayerState(ctx context.Context, p *model.PlayerState) error {
	row, err := playerStateRecordFromDomain(p)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *pgStore) GetPlayerState(ctx context.Context, sessionID, userID string) (*model.PlayerState, error) {
	var rec PlayerStateRecord
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ?", sessionID, userID).
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return playerStateRecordToDomain(&rec)
}

func (s *pgStore) ListPlayerStatesBySessionID(ctx context.Context, sessionID string) ([]*model.PlayerState, error) {
	var rows []PlayerStateRecord
	if err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).Order("seat_no ASC").Find(&rows).Error; err != nil {
		return nil, mapErr(err)
	}
	out := make([]*model.PlayerState, 0, len(rows))
	for i := range rows {
		item, err := playerStateRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *pgStore) CreateDealerState(ctx context.Context, d *model.DealerState) error {
	row, err := dealerStateRecordFromDomain(d)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *pgStore) UpdateDealerState(ctx context.Context, d *model.DealerState) error {
	row, err := dealerStateRecordFromDomain(d)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *pgStore) GetDealerState(ctx context.Context, sessionID string) (*model.DealerState, error) {
	var rec DealerStateRecord
	if err := s.db.WithContext(ctx).First(&rec, "session_id = ?", sessionID).Error; err != nil {
		return nil, mapErr(err)
	}
	return dealerStateRecordToDomain(&rec)
}

func (s *pgStore) CreateActionLog(ctx context.Context, actionLog *model.ActionLog) error {
	row := actionLogRecordFromDomain(actionLog)
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *pgStore) GetActionLogByActionID(ctx context.Context, sessionID, actorUserID, actionID string) (*model.ActionLog, error) {
	var rec ActionLogRecord
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND actor_user_id = ? AND action_id = ?", sessionID, actorUserID, actionID).
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return actionLogRecordToDomain(&rec)
}

func (s *pgStore) UpsertRematchVote(ctx context.Context, vote *model.RematchVote) error {
	row := rematchVoteRecordFromDomain(vote)
	return s.db.WithContext(ctx).
		Where("session_id = ? AND user_id = ?", row.SessionID, row.UserID).
		Assign(map[string]any{
			"agree":      row.Agree,
			"updated_at": time.Now().UTC(),
		}).
		FirstOrCreate(row).Error
}

func (s *pgStore) ListRematchVotes(ctx context.Context, sessionID string) ([]*model.RematchVote, error) {
	var rows []RematchVoteRecord
	if err := s.db.WithContext(ctx).Where("session_id = ?", sessionID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*model.RematchVote, 0, len(rows))
	for i := range rows {
		out = append(out, rematchVoteRecordToDomain(&rows[i]))
	}
	return out, nil
}

func (s *pgStore) CreateRoundLog(ctx context.Context, logItem *model.RoundLog) error {
	row, err := roundLogRecordFromDomain(logItem)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *pgStore) GetRoundLog(ctx context.Context, sessionID string, roundNo int) (*model.RoundLog, error) {
	var rec RoundLogRecord
	err := s.db.WithContext(ctx).
		Where("session_id = ? AND round_no = ?", sessionID, roundNo).
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return roundLogRecordToDomain(&rec), nil
}

func (s *pgStore) ListRoundLogsByRoomID(ctx context.Context, roomID string) ([]*model.RoundLog, error) {
	var rows []RoundLogRecord
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
		out = append(out, roundLogRecordToDomain(&rows[i]))
	}
	return out, nil
}
