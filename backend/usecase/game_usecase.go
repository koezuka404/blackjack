package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"blackjack/backend/model"
	"blackjack/backend/observability"
	"blackjack/backend/repository"

	"github.com/google/uuid"
)

var ErrUnauthorizedUser = errors.New("unauthorized")
var ErrInvalidGameState = errors.New("invalid_game_state")
var ErrForbiddenAction = errors.New("forbidden")

const PlayerTurnTimeout = 15 * time.Second

// HostTransfer は卓ホスト移譲が発生したときの監査用メタデータ（仕様 12.7 / 監査 20.4）。
type HostTransfer struct {
	RoomID             string
	PreviousHostUserID string
	NewHostUserID      string
}

type RoomUsecase interface {
	CreateRoom(ctx context.Context, hostUserID string) (*model.Room, error)
	JoinRoom(ctx context.Context, roomID, userID string) (*model.Room, error)
	GetRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error)
	GetRoomState(ctx context.Context, roomID, userID string) (*RoomState, error)
	ListRooms(ctx context.Context, userID string) ([]*model.Room, error)
	GetRoomHistory(ctx context.Context, roomID, userID string) ([]*model.RoundLog, error)
	LeaveRoom(ctx context.Context, roomID, userID string) (*model.Room, *HostTransfer, error)
	StartRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error)
	Hit(ctx context.Context, roomID, userID string, expectedVersion int64, actionID string) (*model.Room, *model.GameSession, error)
	Stand(ctx context.Context, roomID, userID string, expectedVersion int64, actionID string) (*model.Room, *model.GameSession, error)
	VoteRematch(ctx context.Context, roomID, userID string, agree bool, expectedVersion int64, actionID string) (*model.Room, *model.GameSession, error)
	MarkConnected(ctx context.Context, roomID, userID string) error
	MarkDisconnected(ctx context.Context, roomID, userID string) error
	AutoStandDueSessions(ctx context.Context) ([]string, error)
	SuggestPlayerAction(ctx context.Context, roomID, userID string) (*PlayHint, error)
	// ResetRoomForDebug は開発用に卓を初期状態へ戻す（§15.3）。本番無効化は Controller 側。
	ResetRoomForDebug(ctx context.Context, roomID, userID string) (*model.Room, error)
}

type RoomState struct {
	Room       *model.Room
	Session    *model.GameSession
	Dealer     *model.DealerState
	Players    []*model.PlayerState
	CanHit     bool
	CanStand   bool
	CanRematch bool
}

// PlayHint は中級者向けヒューリスティックによる推奨手と説明。
type PlayHint struct {
	Recommendation string
	SessionVersion int64
	Rationale      string
}

type roomService struct {
	store     repository.Store
	evaluator model.HandEvaluator
	engine    model.RoundEngine
}

// NewRoomUsecase はルーム・ゲーム進行のユースケースを組み立てる。
func NewRoomUsecase(store repository.Store, evaluator model.HandEvaluator, engine model.RoundEngine) RoomUsecase {
	return &roomService{
		store:     store,
		evaluator: evaluator,
		engine:    engine,
	}
}

// CreateRoom はホストのみがルーム行を作成する（卓・参加者はまだ作らない）。
func (u *roomService) CreateRoom(ctx context.Context, hostUserID string) (*model.Room, error) {
	if hostUserID == "" {
		return nil, ErrUnauthorizedUser
	}
	now := time.Now().UTC()
	roomID := uuid.NewString()

	room, err := newRoomForCreate(roomID, hostUserID, now)
	if err != nil {
		return nil, err
	}
	if err := roomRecalculateStatus(room, 0, false); err != nil {
		return nil, err
	}
	room.Touch(now)

	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		return tx.CreateRoom(ctx, room)
	}); err != nil {
		return nil, err
	}
	return room, nil
}

// JoinRoom はホスト本人が卓に参加（人間プレイヤー1名まで）し、ルーム状態を再計算する。
func (u *roomService) JoinRoom(ctx context.Context, roomID, userID string) (*model.Room, error) {
	if userID == "" {
		return nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !model.CanJoinAsHumanPlayer(room.Status) {
		return nil, ErrInvalidGameState
	}
	if room.HostUserID != userID {
		return nil, ErrForbiddenAction
	}
	players, err := u.store.ListRoomPlayersByRoomID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	activeHuman := 0
	for _, p := range players {
		if p.Status == model.RoomPlayerActive || p.Status == model.RoomPlayerDisconnected {
			activeHuman++
		}
	}
	if activeHuman >= 1 {
		return nil, model.ErrRoomFull
	}

	now := time.Now().UTC()
	joiner, err := newRoomPlayerForJoin(roomID, userID, 1, now)
	if err != nil {
		return nil, err
	}
	if err := roomRecalculateStatus(room, activeHuman+1, room.CurrentSessionID != nil); err != nil {
		return nil, err
	}
	room.Touch(now)

	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		existing, err := tx.GetRoomPlayer(ctx, roomID, userID)
		if err != nil && err != repository.ErrNotFound {
			return err
		}
		if err == repository.ErrNotFound {
			if err := tx.CreateRoomPlayer(ctx, joiner); err != nil {
				if err == repository.ErrAlreadyExists {
					return model.ErrRoomFull
				}
				return err
			}
		} else {
			// 一度 LEFT になった同一ユーザーは既存行を再利用して再参加させる。
			if existing.Status != model.RoomPlayerLeft {
				return model.ErrRoomFull
			}
			if err := existing.SetStatus(model.RoomPlayerActive); err != nil {
				return err
			}
			existing.LeftAt = nil
			existing.JoinedAt = now
			if err := tx.UpdateRoomPlayer(ctx, existing); err != nil {
				return err
			}
		}
		return tx.UpdateRoom(ctx, room)
	}); err != nil {
		return nil, err
	}
	return room, nil
}

