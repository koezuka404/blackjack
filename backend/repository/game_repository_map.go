package repository

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"blackjack/backend/model"
)

// marshalStoredCardsHook はテスト専用。nil のとき通常の JSON マーシャルを使う。
var marshalStoredCardsHook func([]model.StoredCard) ([]byte, error)

func marshalStoredCards(c []model.StoredCard) ([]byte, error) {
	if marshalStoredCardsHook != nil {
		return marshalStoredCardsHook(c)
	}
	if c == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(c)
}

func unmarshalStoredCards(b []byte) ([]model.StoredCard, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var c []model.StoredCard
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return c, nil
}

func roomRecordFromDomain(r *model.Room) *RoomRecord {
	if r == nil {
		return nil
	}
	return &RoomRecord{
		ID:               r.ID,
		HostUserID:       r.HostUserID,
		Status:           string(r.Status),
		CurrentSessionID: r.CurrentSessionID,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
	}
}

func roomRecordToDomain(m *RoomRecord) (*model.Room, error) {
	if m == nil {
		return nil, nil
	}
	st := model.RoomStatus(m.Status)
	if !st.IsValid() {
		return nil, fmt.Errorf("invalid room status: %s", m.Status)
	}
	return &model.Room{
		ID:               m.ID,
		HostUserID:       m.HostUserID,
		Status:           st,
		CurrentSessionID: m.CurrentSessionID,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}, nil
}

func gameSessionRecordFromDomain(s *model.GameSession) (*GameSessionRecord, error) {
	if s == nil {
		return nil, nil
	}
	deck, err := marshalStoredCards(s.Deck)
	if err != nil {
		return nil, err
	}
	var resultSnap []byte
	if s.ResultSnapshot != nil {
		resultSnap = []byte(*s.ResultSnapshot)
	}
	return &GameSessionRecord{
		ID:                s.ID,
		RoomID:            s.RoomID,
		RoundNo:           s.RoundNo,
		Status:            string(s.Status),
		Version:           s.Version,
		Deck:              deck,
		DrawIndex:         s.DrawIndex,
		TurnSeat:          s.TurnSeat,
		TurnDeadlineAt:    s.TurnDeadlineAt,
		ResultSnapshot:    resultSnap,
		RematchDeadlineAt: s.RematchDeadlineAt,
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
	}, nil
}

func gameSessionRecordToDomain(m *GameSessionRecord) (*model.GameSession, error) {
	if m == nil {
		return nil, nil
	}
	st := model.SessionStatus(m.Status)
	if !st.IsValid() {
		return nil, fmt.Errorf("invalid session status: %s", m.Status)
	}
	deck, err := unmarshalStoredCards(m.Deck)
	if err != nil {
		return nil, err
	}
	var rs *string
	if len(m.ResultSnapshot) > 0 {
		t := string(m.ResultSnapshot)
		rs = &t
	}
	return &model.GameSession{
		ID:                m.ID,
		RoomID:            m.RoomID,
		RoundNo:           m.RoundNo,
		Status:            st,
		Version:           m.Version,
		TurnSeat:          m.TurnSeat,
		Deck:              deck,
		DrawIndex:         m.DrawIndex,
		TurnDeadlineAt:    m.TurnDeadlineAt,
		ResultSnapshot:    rs,
		RematchDeadlineAt: m.RematchDeadlineAt,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}, nil
}

func playerStateRecordFromDomain(p *model.PlayerState) (*PlayerStateRecord, error) {
	if p == nil {
		return nil, nil
	}
	hand, err := marshalStoredCards(p.Hand)
	if err != nil {
		return nil, err
	}
	var oc *string
	if p.Outcome != nil {
		t := string(*p.Outcome)
		oc = &t
	}
	return &PlayerStateRecord{
		SessionID:  p.SessionID,
		UserID:     p.UserID,
		SeatNo:     p.SeatNo,
		Hand:       hand,
		Status:     string(p.Status),
		Outcome:    oc,
		FinalScore: p.FinalScore,
	}, nil
}

func playerStateRecordToDomain(m *PlayerStateRecord) (*model.PlayerState, error) {
	if m == nil {
		return nil, nil
	}
	st := model.PlayerStatus(m.Status)
	if !st.IsValid() {
		return nil, fmt.Errorf("invalid player status: %s", m.Status)
	}
	hand, err := unmarshalStoredCards(m.Hand)
	if err != nil {
		return nil, err
	}
	var oc *model.Outcome
	if m.Outcome != nil {
		o := model.Outcome(*m.Outcome)
		if !o.IsValid() {
			return nil, fmt.Errorf("invalid outcome: %s", *m.Outcome)
		}
		oc = &o
	}
	return &model.PlayerState{
		SessionID:  m.SessionID,
		UserID:     m.UserID,
		SeatNo:     m.SeatNo,
		Hand:       hand,
		Status:     st,
		Outcome:    oc,
		FinalScore: m.FinalScore,
	}, nil
}

func dealerStateRecordFromDomain(d *model.DealerState) (*DealerStateRecord, error) {
	if d == nil {
		return nil, nil
	}
	hand, err := marshalStoredCards(d.Hand)
	if err != nil {
		return nil, err
	}
	return &DealerStateRecord{
		SessionID:  d.SessionID,
		Hand:       hand,
		HoleHidden: d.HoleHidden,
		FinalScore: d.FinalScore,
	}, nil
}

func dealerStateRecordToDomain(m *DealerStateRecord) (*model.DealerState, error) {
	if m == nil {
		return nil, nil
	}
	hand, err := unmarshalStoredCards(m.Hand)
	if err != nil {
		return nil, err
	}
	return &model.DealerState{
		SessionID:  m.SessionID,
		Hand:       hand,
		HoleHidden: m.HoleHidden,
		FinalScore: m.FinalScore,
	}, nil
}

