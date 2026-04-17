package controller

import (
	"net/http"
	"os"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/middleware"
	"blackjack/backend/model"
	"blackjack/backend/observability"
	"blackjack/backend/realtime"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)

type RoomController struct {
	room       usecase.RoomUsecase
	limiter    middleware.RateLimiter
	syncBroker *realtime.RoomSyncBroker
}

// NewRoomController はルーム API / WS 用コントローラを生成する。
func NewRoomController(room usecase.RoomUsecase, limiter middleware.RateLimiter, syncBroker *realtime.RoomSyncBroker) *RoomController {
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

// GetPlayHint は中級者向けヒューリスティックによる HIT/STAND 推奨（プレイヤーターン・操作可能時のみ）。
func (r *RoomController) GetPlayHint(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	hint, err := r.room.SuggestPlayerAction(c.Request().Context(), roomID, userID)
	if err != nil {
		switch err {
		case usecase.ErrUnauthorizedUser:
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
		case usecase.ErrForbiddenAction:
			return c.JSON(http.StatusForbidden, dto.Fail("forbidden", "room access denied"))
		case usecase.ErrInvalidInput:
			return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "room id is required"))
		case usecase.ErrInvalidGameState:
			return c.JSON(http.StatusConflict, dto.Fail("invalid_game_state", "hint is only available on your turn when you can hit"))
		case repository.ErrNotFound:
			return c.JSON(http.StatusNotFound, dto.Fail("not_found", "room or session not found"))
		default:
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
		}
	}
	return c.JSON(http.StatusOK, dto.OK(dto.PlayHintData{
		Recommendation: hint.Recommendation,
		SessionVersion: hint.SessionVersion,
		Rationale:      hint.Rationale,
	}))
}

// GetRoomHistory は round_logs 由来の履歴取得。
func (r *RoomController) GetRoomHistory(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	history, err := r.room.GetRoomHistory(c.Request().Context(), roomID, userID)
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
	items := make([]dto.RoomHistoryItemJSON, 0, len(history))
	for _, h := range history {
		items = append(items, dto.RoomHistoryItemFromDomain(h.SessionID, h.RoundNo, h.ResultPayload, h.CreatedAt))
	}
	return c.JSON(http.StatusOK, dto.OK(dto.RoomHistoryData{
		RoomID: roomID,
		Items:  items,
	}))
}

// StartRoom はゲーム開始（配札）。
func (r *RoomController) StartRoom(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	room, sess, err := r.room.StartRoom(c.Request().Context(), roomID, userID)
	if err != nil {
		switch err {
		case usecase.ErrUnauthorizedUser:
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
		case usecase.ErrForbiddenAction:
			return c.JSON(http.StatusForbidden, dto.Fail("forbidden", "only host can start room"))
		case usecase.ErrInvalidInput:
			return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "room id is required"))
		case usecase.ErrInvalidGameState:
			return c.JSON(http.StatusConflict, dto.Fail("invalid_game_state", "room is not startable"))
		case repository.ErrNotFound:
			return c.JSON(http.StatusNotFound, dto.Fail("room_not_found", "room not found"))
		default:
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
		}
	}
	middleware.SetAuditSessionVersions(c, nil, &sess.Version)
	setAuditGameSessionID(c, sess)
	r.broadcastRoomState(c.Request().Context(), room.ID, userID, "ROOM_STATE_SYNC")
	return c.JSON(http.StatusOK, dto.OK(dto.StartRoomData{
		Room: roomDetailJSON(room),
		Session: dto.SessionFromDomain(sess, func(t time.Time) string {
			return t.UTC().Format(time.RFC3339)
		}),
	}))
}

// Hit は HTTP 経由のヒット（WS と二重になる場合はクライアント方針次第）。
func (r *RoomController) Hit(c echo.Context) error {
	return r.turnAction(c, true)
}

// Stand は HTTP 経由のスタンド。
func (r *RoomController) Stand(c echo.Context) error {
	return r.turnAction(c, false)
}

// ResetRoomDebug は開発専用の卓リセット（§15.3）。BLACKJACK_DEBUG_ROOM_RESET=true のときのみ有効。
func (r *RoomController) ResetRoomDebug(c echo.Context) error {
	if os.Getenv("BLACKJACK_DEBUG_ROOM_RESET") != "true" {
		return c.JSON(http.StatusForbidden, dto.Fail("debug_disabled", "room reset is disabled"))
	}
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	room, err := r.room.ResetRoomForDebug(c.Request().Context(), roomID, userID)
	if err != nil {
		switch err {
		case usecase.ErrUnauthorizedUser:
			return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
		case usecase.ErrForbiddenAction:
			return c.JSON(http.StatusForbidden, dto.Fail("forbidden", "only host can reset room"))
		case usecase.ErrInvalidInput:
			return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "room id is required"))
		case repository.ErrNotFound:
			return c.JSON(http.StatusNotFound, dto.Fail("room_not_found", "room not found"))
		default:
			return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
		}
	}
	r.broadcastRoomState(c.Request().Context(), roomID, userID, "ROOM_STATE_SYNC")
	return c.JSON(http.StatusOK, dto.OK(dto.CreateRoomData{
		Room: roomDetailJSON(room),
	}))
}