// GetRoom は参加者がルームと現在セッション（あれば）を取得する。
func (u *roomService) GetRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error) {
	if userID == "" {
		return nil, nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	if room.HostUserID != userID {
		p, err := u.store.GetRoomPlayer(ctx, roomID, userID)
		if err != nil {
			if err == repository.ErrNotFound {
				return nil, nil, ErrForbiddenAction
			}
			return nil, nil, err
		}
		if p.Status == model.RoomPlayerLeft {
			return nil, nil, ErrForbiddenAction
		}
	}
	if room.CurrentSessionID == nil {
		return room, nil, nil
	}
	sess, err := u.store.GetSession(ctx, *room.CurrentSessionID)
	if err != nil {
		if err == repository.ErrNotFound {
			return room, nil, nil
		}
		return nil, nil, err
	}
	return room, sess, nil
}

// ListRooms はユーザーがホストのルーム一覧を返す。
func (u *roomService) ListRooms(ctx context.Context, userID string) ([]*model.Room, error) {
	if userID == "" {
		return nil, ErrUnauthorizedUser
	}
	return u.store.ListRoomsByUserID(ctx, userID)
}

// LeaveRoom は参加者が卓から離脱し、ルーム状態を更新する。
func (u *roomService) LeaveRoom(ctx context.Context, roomID, userID string) (*model.Room, *HostTransfer, error) {
	if userID == "" {
		return nil, nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	if room.CurrentSessionID != nil {
		return nil, nil, ErrInvalidGameState
	}
	p, err := u.store.GetRoomPlayer(ctx, roomID, userID)
	if err != nil {
		return nil, nil, err
	}
	if p.Status == model.RoomPlayerLeft {
		return room, nil, nil
	}
	players, err := u.store.ListRoomPlayersByRoomID(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	var transfer *HostTransfer
	now := time.Now().UTC()
	p.MarkLeft(now)
	activeAfterLeave := 0
	for _, rp := range players {
		if rp.UserID == userID {
			continue
		}
		if rp.Status == model.RoomPlayerActive || rp.Status == model.RoomPlayerDisconnected {
			activeAfterLeave++
		}
	}
	// 仕様 §12.7: host が LEFT の場合は最小 seat の有効参加者へ委譲する。
	if room.HostUserID == userID {
		if nextHost := selectNextHost(players, userID); nextHost != nil {
			transfer = &HostTransfer{
				RoomID:             roomID,
				PreviousHostUserID: userID,
				NewHostUserID:      nextHost.UserID,
			}
			room.HostUserID = nextHost.UserID
		}
	}
	if err := roomRecalculateStatus(room, activeAfterLeave, false); err != nil {
		return nil, nil, err
	}
	room.Touch(now)
	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		if err := tx.UpdateRoomPlayer(ctx, p); err != nil {
			return err
		}
		return tx.UpdateRoom(ctx, room)
	}); err != nil {
		return nil, nil, err
	}
	return room, transfer, nil
}

func selectNextHost(players []*model.RoomPlayer, leavingUserID string) *model.RoomPlayer {
	candidates := make([]*model.RoomPlayer, 0)
	for _, p := range players {
		if p.UserID == leavingUserID {
			continue
		}
		if p.Status == model.RoomPlayerActive || p.Status == model.RoomPlayerDisconnected {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].SeatNo != candidates[j].SeatNo {
			return candidates[i].SeatNo < candidates[j].SeatNo
		}
		return candidates[i].UserID < candidates[j].UserID
	})
	return candidates[0]
}

