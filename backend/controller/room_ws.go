package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"blackjack/backend/dto"
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
	mu    sync.Mutex
	rooms map[string]map[*websocket.Conn]wsConnMeta
}

var globalRoomHub = &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (r *RoomController) RoomWS(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	if userID == "" || roomID == "" {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "user and room are required"))
	}
	if _, _, err := r.room.GetRoom(c.Request().Context(), roomID, userID); err != nil {
		return c.JSON(http.StatusForbidden, dto.Fail("forbidden", "room access denied"))
	}
	conn, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	meta := wsConnMeta{userID: userID, writeMu: &sync.Mutex{}}
	globalRoomHub.add(roomID, conn, meta)
	r.broadcastRoomState(c.Request().Context(), roomID, userID, "ROOM_STATE_SYNC")
	go func() {
		defer func() {
			globalRoomHub.remove(roomID, conn)
			_ = conn.Close()
		}()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req wsActionRequest
			if err := json.Unmarshal(msg, &req); err != nil {
				sendWSError(conn, meta, "invalid_input", "invalid message payload")
				continue
			}
			actionType := strings.ToUpper(req.Type)
			switch actionType {
			case "START":
				_, _, err := r.room.StartRoom(context.Background(), roomID, userID)
				if err != nil {
					code, message := mapWSError(err)
					sendWSError(conn, meta, code, message)
					continue
				}
				r.broadcastRoomState(context.Background(), roomID, userID, "ROOM_STATE_SYNC")
			case "LEAVE":
				_, err := r.room.LeaveRoom(context.Background(), roomID, userID)
				if err != nil {
					code, message := mapWSError(err)
					sendWSError(conn, meta, code, message)
					continue
				}
				r.broadcastRoomState(context.Background(), roomID, userID, "ROOM_STATE_SYNC")
			case "HIT":
				if req.ActionID == "" || req.ExpectedVersion <= 0 {
					sendWSError(conn, meta, "invalid_input", "action_id and expected_version are required")
					continue
				}
				_, _, err := r.room.Hit(context.Background(), roomID, userID, req.ExpectedVersion, req.ActionID)
				if err != nil {
					code, message := mapWSError(err)
					sendWSError(conn, meta, code, message)
					continue
				}
				r.broadcastRoomState(context.Background(), roomID, userID, "ROOM_STATE_SYNC")
			case "STAND":
				if req.ActionID == "" || req.ExpectedVersion <= 0 {
					sendWSError(conn, meta, "invalid_input", "action_id and expected_version are required")
					continue
				}
				_, _, err := r.room.Stand(context.Background(), roomID, userID, req.ExpectedVersion, req.ActionID)
				if err != nil {
					code, message := mapWSError(err)
					sendWSError(conn, meta, code, message)
					continue
				}
				r.broadcastRoomState(context.Background(), roomID, userID, "ROOM_STATE_SYNC")
			case "REMATCH_VOTE":
				if req.ActionID == "" || req.ExpectedVersion <= 0 || req.Agree == nil {
					sendWSError(conn, meta, "invalid_input", "agree, action_id and expected_version are required")
					continue
				}
				_, _, err := r.room.VoteRematch(context.Background(), roomID, userID, *req.Agree, req.ExpectedVersion, req.ActionID)
				if err != nil {
					code, message := mapWSError(err)
					sendWSError(conn, meta, code, message)
					continue
				}
				r.broadcastRoomState(context.Background(), roomID, userID, "ROOM_STATE_SYNC")
			default:
				sendWSError(conn, meta, "invalid_input", "unsupported ws event type")
			}
		}
	}()
	return nil
}

func (r *RoomController) broadcastRoomState(ctx context.Context, roomID, actorUserID, eventType string) {
	snapshot := globalRoomHub.snapshot(roomID)
	for conn, meta := range snapshot {
		state, err := r.room.GetRoomState(ctx, roomID, meta.userID)
		if err != nil {
			continue
		}
		payload := map[string]any{
			"type": eventType,
			"data": buildRoomStateSyncDTO(state, meta.userID),
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

type wsActionRequest struct {
	Type            string `json:"type"`
	ActionID        string `json:"action_id"`
	ExpectedVersion int64  `json:"expected_version"`
	Agree           *bool  `json:"agree,omitempty"`
}

func writeWS(conn *websocket.Conn, meta wsConnMeta, payload []byte) error {
	meta.writeMu.Lock()
	defer meta.writeMu.Unlock()
	_ = conn.SetWriteDeadline(time.Now().Add(3 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, payload)
}

func sendWSError(conn *websocket.Conn, meta wsConnMeta, code, message string) {
	b, err := json.Marshal(map[string]any{
		"type": "ERROR",
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
	if err != nil {
		return
	}
	_ = writeWS(conn, meta, b)
}

func mapWSError(err error) (string, string) {
	switch err {
	case usecase.ErrUnauthorizedUser:
		return "unauthorized", "login required"
	case usecase.ErrForbiddenAction:
		return "forbidden", "room access denied"
	case usecase.ErrInvalidInput:
		return "invalid_input", "action_id and expected_version are required"
	case usecase.ErrInvalidGameState, model.ErrNotPlayerTurn, model.ErrNotYourTurn, model.ErrInvalidPlayerStatus:
		return "invalid_game_state", err.Error()
	case model.ErrRoomFull:
		return "room_full", "room is full"
	case model.ErrVersionConflict:
		return "version_conflict", "session version conflict"
	case model.ErrDuplicateAction:
		return "duplicate_action", "action id already used with different payload"
	case repository.ErrNotFound:
		return "not_found", "room or session not found"
	default:
		return "internal_error", err.Error()
	}
}

func buildRoomStateSyncDTO(s *usecase.RoomState, userID string) dto.RoomStateSyncJSON {
	out := dto.RoomStateSyncJSON{
		Room: dto.RoomJSON{ID: s.Room.ID, Status: string(s.Room.Status)},
	}
	if s.Session != nil {
		out.Session = dto.SessionFromDomain(s.Session, func(t time.Time) string {
			return t.UTC().Format(time.RFC3339)
		})
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
		if item.IsMe || (s.Session != nil && s.Session.Status == model.SessionStatusResult) {
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

func (h *roomHub) add(roomID string, conn *websocket.Conn, meta wsConnMeta) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[roomID] == nil {
		h.rooms[roomID] = map[*websocket.Conn]wsConnMeta{}
	}
	h.rooms[roomID][conn] = meta
}

func (h *roomHub) remove(roomID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[roomID] == nil {
		return
	}
	delete(h.rooms[roomID], conn)
	if len(h.rooms[roomID]) == 0 {
		delete(h.rooms, roomID)
	}
}

func (h *roomHub) snapshot(roomID string) map[*websocket.Conn]wsConnMeta {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[*websocket.Conn]wsConnMeta{}
	for c, m := range h.rooms[roomID] {
		out[c] = m
	}
	return out
}
