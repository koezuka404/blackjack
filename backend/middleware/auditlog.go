package middleware

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

const RequestIDContextKey = "request_id"

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
			actionID := c.Request().Header.Get("X-Action-Id")
			userID, _ := c.Get("user_id").(string)
			entry := map[string]any{
				"timestamp":              start.Format(time.RFC3339Nano),
				"level":                  "INFO",
				"request_id":             reqID,
				"action_id":              actionID,
				"room_id":                c.Param("id"),
				"session_id":             c.Get("session_id"),
				"user_id":                userID,
				"actor_type":             "USER",
				"request_type":           c.Request().Method + " " + c.Path(),
				"session_version_before": nil,
				"session_version_after":  nil,
				"latency_ms":             latency,
				"result":                 result,
				"error_code":             errorCode,
			}
			if b, marshalErr := json.Marshal(entry); marshalErr == nil {
				c.Logger().Info(string(b))
			}
			return err
		}
	}
}