// ResetRoomForDebug は開発用に game_sessions 系と room_players を削除し、卓を WAITING に戻す（§15.3）。
func (u *roomService) ResetRoomForDebug(ctx context.Context, roomID, userID string) (*model.Room, error) {
	if userID == "" {
		return nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, ErrInvalidInput
	}
	var out *model.Room
	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		room, err := tx.GetRoom(ctx, roomID)
		if err != nil {
			return err
		}
		if room.HostUserID != userID {
			return ErrForbiddenAction
		}
		now := time.Now().UTC()
		room.CurrentSessionID = nil
		if err := tx.UpdateRoom(ctx, room); err != nil {
			return err
		}
		if err := tx.DeleteGameSessionsByRoomID(ctx, roomID); err != nil {
			return err
		}
		if err := tx.DeleteRoomPlayersByRoomID(ctx, roomID); err != nil {
			return err
		}
		if err := roomRecalculateStatus(room, 0, false); err != nil {
			return err
		}
		room.Touch(now)
		if err := tx.UpdateRoom(ctx, room); err != nil {
			return err
		}
		out = room
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// GetRoomHistory は卓のラウンド監査ログ（round_logs）を参加者向けに返す。
func (u *roomService) GetRoomHistory(ctx context.Context, roomID, userID string) ([]*model.RoundLog, error) {
	if userID == "" {
		return nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.HostUserID != userID {
		p, err := u.store.GetRoomPlayer(ctx, roomID, userID)
		if err != nil {
			if err == repository.ErrNotFound {
				return nil, ErrForbiddenAction
			}
			return nil, err
		}
		if p.Status == model.RoomPlayerLeft {
			return nil, ErrForbiddenAction
		}
	}
	return u.store.ListRoundLogsByRoomID(ctx, roomID)
}

// StartRoom はホストがゲームを開始し、山札・配札・最初のセッションを作成する。
func (u *roomService) StartRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error) {
	if userID == "" {
		return nil, nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	if room.HostUserID != userID {
		return nil, nil, ErrForbiddenAction
	}
	if room.Status != model.RoomStatusReady {
		return nil, nil, ErrInvalidGameState
	}
	player, err := u.store.GetRoomPlayer(ctx, roomID, userID)
	if err != nil {
		return nil, nil, err
	}
	if player.Status != model.RoomPlayerActive && player.Status != model.RoomPlayerDisconnected {
		return nil, nil, ErrInvalidGameState
	}
	if room.CurrentSessionID != nil {
		return nil, nil, ErrInvalidGameState
	}
	roundNo := 1
	latest, err := u.store.GetLatestSessionByRoomID(ctx, roomID)
	if err != nil && err != repository.ErrNotFound {
		return nil, nil, err
	}
	if latest != nil {
		roundNo = latest.RoundNo + 1
	}

	now := time.Now().UTC()
	sess, err := newGameSessionUC(uuid.NewString(), roomID, roundNo, now)
	if err != nil {
		return nil, nil, err
	}
	sess.SetDeck(newShuffledDeck())
	dealer, err := newDealerStateUC(sess.ID)
	if err != nil {
		return nil, nil, err
	}
	pstate, err := newPlayerStateUC(sess.ID, userID, 1)
	if err != nil {
		return nil, nil, err
	}

	if err := u.initialDeal(sess, pstate, dealer); err != nil {
		return nil, nil, err
	}
	if err := gameSessionTransition(sess, model.SessionStatusPlayerTurn); err != nil {
		return nil, nil, err
	}
	deadline := now.Add(PlayerTurnTimeout)
	sess.SetTurnDeadline(&deadline)

	if u.evaluator.IsBlackjack(pstate.Hand) {
		if err := playerStateSetStatus(pstate, model.PlayerStatusBlackjack); err != nil {
			return nil, nil, err
		}
		if err := gameSessionTransition(sess, model.SessionStatusDealerTurn); err != nil {
			return nil, nil, err
		}
		sess.SetTurnDeadline(nil)
	}
	room.CurrentSessionID = &sess.ID
	if err := roomRecalculateStatus(room, 1, room.CurrentSessionID != nil); err != nil {
		return nil, nil, err
	}
	room.Touch(now)
	sess.Touch(now)

	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		if err := tx.CreateSession(ctx, sess); err != nil {
			return err
		}
		if err := tx.CreatePlayerState(ctx, pstate); err != nil {
			return err
		}
		if err := tx.CreateDealerState(ctx, dealer); err != nil {
			return err
		}
		return tx.UpdateRoom(ctx, room)
	}); err != nil {
		return nil, nil, err
	}
	return room, sess, nil
}

// Hit はプレイヤーのヒット操作（冪等・version 整合つき）。
func (u *roomService) Hit(ctx context.Context, roomID, userID string, expectedVersion int64, actionID string) (*model.Room, *model.GameSession, error) {
	return u.playerTurn(ctx, roomID, userID, expectedVersion, actionID, true)
}

// Stand はプレイヤーのスタンド操作（冪等・version 整合つき）。
func (u *roomService) Stand(ctx context.Context, roomID, userID string, expectedVersion int64, actionID string) (*model.Room, *model.GameSession, error) {
	return u.playerTurn(ctx, roomID, userID, expectedVersion, actionID, false)
}

// playerTurn は Hit/Stand の共通処理（状態検証・ライブラリ連携・永続化）。
func (u *roomService) playerTurn(ctx context.Context, roomID, userID string, expectedVersion int64, actionID string, hit bool) (*model.Room, *model.GameSession, error) {
	if userID == "" {
		return nil, nil, ErrUnauthorizedUser
	}
	if roomID == "" || expectedVersion <= 0 || actionID == "" {
		return nil, nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	if room.CurrentSessionID == nil {
		return nil, nil, ErrInvalidGameState
	}
	sess, err := u.store.GetSession(ctx, *room.CurrentSessionID)
	if err != nil {
		return nil, nil, err
	}
	if err := sess.CheckVersion(expectedVersion); err != nil {
		return nil, nil, err
	}
	player, err := u.store.GetPlayerState(ctx, sess.ID, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, nil, ErrForbiddenAction
		}
		return nil, nil, err
	}
	dealer, err := u.store.GetDealerState(ctx, sess.ID)
	if err != nil {
		return nil, nil, err
	}
	if err := player.AssertCanHitOrStand(sess, userID); err != nil {
		return nil, nil, err
	}
	requestType := "HIT"
	if !hit {
		requestType = "STAND"
	}
	payload := requestType + ":" + strconv.FormatInt(expectedVersion, 10)
	hash := sha256.Sum256([]byte(payload))
	actionLog := &model.ActionLog{
		SessionID:          sess.ID,
		ActorType:          model.ActorTypeUser,
		ActorUserID:        userID,
		TargetUserID:       userID,
		ActionID:           actionID,
		RequestType:        requestType,
		RequestPayloadHash: hex.EncodeToString(hash[:]),
	}
	if _, replay, err := EnsureActionIdempotency(ctx, u.store, actionLog); err != nil {
		return nil, nil, err
	} else if replay {
		return room, sess, nil
	}

	now := time.Now().UTC()
	if hit {
		card, err := sess.DrawCard()
		if err != nil {
			return nil, nil, err
		}
		nextHand, err := u.engine.ApplyPlayerHit(player.Hand, card)
		if err != nil {
			return nil, nil, err
		}
		player.Hand = nextHand
		v := u.evaluator.Value(player.Hand)
		if v > 21 {
			if err := playerStateSetStatus(player, model.PlayerStatusBust); err != nil {
				return nil, nil, err
			}
			if err := gameSessionTransition(sess, model.SessionStatusDealerTurn); err != nil {
				return nil, nil, err
			}
			sess.SetTurnDeadline(nil)
		} else if u.evaluator.IsBlackjack(player.Hand) {
			if err := playerStateSetStatus(player, model.PlayerStatusBlackjack); err != nil {
				return nil, nil, err
			}
			if err := gameSessionTransition(sess, model.SessionStatusDealerTurn); err != nil {
				return nil, nil, err
			}
			sess.SetTurnDeadline(nil)
		}
	} else {
		nextDeadline := now.Add(PlayerTurnTimeout)
		sess.SetTurnDeadline(&nextDeadline)
		if err := playerStateSetStatus(player, model.PlayerStatusStand); err != nil {
			return nil, nil, err
		}
		if err := gameSessionTransition(sess, model.SessionStatusDealerTurn); err != nil {
			return nil, nil, err
		}
		sess.SetTurnDeadline(nil)
	}

	if sess.Status == model.SessionStatusPlayerTurn {
		nextDeadline := now.Add(PlayerTurnTimeout)
		sess.SetTurnDeadline(&nextDeadline)
	}
	if err := roomRecalculateStatus(room, 1, true); err != nil {
		return nil, nil, err
	}
	room.CurrentSessionID = &sess.ID
	room.Touch(now)
	sess.IncrementVersion()
	sess.Touch(now)
	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		ok, err := tx.UpdateSessionIfVersion(ctx, sess, expectedVersion)
		if err != nil {
			return err
		}
		if !ok {
			return model.ErrVersionConflict
		}
		if err := tx.UpdatePlayerState(ctx, player); err != nil {
			return err
		}
		if err := tx.UpdateDealerState(ctx, dealer); err != nil {
			return err
		}
		if err := tx.UpdateRoom(ctx, room); err != nil {
			return err
		}
		snapshotBytes, err := marshalGameJSON(map[string]any{
			"room_id":    room.ID,
			"session_id": sess.ID,
			"version":    sess.Version,
		})
		if err != nil {
			return err
		}
		if err := SaveActionSuccessSnapshot(ctx, tx, actionLog, string(snapshotBytes)); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return room, sess, nil
}

