package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"blackjack/backend/auditlog"
	"blackjack/backend/dto"
	"blackjack/backend/middleware"
	"blackjack/backend/model"
	"blackjack/backend/observability"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

type wsConnMeta struct {
	userID  string
	epoch   int64
	writeMu *sync.Mutex
}

type roomHub struct {
	mu     sync.Mutex
	rooms  map[string]map[*websocket.Conn]wsConnMeta
	latest map[string]*websocket.Conn
}

var globalRoomHub = &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}, latest: map[string]*websocket.Conn{}}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	wsEpochRedis *redis.Client
	wsEpochTTL   = 2 * time.Minute
)

// ConfigureWebSocketAllowedOrigins は本番用に WS の Origin を制限する。
// origins が空のときは CheckOrigin が常に true（ローカル開発向け）。1 件以上あるときは Origin ヘッダがいずれかと完全一致する場合のみ許可する。
func ConfigureWebSocketAllowedOrigins(origins []string) {
	trimmed := make([]string, 0, len(origins))
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o != "" {
			trimmed = append(trimmed, o)
		}
	}
	if len(trimmed) == 0 {
		wsUpgrader.CheckOrigin = func(r *http.Request) bool { return true }
		return
	}
	wsUpgrader.CheckOrigin = func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		for _, o := range trimmed {
			if o == origin {
				return true
			}
		}
		return false
	}
}

