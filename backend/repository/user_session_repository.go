package repository

import (
	"context"
	"time"

	"blackjack/backend/model"
)

func (s *pgStore) UpsertSession(ctx context.Context, session *model.Session) error {
	row, err := authSessionRecordFromDomain(session)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *pgStore) GetAuthSession(ctx context.Context, sessionID string) (*model.Session, error) {
	var rec SessionRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", sessionID).Error; err != nil {
		return nil, mapErr(err)
	}
	return authSessionRecordToDomain(&rec)
}

func (s *pgStore) DeleteSession(ctx context.Context, sessionID string) error {
	return s.db.WithContext(ctx).Delete(&SessionRecord{}, "id = ?", sessionID).Error
}

func (s *pgStore) DeleteSessionsByUserID(ctx context.Context, userID string) error {
	return s.db.WithContext(ctx).Delete(&SessionRecord{}, "user_id = ?", userID).Error
}

func (s *pgStore) DeleteExpiredSessions(ctx context.Context) error {
	return s.db.WithContext(ctx).
		Delete(&SessionRecord{}, "expires_at <= ?", time.Now().UTC()).
		Error
}