// GetRoomState は WS/HTTP 同期用にルーム・セッション・手札可否を組み立てる。
func (u *roomService) GetRoomState(ctx context.Context, roomID, userID string) (*RoomState, error) {
	room, sess, err := u.GetRoom(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	state := &RoomState{Room: room, Session: sess}
	if sess == nil {
		return state, nil
	}
	dealer, err := u.store.GetDealerState(ctx, sess.ID)
	if err != nil && err != repository.ErrNotFound {
		return nil, err
	}
	players, err := u.store.ListPlayerStatesBySessionID(ctx, sess.ID)
	if err != nil && err != repository.ErrNotFound {
		return nil, err
	}
	state.Dealer = dealer
	state.Players = players
	for _, p := range players {
		if p.UserID == userID {
			state.CanHit = p.Status == model.PlayerStatusActive && sess.Status == model.SessionStatusPlayerTurn && sess.TurnSeat == p.SeatNo
			state.CanStand = state.CanHit
			break
		}
	}
	state.CanRematch = sess.Status == model.SessionStatusResetting
	return state, nil
}

// SuggestPlayerAction は現在局面での HIT/STAND 推奨を返す（プレイヤーターンでヒット可能なときのみ）。
func (u *roomService) SuggestPlayerAction(ctx context.Context, roomID, userID string) (*PlayHint, error) {
	state, err := u.GetRoomState(ctx, roomID, userID)
	if err != nil {
		return nil, err
	}
	if state.Session == nil || !state.CanHit {
		return nil, ErrInvalidGameState
	}
	if state.Dealer == nil || len(state.Dealer.Hand) == 0 {
		return nil, ErrInvalidGameState
	}
	var hand []model.StoredCard
	for _, p := range state.Players {
		if p.UserID == userID {
			hand = p.Hand
			break
		}
	}
	if len(hand) == 0 {
		return nil, ErrInvalidGameState
	}
	up := state.Dealer.Hand[0]
	// 推奨は純粋関数で計算し、ここでは結果をレスポンス用に整形するだけ。
	wantHit := model.RecommendHitOrStand(u.evaluator, hand, up)
	rec := "STAND"
	rationale := "中級向け簡略ベーシック戦略（S17・ダブル/スプリットなし）に基づきスタンド。"
	if wantHit {
		rec = "HIT"
		rationale = "中級向け簡略ベーシック戦略（S17・ダブル/スプリットなし）に基づきヒット。"
	}
	return &PlayHint{
		Recommendation: rec,
		SessionVersion: state.Session.Version,
		Rationale:      rationale,
	}, nil
}

// rematchEligibleUserIDs は再戦投票の対象となる人間プレイヤー user_id を返す（§12.1）。
func rematchEligibleUserIDs(players []*model.RoomPlayer) []string {
	out := make([]string, 0)
	for _, p := range players {
		if p.Status == model.RoomPlayerActive || p.Status == model.RoomPlayerDisconnected {
			out = append(out, p.UserID)
		}
	}
	return out
}

// rematchAgreeMapAtDeadline は締切時点の賛否マップ（未投票は false）（§12.5）。
func rematchAgreeMapAtDeadline(eligible []string, votes []*model.RematchVote) map[string]bool {
	byUser := make(map[string]bool)
	for _, v := range votes {
		byUser[v.UserID] = v.Agree
	}
	m := make(map[string]bool)
	for _, uid := range eligible {
		if v, ok := byUser[uid]; ok {
			m[uid] = v
		} else {
			m[uid] = false
		}
	}
	return m
}

// hasExplicitRematchDenial は誰かが明示的に否認したか（締切前の即時不成⽴用）。
func hasExplicitRematchDenial(eligible []string, agreeMap map[string]bool) bool {
	for _, uid := range eligible {
		if v, ok := agreeMap[uid]; ok && !v {
			return true
		}
	}
	return false
}

// finalizeRematchFailureTx は再戦不成⽴時に current_session を外しルームだけ更新する（§9.3.11）。
func (u *roomService) finalizeRematchFailureTx(ctx context.Context, tx repository.Store, room *model.Room) error {
	room.CurrentSessionID = nil
	players, err := tx.ListRoomPlayersByRoomID(ctx, room.ID)
	if err != nil {
		return err
	}
	n := 0
	for _, p := range players {
		if p.Status == model.RoomPlayerActive || p.Status == model.RoomPlayerDisconnected {
			n++
		}
	}
	now := time.Now().UTC()
	if err := roomRecalculateStatus(room, n, false); err != nil {
		return err
	}
	room.Touch(now)
	return tx.UpdateRoom(ctx, room)
}

// rematchUnanimousSuccessTx は全会一致で次ラウンドのセッションを作成する（§9.3.10）。
func (u *roomService) rematchUnanimousSuccessTx(ctx context.Context, tx repository.Store, room *model.Room, prev *model.GameSession, playerUserID string, now time.Time, expectedVersion int64) (*model.GameSession, error) {
	prev.IncrementVersion()
	prev.Touch(now)
	ok, err := tx.UpdateSessionIfVersion(ctx, prev, expectedVersion)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, model.ErrVersionConflict
	}
	next, err := newGameSessionUC(uuid.NewString(), room.ID, prev.RoundNo+1, now)
	if err != nil {
		return nil, err
	}
	next.SetDeck(newShuffledDeck())
	dealer, err := newDealerStateUC(next.ID)
	if err != nil {
		return nil, err
	}
	pstate, err := newPlayerStateUC(next.ID, playerUserID, 1)
	if err != nil {
		return nil, err
	}
	if err := u.initialDeal(next, pstate, dealer); err != nil {
		return nil, err
	}
	if err := gameSessionTransition(next, model.SessionStatusPlayerTurn); err != nil {
		return nil, err
	}
	deadline := now.Add(PlayerTurnTimeout)
	next.SetTurnDeadline(&deadline)
	if u.evaluator.IsBlackjack(pstate.Hand) {
		if err := playerStateSetStatus(pstate, model.PlayerStatusBlackjack); err != nil {
			return nil, err
		}
		if err := gameSessionTransition(next, model.SessionStatusDealerTurn); err != nil {
			return nil, err
		}
		next.SetTurnDeadline(nil)
	}
	room.CurrentSessionID = &next.ID
	if err := roomRecalculateStatus(room, 1, true); err != nil {
		return nil, err
	}
	room.Touch(now)
	if err := tx.CreateSession(ctx, next); err != nil {
		return nil, err
	}
	if err := tx.CreatePlayerState(ctx, pstate); err != nil {
		return nil, err
	}
	if err := tx.CreateDealerState(ctx, dealer); err != nil {
		return nil, err
	}
	if err := tx.UpdateRoom(ctx, room); err != nil {
		return nil, err
	}
	return next, nil
}