// wsShouldMarkDisconnected は環境変数 BLACKJACK_WS_MARK_DISCONNECTED（true/false）で WS 切断時の DISCONNECTED 反映を制御する。未設定は true。
func wsShouldMarkDisconnected() bool {
	v := strings.TrimSpace(os.Getenv("BLACKJACK_WS_MARK_DISCONNECTED"))
	if v == "" {
		return true
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return b
}

// ConfigureWebSocketConnectionEpochStore configures Redis-backed connection_epoch enforcement (§13.3).
// If rdb is nil, epoch checks are disabled and single-process in-memory latest-only behavior remains.
func ConfigureWebSocketConnectionEpochStore(rdb *redis.Client, ttl time.Duration) {
	wsEpochRedis = rdb
	if ttl > 0 {
		wsEpochTTL = ttl
	}
}

func wsEpochCounterKey(roomID, userID string) string {
	return fmt.Sprintf("ws:room:%s:user:%s:epoch_counter", roomID, userID)
}

func wsEpochLatestKey(roomID, userID string) string {
	return fmt.Sprintf("ws:room:%s:user:%s:latest_epoch", roomID, userID)
}

func registerConnectionEpoch(ctx context.Context, roomID, userID string) (int64, error) {
	if wsEpochRedis == nil {
		return 0, nil
	}
	epoch, err := wsEpochRedis.Incr(ctx, wsEpochCounterKey(roomID, userID)).Result()
	if err != nil {
		return 0, err
	}
	if err := wsEpochRedis.Set(ctx, wsEpochLatestKey(roomID, userID), epoch, wsEpochTTL).Err(); err != nil {
		return 0, err
	}
	return epoch, nil
}

func refreshConnectionEpoch(ctx context.Context, roomID, userID string, epoch int64) error {
	if wsEpochRedis == nil || epoch <= 0 {
		return nil
	}
	return wsEpochRedis.Set(ctx, wsEpochLatestKey(roomID, userID), epoch, wsEpochTTL).Err()
}

func isCurrentConnectionEpoch(ctx context.Context, roomID, userID string, epoch int64) (bool, error) {
	if wsEpochRedis == nil || epoch <= 0 {
		return true, nil
	}
	current, err := wsEpochRedis.Get(ctx, wsEpochLatestKey(roomID, userID)).Int64()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return current == epoch, nil
}

// RoomWS は卓用 WebSocket（受信ループ・レート制限・多重接続の最新のみ有効）。
func (r *RoomController) RoomWS(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	if userID == "" || roomID == "" {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "user and room are required"))
	}
	_, sess, err := r.room.GetRoom(c.Request().Context(), roomID, userID)
	if err != nil {
		return c.JSON(http.StatusForbidden, dto.Fail("forbidden", "room access denied"))
	}
	if r.limiter != nil {
		// WS 接続確立（Upgrade）にもレート制限を適用する。
		ok, err := r.limiter.Allow(c.Request().Context(), "ws-open:"+userID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
		}
		if !ok {
			return c.JSON(http.StatusTooManyRequests, dto.Fail("rate_limited", "too many websocket connection attempts"))
		}
	}
	conn, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	// 最新接続のみ有効化するため、同一 room+userの古い接続はclose
	epoch, err := registerConnectionEpoch(c.Request().Context(), roomID, userID)
	if err != nil {
		_ = conn.Close()
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	meta := wsConnMeta{userID: userID, epoch: epoch, writeMu: &sync.Mutex{}}
	if err := r.room.MarkConnected(c.Request().Context(), roomID, userID); err != nil && err != repository.ErrNotFound {
		_ = conn.Close()
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	old := globalRoomHub.add(roomID, conn, meta)
	observability.IncActiveWSConnections()
	if old != nil {
		observability.IncReconnect()
		_ = old.Close()
	}
	var gameSessPtr *string
	if sess != nil {
		sid := sess.ID
		gameSessPtr = &sid
	}
	middleware.SetAuditGameSessionID(c, gameSessPtr)
	middleware.SetAuditExtra(c, map[string]any{
		"audit_event":      "WS_CONNECTION_EPOCH_ASSIGNED",
		"connection_epoch": epoch,
	})
	r.broadcastRoomState(c.Request().Context(), roomID, userID, "ROOM_STATE_SYNC")
	go func() {
		defer func() {
			isLatest := globalRoomHub.isLatest(roomID, userID, conn)
			isCurrentEpoch, epochErr := isCurrentConnectionEpoch(context.Background(), roomID, userID, meta.epoch)
			if epochErr != nil {
				// Redis 瞬断時は誤切断を避けるため in-memory latest 判定のみで継続する。
				isCurrentEpoch = true
			}
			if isLatest && isCurrentEpoch && wsShouldMarkDisconnected() {
				_ = r.room.MarkDisconnected(context.Background(), roomID, userID)
			}
			globalRoomHub.remove(roomID, conn)
			observability.DecActiveWSConnections()
			_ = conn.Close()
		}()
		for {
			msgStart := time.Now().UTC()
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if r.limiter != nil {
				// WS更新イベントにもRedis token bucketを適用する
				ok, err := r.limiter.Allow(context.Background(), "ws:"+userID)
				if err != nil {
					sendWSError(conn, meta, dto.WSErrorInternal, err.Error())
					continue
				}
				if !ok {
					sendWSError(conn, meta, dto.WSErrorRateLimited, "too many requests")
					continue
				}
			}
			var req dto.WSActionRequest
			if err := json.Unmarshal(msg, &req); err != nil {
				sendWSError(conn, meta, dto.WSErrorInvalidInput, "invalid message payload")
				continue
			}
			if err := refreshConnectionEpoch(context.Background(), roomID, userID, meta.epoch); err != nil {
				sendWSError(conn, meta, dto.WSErrorInternal, err.Error())
				continue
			}
			isCurrentEpoch, err := isCurrentConnectionEpoch(context.Background(), roomID, userID, meta.epoch)
			if err != nil {
				sendWSError(conn, meta, dto.WSErrorInternal, err.Error())
				continue
			}
			if !isCurrentEpoch {
				logWSEvent(c, req, roomID, userID, nil, nil, nil, msgStart, "failure", dto.WSErrorForbidden, map[string]any{
					"audit_event":      "WS_CONNECTION_EPOCH_SUPERSEDED",
					"connection_epoch": meta.epoch,
				})
				sendWSError(conn, meta, dto.WSErrorForbidden, "stale websocket connection")
				return
			}
			switch req.Type {
			case dto.WSEventHit:
				// 更新系: action_id + expected_version
				if req.ActionID == "" || req.ExpectedVersion <= 0 {
					sendWSError(conn, meta, dto.WSErrorInvalidInput, "action_id and expected_version are required")
					continue
				}
				_, sess, err := r.room.Hit(context.Background(), roomID, userID, req.ExpectedVersion, req.ActionID)
				ev := req.ExpectedVersion
				if err != nil {
					code, message := mapWSError(err)
					logWSEvent(c, req, roomID, userID, nil, &ev, &ev, msgStart, "failure", code, nil)
					sendWSError(conn, meta, code, message)
					continue
				}
				sv := sess.Version
				gid := sess.ID
				logWSEvent(c, req, roomID, userID, &gid, &ev, &sv, msgStart, "success", "", nil)
				r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
			case dto.WSEventStand:
				// 更新系: STAND も HIT と同じ検証・整合性フローで処理する
				if req.ActionID == "" || req.ExpectedVersion <= 0 {
					sendWSError(conn, meta, dto.WSErrorInvalidInput, "action_id and expected_version are required")
					continue
				}
				_, sess, err := r.room.Stand(context.Background(), roomID, userID, req.ExpectedVersion, req.ActionID)
				ev := req.ExpectedVersion
				if err != nil {
					code, message := mapWSError(err)
					logWSEvent(c, req, roomID, userID, nil, &ev, &ev, msgStart, "failure", code, nil)
					sendWSError(conn, meta, code, message)
					continue
				}
				sv := sess.Version
				gid := sess.ID
				logWSEvent(c, req, roomID, userID, &gid, &ev, &sv, msgStart, "success", "", nil)
				r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
			case dto.WSEventRematchVote:
				// 再戦投票は WS のみ受け付ける（HTTP fallback なし）
				if req.ActionID == "" || req.ExpectedVersion <= 0 || req.Agree == nil {
					sendWSError(conn, meta, dto.WSErrorInvalidInput, "agree, action_id and expected_version are required")
					continue
				}
				_, sess, err := r.room.VoteRematch(context.Background(), roomID, userID, *req.Agree, req.ExpectedVersion, req.ActionID)
				ev := req.ExpectedVersion
				if err != nil {
					code, message := mapWSError(err)
					logWSEvent(c, req, roomID, userID, nil, &ev, &ev, msgStart, "failure", code, nil)
					sendWSError(conn, meta, code, message)
					continue
				}
				sv := sess.Version
				gid := sess.ID
				logWSEvent(c, req, roomID, userID, &gid, &ev, &sv, msgStart, "success", "", nil)
				r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
			case dto.WSEventRoomSyncReq:
				// 読み取り系同期要求は version 不一致をエラーにせず、正本を再送する
				logWSEvent(c, req, roomID, userID, nil, nil, nil, msgStart, "success", "", nil)
				r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
			case dto.WSEventPing:
				// 接続が生きているか確認
				logWSEvent(c, req, roomID, userID, nil, nil, nil, msgStart, "success", "", nil)
				sendWSPong(conn, meta)
			default:
				logWSEvent(c, req, roomID, userID, nil, nil, nil, msgStart, "failure", dto.WSErrorInvalidInput, nil)
				sendWSError(conn, meta, dto.WSErrorInvalidInput, "unsupported ws event type")
			}
		}
	}()
	return nil
}

// broadcastRoomState は自インスタンスの WS 接続へ配信し、続けて Redis Pub/Sub で他インスタンスへ伝える（Phase 3）。
func (r *RoomController) broadcastRoomState(ctx context.Context, roomID, actorUserID, eventType string) {
	r.broadcastRoomStateLocal(ctx, roomID, actorUserID, eventType)
	if r.syncBroker != nil {
		_ = r.syncBroker.Publish(context.Background(), roomID, eventType)
	}
}

// BroadcastRoomStateFromPeer は他インスタンスからの Pub/Sub 通知に応じ、ローカル接続のみへ同期する（再 publish しない）。
func (r *RoomController) BroadcastRoomStateFromPeer(ctx context.Context, roomID, eventType string) {
	r.broadcastRoomStateLocal(ctx, roomID, "", eventType)
}

// broadcastRoomStateLocal は卓の全接続へユーザー別に ROOM_STATE_SYNC を送る。
func (r *RoomController) broadcastRoomStateLocal(ctx context.Context, roomID, actorUserID, eventType string) {
	// room参加中の全接続へ、ユーザーごとの公開範囲で state を組み立てて送信する
	snapshot := globalRoomHub.snapshot(roomID)
	for conn, meta := range snapshot {
		state, err := r.room.GetRoomState(ctx, roomID, meta.userID)
		if err != nil {
			continue
		}
		payload := dto.WSRoomStateSyncEvent{
			Type: eventType,
			Data: buildRoomStateSyncDTO(state, meta.userID),
		}
		b, err := json.Marshal(payload)
		if err != nil {
			continue
		}
		if err := writeWS(conn, meta, b); err != nil {
			globalRoomHub.remove(roomID, conn)
			_ = conn.Close()
		}
	}
}

// BroadcastRoomSync はバックグラウンド処理後に同期イベントを送る。
func (r *RoomController) BroadcastRoomSync(ctx context.Context, roomID string) {
	r.broadcastRoomState(ctx, roomID, "", dto.WSEventRoomSync)
}

// writeWS はコネクション単位で書き込みを直列化する。
func writeWS(conn *websocket.Conn, meta wsConnMeta, payload []byte) error {
	meta.writeMu.Lock()
	defer meta.writeMu.Unlock()
	start := time.Now()
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	err := conn.WriteMessage(websocket.TextMessage, payload)
	observability.ObserveWSSendLatency(time.Since(start).Seconds())
	return err
}

// sendWSError は WS 用の ERROR ペイロードを送る。
func sendWSError(conn *websocket.Conn, meta wsConnMeta, code, message string) {
	// エラー契約は { type: "ERROR", error: { code, message } } で固定
	b, err := json.Marshal(dto.WSErrorEvent{
		Type: dto.WSEventError,
		Error: dto.WSErrorBody{
			Code:    code,
			Message: message,
		},
	})
	if err != nil {
		return
	}
	_ = writeWS(conn, meta, b)
}

// sendWSPong は PING に対する PONG。
func sendWSPong(conn *websocket.Conn, meta wsConnMeta) {
	b, err := json.Marshal(map[string]string{"type": dto.WSEventPong})
	if err != nil {
		return
	}
	_ = writeWS(conn, meta, b)
}

// mapWSError はドメインエラーを WS の code/message に変換する。
func mapWSError(err error) (string, string) {
	switch err {
	case usecase.ErrUnauthorizedUser:
		return dto.WSErrorUnauthorized, "login required"
	case usecase.ErrForbiddenAction:
		return dto.WSErrorForbidden, "room access denied"
	case usecase.ErrInvalidInput:
		return dto.WSErrorInvalidInput, "action_id and expected_version are required"
	case usecase.ErrInvalidGameState, model.ErrNotPlayerTurn, model.ErrNotYourTurn, model.ErrInvalidPlayerStatus:
		return dto.WSErrorInvalidGame, err.Error()
	case model.ErrRoomFull:
		return dto.WSErrorRoomFull, "room is full"
	case model.ErrVersionConflict:
		observability.IncVersionConflict()
		return dto.WSErrorVersionConflict, "session version conflict"
	case model.ErrDuplicateAction:
		observability.IncDuplicateAction()
		return dto.WSErrorDuplicateAction, "action id already used with different payload"
	case repository.ErrNotFound:
		return dto.WSErrorNotFound, "room or session not found"
	default:
		return dto.WSErrorInternal, err.Error()
	}
}

// buildRoomStateSyncDTO は閲覧者ごとの手札公開範囲を反映した同期ペイロードを組み立てる。
func buildRoomStateSyncDTO(s *usecase.RoomState, userID string) dto.RoomStateSyncPayload {
	out := dto.RoomStateSyncPayload{
		Room: dto.RoomJSON{ID: s.Room.ID, Status: string(s.Room.Status)},
		Session: dto.RoomStateSyncSessionJSON{
			ID:                nil,
			Status:            nil,
			Version:           nil,
			RoundNo:           nil,
			TurnSeat:          nil,
			TurnDeadlineAt:    nil,
			RematchDeadlineAt: nil,
		},
		Dealer: dto.DealerJSON{
			VisibleCards: []string{},
			Hidden:       false,
			CardCount:    0,
		},
		Players: []dto.PlayerJSON{},
	}
	if s.Session != nil {
		id := s.Session.ID
		status := string(s.Session.Status)
		version := s.Session.Version
		roundNo := s.Session.RoundNo
		turnSeat := s.Session.TurnSeat
		out.Session.ID = &id
		out.Session.Status = &status
		out.Session.Version = &version
		out.Session.RoundNo = &roundNo
		out.Session.TurnSeat = &turnSeat
		if s.Session.TurnDeadlineAt != nil {
			v := s.Session.TurnDeadlineAt.UTC().Format(time.RFC3339)
			out.Session.TurnDeadlineAt = &v
		}
		if s.Session.RematchDeadlineAt != nil {
			v := s.Session.RematchDeadlineAt.UTC().Format(time.RFC3339)
			out.Session.RematchDeadlineAt = &v
		}
	}
	if s.Dealer != nil {
		visible := make([]string, 0, len(s.Dealer.Hand))
		for i, c := range s.Dealer.Hand {
			if s.Dealer.HoleHidden && i == 1 {
				continue
			}
			visible = append(visible, c.Rank+c.Suit)
		}
		out.Dealer = dto.DealerJSON{
			VisibleCards: visible,
			Hidden:       s.Dealer.HoleHidden,
			CardCount:    len(s.Dealer.Hand),
		}
	}
	players := make([]dto.PlayerJSON, 0, len(s.Players))
	for _, p := range s.Players {
		item := dto.PlayerJSON{
			UserID:    p.UserID,
			SeatNo:    p.SeatNo,
			Status:    string(p.Status),
			IsMe:      p.UserID == userID,
			CardCount: len(p.Hand),
		}
		if item.IsMe {
			// 仕様どおり自分のhandのみ公開　他人はcard_countのみ返す
			cards := make([]string, 0, len(p.Hand))
			for _, c := range p.Hand {
				cards = append(cards, c.Rank+c.Suit)
			}
			item.Hand = cards
		}
		if p.Outcome != nil {
			v := string(*p.Outcome)
			item.Outcome = &v
		}
		item.FinalScore = p.FinalScore
		players = append(players, item)
	}
	out.Players = players
	out.MyActions = dto.MyActionsJSON{
		CanHit:         s.CanHit,
		CanStand:       s.CanStand,
		CanRematchVote: s.CanRematch,
	}
	return out
}

// add は卓に接続を登録し、同一ユーザーの旧接続があれば返す（切断用）。
func (h *roomHub) add(roomID string, conn *websocket.Conn, meta wsConnMeta) *websocket.Conn {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = map[*websocket.Conn]wsConnMeta{}
	}
	key := roomID + ":" + meta.userID
	old := h.latest[key]
	if old != nil {
		delete(h.rooms[roomID], old)
	}
	h.rooms[roomID][conn] = meta
	h.latest[key] = conn
	return old
}

