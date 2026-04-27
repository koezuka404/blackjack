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

	"blackjack/backend/dto"
	"blackjack/backend/jwtauth"
	"blackjack/backend/middleware"
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

func allowAllWSOrigins(_ *http.Request) bool {
	return true
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: allowAllWSOrigins,
}

var (
	wsEpochRedis  *redis.Client
	wsEpochTTL    = 2 * time.Minute
	wsEpochIncrFn = func(ctx context.Context, rdb *redis.Client, key string) (int64, error) {
		return rdb.Incr(ctx, key).Result()
	}
	wsEpochSetFn = func(ctx context.Context, rdb *redis.Client, key string, value any, ttl time.Duration) error {
		return rdb.Set(ctx, key, value, ttl).Err()
	}
	wsEpochGetInt64Fn = func(ctx context.Context, rdb *redis.Client, key string) (int64, error) {
		return rdb.Get(ctx, key).Int64()
	}
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
	epoch, err := wsEpochIncrFn(ctx, wsEpochRedis, wsEpochCounterKey(roomID, userID))
	if err != nil {
		return 0, err
	}
	if err := wsEpochSetFn(ctx, wsEpochRedis, wsEpochLatestKey(roomID, userID), epoch, wsEpochTTL); err != nil {
		return 0, err
	}
	return epoch, nil
}

func refreshConnectionEpoch(ctx context.Context, roomID, userID string, epoch int64) error {
	if wsEpochRedis == nil || epoch <= 0 {
		return nil
	}
	return wsEpochSetFn(ctx, wsEpochRedis, wsEpochLatestKey(roomID, userID), epoch, wsEpochTTL)
}

func isCurrentConnectionEpoch(ctx context.Context, roomID, userID string, epoch int64) (bool, error) {
	if wsEpochRedis == nil || epoch <= 0 {
		return true, nil
	}
	current, err := wsEpochGetInt64Fn(ctx, wsEpochRedis, wsEpochLatestKey(roomID, userID))
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return current == epoch, nil
}

func wsAuthReadDeadline() time.Duration {
	raw := strings.TrimSpace(os.Getenv("WS_AUTH_DEADLINE"))
	if raw == "" {
		return 15 * time.Second
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 15 * time.Second
	}
	return d
}

func preWSConnectionKey(c echo.Context) string {
	ip := strings.TrimSpace(c.RealIP())
	if ip == "" {
		ip = c.Request().RemoteAddr
	}
	return "ws-open-pre:" + ip
}