// VoteRematch は再戦投票を処理し、全会一致・否認・継続のいずれかに分岐する。
func (u *roomService) VoteRematch(ctx context.Context, roomID, userID string, agree bool, expectedVersion int64, actionID string) (*model.Room, *model.GameSession, error) {
	if userID == "" {
		return nil, nil, ErrUnauthorizedUser
	}
	if roomID == "" || expectedVersion <= 0 || actionID == "" {
		return nil, nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := u.store.GetRoomPlayer(ctx, roomID, userID); err != nil {
		if err == repository.ErrNotFound {
			return nil, nil, ErrForbiddenAction
		}
		return nil, nil, err
	}
	sess, err := u.store.GetLatestSessionByRoomID(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}

	payload := "REMATCH_VOTE:" + strconv.FormatBool(agree) + ":" + strconv.FormatInt(expectedVersion, 10)
	hash := sha256.Sum256([]byte(payload))
	actionLog := &model.ActionLog{
		SessionID:          sess.ID,
		ActorType:          model.ActorTypeUser,
		ActorUserID:        userID,
		TargetUserID:       userID,
		ActionID:           actionID,
		RequestType:        "REMATCH_VOTE",
		RequestPayloadHash: hex.EncodeToString(hash[:]),
	}
	lockedSessionID := sess.ID

	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		locked, err := tx.GetSessionForUpdate(ctx, lockedSessionID)
		if err != nil {
			return err
		}
		if locked.Status != model.SessionStatusResetting {
			return ErrInvalidGameState
		}
		if err := locked.CheckVersion(expectedVersion); err != nil {
			return err
		}
		sess = locked
		actionLog.SessionID = sess.ID

		if _, replay, err := EnsureActionIdempotency(ctx, tx, actionLog); err != nil {
			return err
		} else if replay {
			return nil
		}
		roomPlayers, err := tx.ListRoomPlayersByRoomID(ctx, roomID)
		if err != nil {
			return err
		}
		eligible := rematchEligibleUserIDs(roomPlayers)
		eligibleOK := false
		for _, uid := range eligible {
			if uid == userID {
				eligibleOK = true
				break
			}
		}
		if !eligibleOK {
			return ErrForbiddenAction
		}

		vote := &model.RematchVote{
			SessionID: sess.ID,
			UserID:    userID,
			Agree:     agree,
		}
		now := time.Now().UTC()
		if sess.RematchDeadlineAt == nil {
			sess.SetRematchDeadline(now)
		}
		votes, err := tx.ListRematchVotes(ctx, sess.ID)
		if err != nil && err != repository.ErrNotFound {
			return err
		}
		agreeMap := map[string]bool{}
		for _, v := range votes {
			agreeMap[v.UserID] = v.Agree
		}
		agreeMap[userID] = agree
		unanimous := model.RematchUnanimous(eligible, agreeMap)
		denial := hasExplicitRematchDenial(eligible, agreeMap)

		if err := tx.UpsertRematchVote(ctx, vote); err != nil {
			return err
		}

		switch {
		case unanimous:
			playerUID := eligible[0]
			next, err := u.rematchUnanimousSuccessTx(ctx, tx, room, sess, playerUID, now, expectedVersion)
			if err != nil {
				return err
			}
			sess = next
		case denial:
			if err := u.finalizeRematchFailureTx(ctx, tx, room); err != nil {
				return err
			}
		default:
			sess.IncrementVersion()
			sess.Touch(now)
			ok, err := tx.UpdateSessionIfVersion(ctx, sess, expectedVersion)
			if err != nil {
				return err
			}
			if !ok {
				return model.ErrVersionConflict
			}
		}

		snapshotBytes, err := marshalGameJSON(map[string]any{
			"room_id":    room.ID,
			"session_id": sess.ID,
			"version":    sess.Version,
		})
		if err != nil {
			return err
		}
		return SaveActionSuccessSnapshot(ctx, tx, actionLog, string(snapshotBytes))
	}); err != nil {
		return nil, nil, err
	}
	return room, sess, nil
}