// remove は卓ハブから接続を外す。
func (h *roomHub) remove(roomID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[roomID] == nil {
		return
	}
	delete(h.rooms[roomID], conn)
	for key, current := range h.latest {
		if current == conn {
			delete(h.latest, key)
		}
	}
	if len(h.rooms[roomID]) == 0 {
		delete(h.rooms, roomID)
	}
}

// snapshot はブロードキャスト用に接続一覧のコピーを取る。
func (h *roomHub) snapshot(roomID string) map[*websocket.Conn]wsConnMeta {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[*websocket.Conn]wsConnMeta{}
	for c, m := range h.rooms[roomID] {
		out[c] = m
	}
	return out
}

// isLatest は当該接続が room+user の最新かどうか（切断時の DB 更新判定）。
func (h *roomHub) isLatest(roomID, userID string, conn *websocket.Conn) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.latest[roomID+":"+userID] == conn
}

// logWSEvent は WS メッセージごとの構造化監査ログを出す（HTTP AuditLogMiddleware と同一スキーマ）。
func logWSEvent(c echo.Context, req dto.WSActionRequest, roomID, userID string, gameSessionID *string, before, after *int64, start time.Time, result, errorCode string, extra map[string]any) {
	reqID := req.RequestID
	if reqID == "" {
		reqID, _ = c.Get(middleware.RequestIDContextKey).(string)
	}
	var gs any
	if gameSessionID != nil {
		gs = *gameSessionID
	}
	entry := auditlog.BuildEntry(
		start,
		reqID,
		req.ActionID,
		roomID,
		c.Get("session_id"),
		gs,
		userID,
		"USER",
		"WS "+req.Type,
		before,
		after,
		time.Since(start).Milliseconds(),
		result,
		errorCode,
		extra,
	)
	auditlog.Info(c.Logger(), entry)
	observability.ObserveWSMessage(req.Type, result, time.Since(start).Seconds())
}