func userRecordFromDomain(u *model.User) *UserRecord {
	if u == nil {
		return nil
	}
	return &UserRecord{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

func userRecordToDomain(m *UserRecord) (*model.User, error) {
	if m == nil {
		return nil, nil
	}
	return &model.User{
		ID:           m.ID,
		Username:     m.Username,
		PasswordHash: m.PasswordHash,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}, nil
}

func authSessionRecordFromDomain(s *model.Session) *SessionRecord {
	if s == nil {
		return nil
	}
	return &SessionRecord{
		ID:        s.ID,
		UserID:    s.UserID,
		ExpiresAt: s.ExpiresAt,
		CreatedAt: s.CreatedAt,
	}
}

func authSessionRecordToDomain(m *SessionRecord) (*model.Session, error) {
	if m == nil {
		return nil, nil
	}
	return &model.Session{
		ID:        m.ID,
		UserID:    m.UserID,
		ExpiresAt: m.ExpiresAt,
		CreatedAt: m.CreatedAt,
	}, nil
}

func roomPlayerRecordFromDomain(p *model.RoomPlayer) *RoomPlayerRecord {
	if p == nil {
		return nil
	}
	return &RoomPlayerRecord{
		RoomID:   p.RoomID,
		UserID:   p.UserID,
		SeatNo:   p.SeatNo,
		Status:   string(p.Status),
		JoinedAt: p.JoinedAt,
		LeftAt:   p.LeftAt,
	}
}

func roomPlayerRecordToDomain(m *RoomPlayerRecord) (*model.RoomPlayer, error) {
	if m == nil {
		return nil, nil
	}
	st := model.RoomPlayerStatus(m.Status)
	if !st.IsValid() {
		return nil, fmt.Errorf("invalid room player status: %s", m.Status)
	}
	return &model.RoomPlayer{
		RoomID:   m.RoomID,
		UserID:   m.UserID,
		SeatNo:   m.SeatNo,
		Status:   st,
		JoinedAt: m.JoinedAt,
		LeftAt:   m.LeftAt,
	}, nil
}

func actionLogRecordFromDomain(a *model.ActionLog) *ActionLogRecord {
	if a == nil {
		return nil
	}
	actorUserID := strings.TrimSpace(a.ActorUserID)
	// Postgres の actor_user_id は uuid 型。SYSTEM 系で空が来た場合は対象ユーザーへ寄せる（旧バイナリ／欠損行の保険）。
	if actorUserID == "" && a.ActorType == model.ActorTypeSystem {
		if tid := strings.TrimSpace(a.TargetUserID); tid != "" {
			actorUserID = tid
		} else {
			actorUserID = "00000000-0000-0000-0000-000000000000"
		}
	}
	return &ActionLogRecord{
		SessionID:          a.SessionID,
		ActorType:          string(a.ActorType),
		ActorUserID:        actorUserID,
		TargetUserID:       a.TargetUserID,
		ActionID:           a.ActionID,
		RequestType:        a.RequestType,
		RequestPayloadHash: a.RequestPayloadHash,
		ResponseSnapshot:   []byte(a.ResponseSnapshot),
	}
}

func actionLogRecordToDomain(m *ActionLogRecord) (*model.ActionLog, error) {
	if m == nil {
		return nil, nil
	}
	actorType := model.ActorType(m.ActorType)
	if !actorType.IsValid() {
		return nil, fmt.Errorf("invalid actor type: %s", m.ActorType)
	}
	return &model.ActionLog{
		SessionID:          m.SessionID,
		ActorType:          actorType,
		ActorUserID:        m.ActorUserID,
		TargetUserID:       m.TargetUserID,
		ActionID:           m.ActionID,
		RequestType:        m.RequestType,
		RequestPayloadHash: m.RequestPayloadHash,
		ResponseSnapshot:   string(m.ResponseSnapshot),
	}, nil
}

func rematchVoteRecordFromDomain(v *model.RematchVote) *RematchVoteRecord {
	if v == nil {
		return nil
	}
	return &RematchVoteRecord{
		SessionID: v.SessionID,
		UserID:    v.UserID,
		Agree:     v.Agree,
	}
}

func rematchVoteRecordToDomain(m *RematchVoteRecord) *model.RematchVote {
	if m == nil {
		return nil
	}
	return &model.RematchVote{
		SessionID: m.SessionID,
		UserID:    m.UserID,
		Agree:     m.Agree,
	}
}

func roundLogRecordFromDomain(r *model.RoundLog) (*RoundLogRecord, error) {
	if r == nil {
		return nil, nil
	}
	var id uint
	if r.ID != "" {
		parsed, err := strconv.ParseUint(r.ID, 10, 64)
		if err != nil {
			return nil, err
		}
		id = uint(parsed)
	}
	return &RoundLogRecord{
		ID:            id,
		SessionID:     r.SessionID,
		RoundNo:       r.RoundNo,
		ResultPayload: []byte(r.ResultPayload),
		CreatedAt:     r.CreatedAt,
	}, nil
}

func roundLogRecordToDomain(m *RoundLogRecord) *model.RoundLog {
	if m == nil {
		return nil
	}
	return &model.RoundLog{
		ID:            strconv.FormatUint(uint64(m.ID), 10),
		SessionID:     m.SessionID,
		RoundNo:       m.RoundNo,
		ResultPayload: string(m.ResultPayload),
		CreatedAt:     m.CreatedAt,
	}
}