// initialDeal はラウンド開始時の4枚配札（プレイヤー2・ディーラー2）を行う。
func (u *roomService) initialDeal(sess *model.GameSession, p *model.PlayerState, d *model.DealerState) error {
	deal := func(hand []model.StoredCard, c model.StoredCard) ([]model.StoredCard, error) {
		return u.engine.ApplyPlayerHit(hand, c)
	}
	c1, err := sess.DrawCard()
	if err != nil {
		return err
	}
	ph, err := deal(p.Hand, c1)
	if err != nil {
		return err
	}
	p.Hand = ph
	c2, err := sess.DrawCard()
	if err != nil {
		return err
	}
	dh, err := deal(d.Hand, c2)
	if err != nil {
		return err
	}
	d.Hand = dh
	c3, err := sess.DrawCard()
	if err != nil {
		return err
	}
	ph, err = deal(p.Hand, c3)
	if err != nil {
		return err
	}
	p.Hand = ph
	c4, err := sess.DrawCard()
	if err != nil {
		return err
	}
	dh, err = deal(d.Hand, c4)
	if err != nil {
		return err
	}
	d.Hand = dh
	return nil
}

// dealerresult はディーラー手を公開し勝敗・round_log 用ペイロードを確定する。
func (u *roomService) dealerresult(sess *model.GameSession, p *model.PlayerState, d *model.DealerState, now time.Time) (*model.RoundLog, error) {
	d.RevealHole()
	if err := gameSessionTransition(sess, model.SessionStatusResult); err != nil {
		return nil, err
	}
	if err := gameSessionTransition(sess, model.SessionStatusResetting); err != nil {
		return nil, err
	}
	sess.SetRematchDeadline(now)
	pScore := u.evaluator.Value(p.Hand)
	dScore := u.evaluator.Value(d.Hand)
	outcome, err := u.engine.ResolveOutcome(u.evaluator, p.Hand, d.Hand)
	if err != nil {
		return nil, err
	}
	if err := playerSetOutcomeUC(p, pScore, outcome); err != nil {
		return nil, err
	}
	d.SetFinalScore(dScore)
	payloadBytes, err := marshalGameJSON(map[string]any{
		"player_score": pScore,
		"dealer_score": dScore,
		"outcome":      outcome,
	})
	if err != nil {
		return nil, err
	}
	return &model.RoundLog{
		SessionID:     sess.ID,
		RoundNo:       sess.RoundNo,
		ResultPayload: string(payloadBytes),
		CreatedAt:     now,
	}, nil
}

// dealerresultTimeForfeit は操作時間切れでプレイヤーを敗北に確定する（セッションはディーラーターン前提）。
func (u *roomService) dealerresultTimeForfeit(sess *model.GameSession, p *model.PlayerState, d *model.DealerState, now time.Time) (*model.RoundLog, error) {
	d.RevealHole()
	if err := gameSessionTransition(sess, model.SessionStatusResult); err != nil {
		return nil, err
	}
	if err := gameSessionTransition(sess, model.SessionStatusResetting); err != nil {
		return nil, err
	}
	sess.SetRematchDeadline(now)
	pScore := u.evaluator.Value(p.Hand)
	dScore := u.evaluator.Value(d.Hand)
	if err := playerSetOutcomeUC(p, pScore, model.OutcomeLose); err != nil {
		return nil, err
	}
	d.SetFinalScore(dScore)
	payloadBytes, err := marshalGameJSON(map[string]any{
		"player_score": pScore,
		"dealer_score": dScore,
		"outcome":      model.OutcomeLose,
		"time_forfeit": true,
	})
	if err != nil {
		return nil, err
	}
	return &model.RoundLog{
		SessionID:     sess.ID,
		RoundNo:       sess.RoundNo,
		ResultPayload: string(payloadBytes),
		CreatedAt:     now,
	}, nil
}

