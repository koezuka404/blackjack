package controller

import (
	"time"

	"blackjack/backend/auditlog"
	"blackjack/backend/dto"
	"blackjack/backend/middleware"
	"blackjack/backend/observability"

	"github.com/labstack/echo/v4"
)

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
