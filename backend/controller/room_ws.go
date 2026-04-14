package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/middleware"
	"blackjack/backend/model"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

type wsConnMeta struct {
	userID  string
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

// RoomWS は卓用 WebSocket（受信ループ・レート制限・多重接続の最新のみ有効）。
func (r *RoomController) RoomWS(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	if userID == "" || roomID == "" {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "user and room are required"))
	}
	if _, _, err := r.room.GetRoom(c.Request().Context(), roomID, userID); err != nil {
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
	meta := wsConnMeta{userID: userID, writeMu: &sync.Mutex{}}
	if err := r.room.MarkConnected(c.Request().Context(), roomID, userID); err != nil && err != repository.ErrNotFound {
		_ = conn.Close()
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	old := globalRoomHub.add(roomID, conn, meta)
	if old != nil {
		_ = old.Close()
	}
	r.broadcastRoomState(c.Request().Context(), roomID, userID, "ROOM_STATE_SYNC")
	go func() {
		defer func() {
			if globalRoomHub.isLatest(roomID, userID, conn) {
				_ = r.room.MarkDisconnected(context.Background(), roomID, userID)
			}
			globalRoomHub.remove(roomID, conn)
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
			switch req.Type {
			case dto.WSEventHit:
				// 更新系: action_id + expected_version
				if req.ActionID == "" || req.ExpectedVersion <= 0 {
					sendWSError(conn, meta, dto.WSErrorInvalidInput, "action_id and expected_version are required")
					continue
				}
				_, _, err := r.room.Hit(context.Background(), roomID, userID, req.ExpectedVersion, req.ActionID)
				if err != nil {
					code, message := mapWSError(err)
					logWSEvent(c, req, roomID, userID, req.ExpectedVersion, req.ExpectedVersion, msgStart, "failure", code)
					sendWSError(conn, meta, code, message)
					continue
				}
				logWSEvent(c, req, roomID, userID, req.ExpectedVersion, req.ExpectedVersion+1, msgStart, "success", "")
				r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
			case dto.WSEventStand:
				// 更新系: STAND も HIT と同じ検証・整合性フローで処理する
				if req.ActionID == "" || req.ExpectedVersion <= 0 {
					sendWSError(conn, meta, dto.WSErrorInvalidInput, "action_id and expected_version are required")
					continue
				}
				_, _, err := r.room.Stand(context.Background(), roomID, userID, req.ExpectedVersion, req.ActionID)
				if err != nil {
					code, message := mapWSError(err)
					logWSEvent(c, req, roomID, userID, req.ExpectedVersion, req.ExpectedVersion, msgStart, "failure", code)
					sendWSError(conn, meta, code, message)
					continue
				}
				logWSEvent(c, req, roomID, userID, req.ExpectedVersion, req.ExpectedVersion+1, msgStart, "success", "")
				r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
			case dto.WSEventRematchVote:
				// 再戦投票は WS のみ受け付ける（HTTP fallback なし）
				if req.ActionID == "" || req.ExpectedVersion <= 0 || req.Agree == nil {
					sendWSError(conn, meta, dto.WSErrorInvalidInput, "agree, action_id and expected_version are required")
					continue
				}
				_, _, err := r.room.VoteRematch(context.Background(), roomID, userID, *req.Agree, req.ExpectedVersion, req.ActionID)
				if err != nil {
					code, message := mapWSError(err)
					logWSEvent(c, req, roomID, userID, req.ExpectedVersion, req.ExpectedVersion, msgStart, "failure", code)
					sendWSError(conn, meta, code, message)
					continue
				}
				logWSEvent(c, req, roomID, userID, req.ExpectedVersion, req.ExpectedVersion+1, msgStart, "success", "")
				r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
			case dto.WSEventRoomSyncReq:
				// 読み取り系同期要求は version 不一致をエラーにせず、正本を再送する
				logWSEvent(c, req, roomID, userID, 0, 0, msgStart, "success", "")
				r.broadcastRoomState(context.Background(), roomID, userID, dto.WSEventRoomSync)
			case dto.WSEventPing:
				// 接続が生きているか確認
				logWSEvent(c, req, roomID, userID, 0, 0, msgStart, "success", "")
				sendWSPong(conn, meta)
			default:
				logWSEvent(c, req, roomID, userID, 0, 0, msgStart, "failure", dto.WSErrorInvalidInput)
				sendWSError(conn, meta, dto.WSErrorInvalidInput, "unsupported ws event type")
			}
		}
	}()
	return nil
}

// broadcastRoomState は卓の全接続へユーザー別に ROOM_STATE_SYNC を送る。
func (r *RoomController) broadcastRoomState(ctx context.Context, roomID, actorUserID, eventType string) {
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
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, payload)
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
		return dto.WSErrorVersionConflict, "session version conflict"
	case model.ErrDuplicateAction:
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

// logWSEvent は WS メッセージごとの構造化監査ログを出す。
func logWSEvent(c echo.Context, req dto.WSActionRequest, roomID, userID string, before, after int64, start time.Time, result, errorCode string) {
	reqID := req.RequestID
	if reqID == "" {
		reqID, _ = c.Get(middleware.RequestIDContextKey).(string)
	}
	entry := map[string]any{
		"timestamp":              start.Format(time.RFC3339Nano),
		"level":                  "INFO",
		"request_id":             reqID,
		"action_id":              req.ActionID,
		"room_id":                roomID,
		"session_id":             c.Get("session_id"),
		"user_id":                userID,
		"actor_type":             "USER",
		"request_type":           "WS " + req.Type,
		"session_version_before": before,
		"session_version_after":  after,
		"latency_ms":             time.Since(start).Milliseconds(),
		"result":                 result,
		"error_code":             errorCode,
	}
	if b, err := json.Marshal(entry); err == nil {
		c.Logger().Info(string(b))
	}
}
