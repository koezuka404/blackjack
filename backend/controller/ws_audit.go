package controller

import (
	"time"

	"blackjack/backend/auditlog"
	"blackjack/backend/dto"
	"blackjack/backend/observability"

	"github.com/labstack/echo/v4"
)

// WsAuditLogContext は長寿命 WS ゴルーチン用（Echo の Request 終了後も監査・メトリクスに使う）。
type WsAuditLogContext struct {
	Logger    echo.Logger
	RequestID string
	SessionID any // JWT の jti（監査の session_id 相当）
}

// logWSEvent は WS メッセージごとの構造化監査ログを出す（HTTP AuditLogMiddleware と同一スキーマ）。
func logWSEvent(ws *WsAuditLogContext, req dto.WSActionRequest, roomID, userID string, gameSessionID *string, before, after *int64, start time.Time, result, errorCode string, extra map[string]any) {
	if ws == nil {
		return
	}
	reqID := req.RequestID
	if reqID == "" {
		reqID = ws.RequestID
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
		ws.SessionID,
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
	auditlog.Info(ws.Logger, entry)
	observability.ObserveWSMessage(req.Type, result, time.Since(start).Seconds())
}