// RematchVote は再戦投票処理本体。仕様 §12.2 により HTTP ルートには公開せず、WS REMATCH_VOTE から利用する。
func (r *RoomController) RematchVote(c echo.Context) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	var req dto.RematchVoteRequest
	if err := c.Bind(&req); err != nil || req.ActionID == "" || req.ExpectedVersion <= 0 {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "agree, action_id, expected_version are required"))
	}
	c.Set(middleware.ActionIDContextKey, req.ActionID)
	room, sess, err := r.room.VoteRematch(c.Request().Context(), roomID, userID, req.Agree, req.ExpectedVersion, req.ActionID)
	if err != nil {
		middleware.SetAuditSessionVersions(c, &req.ExpectedVersion, &req.ExpectedVersion)
		if resp := writeTurnMutationError(c, err, "invalid rematch vote payload", "rematch voting is unavailable"); resp != nil {
			return resp
		}
	}
	middleware.SetAuditSessionVersions(c, &req.ExpectedVersion, &sess.Version)
	setAuditGameSessionID(c, sess)
	r.broadcastRoomState(c.Request().Context(), room.ID, userID, "ROOM_STATE_SYNC")
	return c.JSON(http.StatusOK, dto.OK(dto.TurnActionData{
		Room: roomDetailJSON(room),
		Session: dto.SessionFromDomain(sess, func(t time.Time) string {
			return t.UTC().Format(time.RFC3339)
		}),
	}))
}

// turnAction は Hit/Stand 共通の HTTP 処理とブロードキャスト。
func (r *RoomController) turnAction(c echo.Context, hit bool) error {
	userID, _ := c.Get("user_id").(string)
	roomID := c.Param("id")
	var req dto.TurnActionRequest
	if err := c.Bind(&req); err != nil || req.ExpectedVersion <= 0 {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "expected_version is required"))
	}
	if req.ActionID == "" {
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", "action_id is required"))
	}
	c.Set(middleware.ActionIDContextKey, req.ActionID)
	var (
		room *model.Room
		sess *model.GameSession
		err  error
	)
	if hit {
		room, sess, err = r.room.Hit(c.Request().Context(), roomID, userID, req.ExpectedVersion, req.ActionID)
	} else {
		room, sess, err = r.room.Stand(c.Request().Context(), roomID, userID, req.ExpectedVersion, req.ActionID)
	}
	if err != nil {
		middleware.SetAuditSessionVersions(c, &req.ExpectedVersion, &req.ExpectedVersion)
		if resp := writeTurnMutationError(c, err, "room id and expected_version are required", ""); resp != nil {
			return resp
		}
	}
	middleware.SetAuditSessionVersions(c, &req.ExpectedVersion, &sess.Version)
	setAuditGameSessionID(c, sess)
	r.broadcastRoomState(c.Request().Context(), room.ID, userID, "ROOM_STATE_SYNC")
	return c.JSON(http.StatusOK, dto.OK(dto.TurnActionData{
		Room: roomDetailJSON(room),
		Session: dto.SessionFromDomain(sess, func(t time.Time) string {
			return t.UTC().Format(time.RFC3339)
		}),
	}))
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

func writeTurnMutationError(c echo.Context, err error, invalidInputMessage, invalidStateDefault string) error {
	switch err {
	case usecase.ErrUnauthorizedUser:
		return c.JSON(http.StatusUnauthorized, dto.Fail("unauthorized", "login required"))
	case usecase.ErrForbiddenAction:
		return c.JSON(http.StatusForbidden, dto.Fail("forbidden", "room access denied"))
	case usecase.ErrInvalidInput:
		return c.JSON(http.StatusBadRequest, dto.Fail("invalid_input", invalidInputMessage))
	case usecase.ErrInvalidGameState, model.ErrNotPlayerTurn, model.ErrNotYourTurn, model.ErrInvalidPlayerStatus:
		msg := invalidStateDefault
		if msg == "" {
			msg = err.Error()
		}
		return c.JSON(http.StatusConflict, dto.Fail("invalid_game_state", msg))
	case model.ErrVersionConflict:
		observability.IncVersionConflict()
		return c.JSON(http.StatusConflict, dto.Fail("version_conflict", "session version conflict"))
	case model.ErrDuplicateAction:
		observability.IncDuplicateAction()
		return c.JSON(http.StatusConflict, dto.Fail("duplicate_action", "action id already used with different payload"))
	case repository.ErrNotFound:
		return c.JSON(http.StatusNotFound, dto.Fail("not_found", "room or session not found"))
	default:
		return c.JSON(http.StatusInternalServerError, dto.Fail("internal_error", err.Error()))
	}
}
