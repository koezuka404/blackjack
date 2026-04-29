package repository

import (
	"context"

	"blackjack/backend/model"
)

func (s *pgStore) CreateUser(ctx context.Context, user *model.User) error {
	row := userRecordFromDomain(user)
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return mapErr(err)
	}
	return nil
}

func (s *pgStore) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	var rec UserRecord
	if err := s.db.WithContext(ctx).Where("username = ?", username).First(&rec).Error; err != nil {
		return nil, mapErr(err)
	}
	return userRecordToDomain(&rec)
}

func (s *pgStore) GetUserByEmail(ctx context.Context, email string) (*model.User, error) {
	var rec UserRecord
	if err := s.db.WithContext(ctx).Where("email = ?", email).First(&rec).Error; err != nil {
		return nil, mapErr(err)
	}
	return userRecordToDomain(&rec)
}

func (s *pgStore) GetUserByID(ctx context.Context, userID string) (*model.User, error) {
	var rec UserRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", userID).Error; err != nil {
		return nil, mapErr(err)
	}
	return userRecordToDomain(&rec)
}