// MarkConnected は WS 接続時に room_players を ACTIVE に戻す。
func (u *roomService) MarkConnected(ctx context.Context, roomID, userID string) error {
	if roomID == "" || userID == "" {
		return ErrInvalidInput
	}
	p, err := u.store.GetRoomPlayer(ctx, roomID, userID)
	if err != nil {
		return err
	}
	if p.Status == model.RoomPlayerLeft || p.Status == model.RoomPlayerActive {
		return nil
	}
	if err := roomPlayerSetStatusUC(p, model.RoomPlayerActive); err != nil {
		return err
	}
	return u.store.UpdateRoomPlayer(ctx, p)
}

// MarkDisconnected は WS 切断時に room_players を DISCONNECTED にする。
func (u *roomService) MarkDisconnected(ctx context.Context, roomID, userID string) error {
	if roomID == "" || userID == "" {
		return ErrInvalidInput
	}
	p, err := u.store.GetRoomPlayer(ctx, roomID, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil
		}
		return err
	}
	if p.Status == model.RoomPlayerLeft || p.Status == model.RoomPlayerDisconnected {
		return nil
	}
	if err := roomPlayerSetStatusUC(p, model.RoomPlayerDisconnected); err != nil {
		return err
	}
	return u.store.UpdateRoomPlayer(ctx, p)
}

// AutoStandDueSessions はタイムアウト・ディーラー進行・再戦締切をまとめて処理し、更新があった room_id を返す。
func (u *roomService) AutoStandDueSessions(ctx context.Context) ([]string, error) {
	now := time.Now().UTC()
	sessions, err := u.store.ListSessionsByStatusAndDeadlineBefore(ctx, model.SessionStatusPlayerTurn, now)
	if err != nil {
		return nil, err
	}
	updatedRooms := make([]string, 0, len(sessions))
	seen := map[string]struct{}{}
	for _, sess := range sessions {
		if err := u.playerStand(ctx, sess.ID); err != nil && err != repository.ErrNotFound && err != model.ErrVersionConflict {
			return nil, err
		}
		if _, ok := seen[sess.RoomID]; !ok {
			seen[sess.RoomID] = struct{}{}
			updatedRooms = append(updatedRooms, sess.RoomID)
		}
	}
	dealerSessions, err := u.store.ListSessionsByStatus(ctx, model.SessionStatusDealerTurn)
	if err != nil {
		return nil, err
	}
	for _, sess := range dealerSessions {
		if err := u.dealerTurn(ctx, sess.ID); err != nil && err != repository.ErrNotFound && err != model.ErrVersionConflict {
			return nil, err
		}
		if _, ok := seen[sess.RoomID]; !ok {
			seen[sess.RoomID] = struct{}{}
			updatedRooms = append(updatedRooms, sess.RoomID)
		}
	}

	remDue, err := u.store.ListResettingSessionsDueBy(ctx, now)
	if err != nil {
		return nil, err
	}
	for _, sess := range remDue {
		if err := u.processRematchDeadline(ctx, sess.ID); err != nil && err != repository.ErrNotFound && err != model.ErrVersionConflict {
			return nil, err
		}
		if _, ok := seen[sess.RoomID]; !ok {
			seen[sess.RoomID] = struct{}{}
			updatedRooms = append(updatedRooms, sess.RoomID)
		}
	}
	return updatedRooms, nil
}

// processRematchDeadline は RESETTING の再戦締切到達時に成⽴/不成⽴を確定する。
func (u *roomService) processRematchDeadline(ctx context.Context, sessionID string) error {
	return u.store.Transaction(ctx, func(tx repository.Store) error {
		sess, err := tx.GetSessionForUpdate(ctx, sessionID)
		if err != nil {
			return err
		}
		if sess.Status != model.SessionStatusResetting {
			return nil
		}
		now := time.Now().UTC()
		if sess.RematchDeadlineAt == nil || sess.RematchDeadlineAt.After(now) {
			return nil
		}
		room, err := tx.GetRoom(ctx, sess.RoomID)
		if err != nil {
			return err
		}
		if room.CurrentSessionID == nil || *room.CurrentSessionID != sess.ID {
			return nil
		}
		rps, err := tx.ListRoomPlayersByRoomID(ctx, room.ID)
		if err != nil {
			return err
		}
		eligible := rematchEligibleUserIDs(rps)
		votes, err := tx.ListRematchVotes(ctx, sess.ID)
		if err != nil && err != repository.ErrNotFound {
			return err
		}
		agreeMap := rematchAgreeMapAtDeadline(eligible, votes)
		if len(eligible) == 0 {
			return u.finalizeRematchFailureTx(ctx, tx, room)
		}
		if model.RematchUnanimous(eligible, agreeMap) {
			_, err := u.rematchUnanimousSuccessTx(ctx, tx, room, sess, eligible[0], now, sess.Version)
			return err
		}
		return u.finalizeRematchFailureTx(ctx, tx, room)
	})
}

