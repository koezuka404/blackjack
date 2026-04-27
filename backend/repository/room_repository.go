package repository

import (
	"context"

	"blackjack/backend/model"
)

func (s *pgStore) CreateRoom(ctx context.Context, room *model.Room) error {
	row := roomRecordFromDomain(room)
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *pgStore) UpdateRoom(ctx context.Context, room *model.Room) error {
	row := roomRecordFromDomain(room)
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *pgStore) DeleteRoomPlayersByRoomID(ctx context.Context, roomID string) error {
	return s.db.WithContext(ctx).Where("room_id = ?", roomID).Delete(&RoomPlayerRecord{}).Error
}

func (s *pgStore) CountRooms(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&RoomRecord{}).Count(&n).Error
	return n, err
}

func (s *pgStore) GetRoom(ctx context.Context, id string) (*model.Room, error) {
	var rec RoomRecord
	if err := s.db.WithContext(ctx).First(&rec, "id = ?", id).Error; err != nil {
		return nil, mapErr(err)
	}
	return roomRecordToDomain(&rec)
}

func (s *pgStore) ListRoomsByUserID(ctx context.Context, userID string) ([]*model.Room, error) {
	var rows []RoomRecord
	if err := s.db.WithContext(ctx).
		Where("host_user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, mapErr(err)
	}
	out := make([]*model.Room, 0, len(rows))
	for i := range rows {
		item, err := roomRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *pgStore) CreateRoomPlayer(ctx context.Context, p *model.RoomPlayer) error {
	row := roomPlayerRecordFromDomain(p)
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *pgStore) UpdateRoomPlayer(ctx context.Context, p *model.RoomPlayer) error {
	row := roomPlayerRecordFromDomain(p)
	return s.db.WithContext(ctx).Save(row).Error
}

func (s *pgStore) GetRoomPlayer(ctx context.Context, roomID, userID string) (*model.RoomPlayer, error) {
	var rec RoomPlayerRecord
	err := s.db.WithContext(ctx).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		First(&rec).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return roomPlayerRecordToDomain(&rec)
}

func (s *pgStore) ListRoomPlayersByRoomID(ctx context.Context, roomID string) ([]*model.RoomPlayer, error) {
	var rows []RoomPlayerRecord
	if err := s.db.WithContext(ctx).Where("room_id = ?", roomID).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*model.RoomPlayer, 0, len(rows))
	for i := range rows {
		d, err := roomPlayerRecordToDomain(&rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}