// RoomWS は卓用 WebSocket（Upgrade 直後の AUTH で user 確定、受信ループ・レート制限・多重接続の最新のみ有効）。
func (r *RoomController) RoomWS(c echo.Context) error {
	roomID := c.Param("id")
	if roomID == "" {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "room is required"))
	}
	preKey := preWSConnectionKey(c)
	if r.limiter != nil {
		result, err := r.limiter.Allow(c.Request().Context(), preKey)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
		}
		if !result.Allowed {
			c.Response().Header().Set("X-RateLimit-Retry-After-Ms", strconv.FormatInt(result.RetryAfterMS, 10))
			return c.JSON(http.StatusTooManyRequests, dto.Fail("rate_limited", "too many websocket connection attempts"))
		}
	}
	conn, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	writeMu := &sync.Mutex{}
	preMeta := wsConnMeta{writeMu: writeMu}

	authFrameStart := time.Now().UTC()
	_ = conn.SetReadDeadline(time.Now().Add(wsAuthReadDeadline()))
	_, first, err := conn.ReadMessage()
	if err != nil {
		_ = conn.Close()
		return nil
	}
	_ = conn.SetReadDeadline(time.Time{})

	var authMsg dto.WSAuthMessage
	if err := json.Unmarshal(first, &authMsg); err != nil || authMsg.Type != dto.WSEventAuth || strings.TrimSpace(authMsg.AccessToken) == "" {
		sendWSError(conn, preMeta, dto.WSErrorUnauthorized, "first message must be AUTH with access_token")
		_ = conn.Close()
		return nil
	}
	userID, jti, err := jwtauth.ParseAndValidate(r.jwtSecret, authMsg.AccessToken)
	if err != nil {
		sendWSError(conn, preMeta, dto.WSErrorUnauthorized, "invalid or expired token")
		_ = conn.Close()
		return nil
	}
	_, sess, err := r.room.GetRoom(c.Request().Context(), roomID, userID)
	if err != nil {
		sendWSError(conn, preMeta, dto.WSErrorForbidden, "room access denied")
		_ = conn.Close()
		return nil
	}
	if r.limiter != nil {
		result, err := r.limiter.Allow(c.Request().Context(), "ws-open:"+userID)
		if err != nil {
			sendWSError(conn, preMeta, dto.WSErrorInternal, err.Error())
			_ = conn.Close()
			return nil
		}
		if !result.Allowed {
			sendWSErrorWithRetry(conn, preMeta, dto.WSErrorRateLimited, "too many websocket connection attempts", result.RetryAfterMS)
			_ = conn.Close()
			return nil
		}
	}
	epoch, err := registerConnectionEpoch(c.Request().Context(), roomID, userID)
	if err != nil {
		sendWSError(conn, preMeta, dto.WSErrorInternal, err.Error())
		_ = conn.Close()
		return nil
	}
	meta := wsConnMeta{userID: userID, epoch: epoch, writeMu: writeMu}
	if err := r.room.MarkConnected(c.Request().Context(), roomID, userID); err != nil && err != repository.ErrNotFound {
		sendWSError(conn, preMeta, dto.WSErrorInternal, err.Error())
		_ = conn.Close()
		return nil
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
	r.broadcastRoomState(c.Request().Context(), roomID, userID, dto.WSEventRoomSync)

	reqID, _ := c.Get(middleware.RequestIDContextKey).(string)
	audit := &WsAuditLogContext{Logger: c.Logger(), RequestID: reqID, SessionID: jti}
	logWSEvent(audit, dto.WSActionRequest{Type: dto.WSEventAuth, RequestID: authMsg.RequestID}, roomID, userID, nil, nil, nil, authFrameStart, "success", "", nil)

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
				result, err := r.limiter.Allow(context.Background(), "ws:"+userID)
				if err != nil {
					sendWSError(conn, meta, dto.WSErrorInternal, err.Error())
					continue
				}
				if !result.Allowed {
					sendWSErrorWithRetry(conn, meta, dto.WSErrorRateLimited, "too many requests", result.RetryAfterMS)
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
				logWSEvent(audit, req, roomID, userID, nil, nil, nil, msgStart, "failure", dto.WSErrorForbidden, map[string]any{
					"audit_event":      "WS_CONNECTION_EPOCH_SUPERSEDED",
					"connection_epoch": meta.epoch,
				})
				sendWSError(conn, meta, dto.WSErrorForbidden, "stale websocket connection")
				return
			}
			r.handleGameWSAction(audit, req, roomID, userID, conn, meta, msgStart)
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
			Data: buildRoomDTO(state, meta.userID),
		}
		b, _ := json.Marshal(payload)
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
	sendWSErrorWithRetryPtr(conn, meta, code, message, nil)
}

func sendWSErrorWithRetry(conn *websocket.Conn, meta wsConnMeta, code, message string, retryAfterMS int64) {
	sendWSErrorWithRetryPtr(conn, meta, code, message, &retryAfterMS)
}

func sendWSErrorWithRetryPtr(conn *websocket.Conn, meta wsConnMeta, code, message string, retryAfterMS *int64) {
	// エラー契約は { type: "ERROR", error: { code, message } } で固定
	b, _ := json.Marshal(dto.WSErrorEvent{
		Type: dto.WSEventError,
		Error: dto.WSErrorBody{
			Code:         code,
			Message:      message,
			RetryAfterMS: retryAfterMS,
		},
	})
	_ = writeWS(conn, meta, b)
}

// sendWSPong は PING に対する PONG。
func sendWSPong(conn *websocket.Conn, meta wsConnMeta) {
	b, _ := json.Marshal(map[string]string{"type": dto.WSEventPong})
	_ = writeWS(conn, meta, b)
}

// buildRoomDTO は閲覧者ごとの手札公開範囲を反映した同期ペイロードを組み立てる。
func buildRoomDTO(s *usecase.RoomState, userID string) dto.RoomStateSyncPayload {
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