// playerStand はプレイヤーターン締切超過時に操作時間切れ敗北を適用する（ヒューリスティック時は既存どおり Hit を試みる）。
func (u *roomService) playerStand(ctx context.Context, sessionID string) error {
	sess, err := u.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.Status != model.SessionStatusPlayerTurn || sess.TurnDeadlineAt == nil || sess.TurnDeadlineAt.After(time.Now().UTC()) {
		return nil
	}
	room, err := u.store.GetRoom(ctx, sess.RoomID)
	if err != nil {
		return err
	}
	players, err := u.store.ListPlayerStatesBySessionID(ctx, sess.ID)
	if err != nil {
		return err
	}
	if len(players) == 0 {
		return repository.ErrNotFound
	}
	player := players[0]
	dealer, err := u.store.GetDealerState(ctx, sess.ID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(os.Getenv("BLACKJACK_PLAYER_TIMEOUT_POLICY")) == "heuristic" && len(dealer.Hand) > 0 {
		if model.RecommendHitOrStand(u.evaluator, player.Hand, dealer.Hand[0]) {
			// 通常の Hit ユースケースを通して version/冪等ルールを再利用する。
			actionID := "auto-heuristic-hit:" + sessionID + ":" + strconv.FormatInt(sess.Version, 10)
			_, _, err := u.Hit(ctx, room.ID, player.UserID, sess.Version, actionID)
			return err
		}
	}
	now := time.Now().UTC()
	if err := playerStateSetStatus(player, model.PlayerStatusStand); err != nil {
		return err
	}
	if err := gameSessionTransition(sess, model.SessionStatusDealerTurn); err != nil {
		return err
	}
	sess.SetTurnDeadline(nil)
	sessPrev := sess.Version
	roundLog, derr := u.dealerresultTimeForfeit(sess, player, dealer, now)
	if derr != nil {
		return derr
	}
	room.CurrentSessionID = &sess.ID
	if err := roomRecalculateStatus(room, 1, true); err != nil {
		return err
	}
	room.Touch(now)
	sess.IncrementVersion()
	sess.Touch(now)
	sysActionID := "time-forfeit:" + sessionID + ":" + strconv.FormatInt(sess.Version, 10)
	hash := sha256.Sum256([]byte("TIME_FORFEIT:" + strconv.FormatInt(sess.Version, 10)))
	actionLog := &model.ActionLog{
		SessionID:   sess.ID,
		ActorType:   model.ActorTypeSystem,
		ActorUserID: player.UserID, TargetUserID: player.UserID,
		ActionID:           sysActionID,
		RequestType:        "TIME_FORFEIT",
		RequestPayloadHash: hex.EncodeToString(hash[:]),
	}
	err = u.store.Transaction(ctx, func(tx repository.Store) error {
		ok, err := tx.UpdateSessionIfVersion(ctx, sess, sessPrev)
		if err != nil {
			return err
		}
		if !ok {
			return model.ErrVersionConflict
		}
		if err := tx.UpdatePlayerState(ctx, player); err != nil {
			return err
		}
		if err := tx.UpdateDealerState(ctx, dealer); err != nil {
			return err
		}
		if err := tx.CreateRoundLog(ctx, roundLog); err != nil {
			return err
		}
		if err := tx.UpdateRoom(ctx, room); err != nil {
			return err
		}
		snapshotBytes, err := marshalGameJSON(map[string]any{
			"room_id":    room.ID,
			"session_id": sess.ID,
			"version":    sess.Version,
		})
		if err != nil {
			return err
		}
		return SaveActionSuccessSnapshot(ctx, tx, actionLog, string(snapshotBytes))
	})
	if err == nil {
		observability.IncTimeForfeit()
	}
	return err
}

// dealerTurn はディーラーターンを1手進める（1ドロー or 結果確定）。
func (u *roomService) dealerTurn(ctx context.Context, sessionID string) error {
	sess, err := u.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.Status != model.SessionStatusDealerTurn {
		return nil
	}
	room, err := u.store.GetRoom(ctx, sess.RoomID)
	if err != nil {
		return err
	}
	players, err := u.store.ListPlayerStatesBySessionID(ctx, sess.ID)
	if err != nil {
		return err
	}
	if len(players) == 0 {
		return repository.ErrNotFound
	}
	player := players[0]
	dealer, err := u.store.GetDealerState(ctx, sess.ID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	action, terminal := model.NextDealerAction(u.evaluator, dealer.Hand)
	didDraw := !(terminal || action == model.DealerActionStand)
	sessPrev := sess.Version
	var roundLog *model.RoundLog
	if terminal || action == model.DealerActionStand {
		roundLog, err = u.dealerresult(sess, player, dealer, now)
		if err != nil {
			return err
		}
		room.CurrentSessionID = &sess.ID
		if err := roomRecalculateStatus(room, 1, true); err != nil {
			return err
		}
		room.Touch(now)
	} else {
		card, err := sess.DrawCard()
		if err != nil {
			return err
		}
		nextHand, err := u.engine.ApplyPlayerHit(dealer.Hand, card)
		if err != nil {
			return err
		}
		dealer.Hand = nextHand
	}
	sess.IncrementVersion()
	sess.Touch(now)
	err = u.store.Transaction(ctx, func(tx repository.Store) error {
		ok, err := tx.UpdateSessionIfVersion(ctx, sess, sessPrev)
		if err != nil {
			return err
		}
		if !ok {
			return model.ErrVersionConflict
		}
		if err := tx.UpdateDealerState(ctx, dealer); err != nil {
			return err
		}
		if roundLog != nil {
			if err := tx.UpdatePlayerState(ctx, player); err != nil {
				return err
			}
			if err := tx.CreateRoundLog(ctx, roundLog); err != nil {
				return err
			}
			if err := tx.UpdateRoom(ctx, room); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil && didDraw {
		observability.IncDealerDraw()
	}
	return err
}

// newShuffledDeck は52枚の山札を生成してシャッフルする。
func newShuffledDeck() []model.StoredCard {
	suits := []string{"S", "H", "D", "C"}
	ranks := []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	deck := make([]model.StoredCard, 0, 52)
	for _, s := range suits {
		for _, r := range ranks {
			deck = append(deck, model.StoredCard{Rank: r, Suit: s})
		}
	}
	for i := len(deck) - 1; i > 0; i-- {
		n, err := shuffleIntn(big.NewInt(int64(i + 1)))
		if err != nil {
			j := 0
			deck[i], deck[j] = deck[j], deck[i]
			continue
		}
		j := int(n.Int64())
		deck[i], deck[j] = deck[j], deck[i]
	}
	return deck
}
