package controller

import (
	"net/http"
	"os"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/middleware"
	"blackjack/backend/model"
	"blackjack/backend/observability"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)


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


func (r *RoomController) Hit(c echo.Context) error {
	return r.turnAction(c, true)
}


func (r *RoomController) Stand(c echo.Context) error {
	return r.turnAction(c, false)
}


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
		return writeTurnMutationError(c, err, "invalid rematch vote payload", "rematch voting is unavailable")
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
		return writeTurnMutationError(c, err, "room id and expected_version are required", "")
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
