package controller

import (
	"net/http"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/middleware"
	"blackjack/backend/model"
	"blackjack/backend/realtime"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)

type RoomController struct {
	room       usecase.RoomUsecase
	limiter    usecase.RateLimitUsecase
	syncBroker *realtime.RoomSyncBroker
}

// NewRoomController はルーム API / WS 用コントローラを生成する。
func NewRoomController(room usecase.RoomUsecase, limiter usecase.RateLimitUsecase, syncBroker *realtime.RoomSyncBroker) *RoomController {
	return &RoomController{room: room, limiter: limiter, syncBroker: syncBroker}
}

// Register は HTTP のルーム系ルートを登録する（HIT/STAND 等）。
func (r *RoomController) Register(g *echo.Group) {
	g.POST("/rooms", r.CreateRoom)
	g.GET("/rooms", r.ListRooms)
	g.POST("/rooms/:id/join", r.JoinRoom)
	g.POST("/rooms/:id/leave", r.LeaveRoom)
	g.GET("/rooms/:id", r.GetRoom)
	g.GET("/rooms/:id/history", r.GetRoomHistory)
	g.GET("/rooms/:id/play_hint", r.GetPlayHint)
	g.POST("/rooms/:id/start", r.StartRoom)
	g.POST("/rooms/:id/hit", r.Hit)
	g.POST("/rooms/:id/stand", r.Stand)
	g.POST("/rooms/:id/reset", r.ResetRoomDebug)
}

// CreateRoom は卓の作成。
func (r *RoomController) CreateRoom(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	room, err := r.room.CreateRoom(c.Request().Context(), userID)
	if err != nil {
		if err == usecase.ErrUnauthorizedUser {
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
		}
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	return c.JSON(http.StatusOK, dto.OK(dto.CreateRoomData{
		Room: roomDetailJSON(room),
	}))
}

// JoinRoom はホストの卓参加。
func (r *RoomController) JoinRoom(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	room, err := r.room.JoinRoom(c.Request().Context(), roomID, userID)
	if err != nil {
		switch err {
		case usecase.ErrUnauthorizedUser:
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
		case usecase.ErrForbiddenAction:
			return c.JSON(http.StatusForbidden, dto.Fail("forbidden", "only host can join own room"))
		case model.ErrRoomFull:
			return c.JSON(http.StatusConflict, dto.Fail("room_full", "room is full"))
		case usecase.ErrInvalidGameState:
			return c.JSON(http.StatusConflict, dto.Fail("invalid_game_state", "room is not joinable"))
		case usecase.ErrInvalidInput:
			return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "room id is required"))
		default:
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
		}
	}
	if room.CurrentSessionID != nil {
		sid := *room.CurrentSessionID
		middleware.SetAuditGameSessionID(c, &sid)
	}
	r.broadcastRoomState(c.Request().Context(), room.ID, userID, "ROOM_STATE_SYNC")
	return c.JSON(http.StatusOK, dto.OK(dto.CreateRoomData{
		Room: roomDetailJSON(room),
	}))
}

// LeaveRoom は卓からの離脱。
func (r *RoomController) LeaveRoom(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	room, transfer, err := r.room.LeaveRoom(c.Request().Context(), roomID, userID)
	if err != nil {
		switch err {
		case usecase.ErrUnauthorizedUser:
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
		case usecase.ErrInvalidInput:
			return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "room id is required"))
		case usecase.ErrInvalidGameState:
			return c.JSON(http.StatusConflict, dto.Fail("invalid_game_state", "cannot leave during active session"))
		case repository.ErrNotFound:
			return c.JSON(http.StatusNotFound, dto.Fail("not_found", "room or membership not found"))
		default:
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
		}
	}
	if transfer != nil {
		middleware.SetAuditExtra(c, map[string]any{
			"audit_event":           "HOST_TRANSFER",
			"previous_host_user_id": transfer.PreviousHostUserID,
			"new_host_user_id":      transfer.NewHostUserID,
		})
	}
	r.broadcastRoomState(c.Request().Context(), room.ID, userID, "ROOM_STATE_SYNC")
	return c.JSON(http.StatusOK, dto.OK(dto.CreateRoomData{
		Room: roomDetailJSON(room),
	}))
}

// GetRoom は単体ルーム＋セッション概要の取得。
func (r *RoomController) GetRoom(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	room, sess, err := r.room.GetRoom(c.Request().Context(), roomID, userID)
	if err != nil {
		switch err {
		case usecase.ErrUnauthorizedUser:
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
		case usecase.ErrForbiddenAction:
			return c.JSON(http.StatusForbidden, dto.Fail("forbidden", "room access denied"))
		case usecase.ErrInvalidInput:
			return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "room id is required"))
		case repository.ErrNotFound:
			return c.JSON(http.StatusNotFound, dto.Fail("room_not_found", "room not found"))
		default:
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
		}
	}
	data := dto.GetRoomData{
		Room: roomDetailJSON(room),
	}
	if sess != nil {
		setAuditGameSessionID(c, sess)
		s := dto.SessionFromDomain(sess, func(t time.Time) string {
			return t.UTC().Format(time.RFC3339)
		})
		data.Session = &s
	}
	return c.JSON(http.StatusOK, dto.OK(data))
}

// ListRooms は自分がホストのルーム一覧。
func (r *RoomController) ListRooms(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	rooms, err := r.room.ListRooms(c.Request().Context(), userID)
	if err != nil {
		if err == usecase.ErrUnauthorizedUser {
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
		}
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
	items := make([]dto.RoomDetailJSON, 0, len(rooms))
	for _, room := range rooms {
		items = append(items, roomDetailJSON(room))
	}
	return c.JSON(http.StatusOK, dto.OK(dto.ListRoomsData{Rooms: items}))
}

func roomDetailJSON(room *model.Room) dto.RoomDetailJSON {
	return dto.RoomDetailJSON{
		ID:         room.ID,
		HostUserID: room.HostUserID,
		Status:     string(room.Status),
	}
}

func setAuditGameSessionID(c echo.Context, sess *model.GameSession) {
	sid := sess.ID
	middleware.SetAuditGameSessionID(c, &sid)
}
