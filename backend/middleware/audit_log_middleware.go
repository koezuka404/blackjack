package middleware

import (
	"net/http"
	"time"

	"blackjack/backend/auditlog"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const RequestIDContextKey = "request_id"

// ActionIDContextKey は JSON ボディの action_id を監査ログへ載せるためのコンテキストキー（X-Action-Id より優先）。
const ActionIDContextKey = "audit_action_id"

// AuditSessionVersionBeforeKey / AuditSessionVersionAfterKey はハンドラが game session の版を監査に載せるためのキー（*int64、未設定は JSON null）。
const (
	AuditSessionVersionBeforeKey = "audit_session_version_before"
	AuditSessionVersionAfterKey  = "audit_session_version_after"
	// AuditGameSessionIDKey は *string。nil のとき game_session_id は JSON null。
	AuditGameSessionIDKey = "audit_game_session_id"
	// AuditExtraKey は map[string]any。BuildEntry にマージする（HOST_TRANSFER 等）。
	AuditExtraKey = "audit_extra"
)

// SetAuditSessionVersions は HTTP ハンドラが session_version_before / after を監査ログへ反映するために呼ぶ。
func SetAuditSessionVersions(c echo.Context, before, after *int64) {
	c.Set(AuditSessionVersionBeforeKey, before)
	c.Set(AuditSessionVersionAfterKey, after)
}

// SetAuditGameSessionID は監査ログの game_session_id（仕様 20.1）を設定する。未設定は null。
func SetAuditGameSessionID(c echo.Context, id *string) {
	c.Set(AuditGameSessionIDKey, id)
}

// SetAuditExtra は BuildEntry へマージする追加フィールドを設定する（上書き）。
func SetAuditExtra(c echo.Context, extra map[string]any) {
	if len(extra) == 0 {
		return
	}
	c.Set(AuditExtraKey, extra)
}

func auditGameSessionIDFromContext(c echo.Context) any {
	v, ok := c.Get(AuditGameSessionIDKey).(*string)
	if !ok || v == nil {
		return nil
	}
	return *v
}

func auditExtraFromContext(c echo.Context) map[string]any {
	v, ok := c.Get(AuditExtraKey).(map[string]any)
	if !ok || v == nil {
		return nil
	}
	return v
}

func RequestIDMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			reqID := c.Request().Header.Get("X-Request-Id")
			if reqID == "" {
				reqID = uuid.NewString()
			}
			c.Set(RequestIDContextKey, reqID)
			c.Response().Header().Set("X-Request-Id", reqID)
			return next(c)
		}
	}
}

func resolveActionID(c echo.Context) string {
	if v, ok := c.Get(ActionIDContextKey).(string); ok && v != "" {
		return v
	}
	return c.Request().Header.Get("X-Action-Id")
}

func AuditLogMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now().UTC()
			err := next(c)
			latency := time.Since(start).Milliseconds()
			status := c.Response().Status
			if status == 0 {
				status = http.StatusOK
			}
			result := "success"
			errorCode := ""
			if status >= 400 || err != nil {
				result = "failure"
				errorCode = http.StatusText(status)
			}
			reqID, _ := c.Get(RequestIDContextKey).(string)
			userID, _ := c.Get("user_id").(string)
			var verBefore *int64
			if v, ok := c.Get(AuditSessionVersionBeforeKey).(*int64); ok {
				verBefore = v
			}
			var verAfter *int64
			if v, ok := c.Get(AuditSessionVersionAfterKey).(*int64); ok {
				verAfter = v
			}
			entry := auditlog.BuildEntry(
				start,
				reqID,
				resolveActionID(c),
				c.Param("id"),
				c.Get("session_id"),
				auditGameSessionIDFromContext(c),
				userID,
				"USER",
				c.Request().Method+" "+c.Path(),
				verBefore,
				verAfter,
				latency,
				result,
				errorCode,
				auditExtraFromContext(c),
			)
			auditlog.Info(c.Logger(), entry)
			return err
		}
	}
}
