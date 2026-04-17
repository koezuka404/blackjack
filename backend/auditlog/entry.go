package auditlog

import (
	"encoding/json"
	"time"

	"github.com/labstack/echo/v4"
)

// BuildEntry は仕様 20 章に沿った構造化監査ログ 1 行分（HTTP / WebSocket 共通スキーマ）。
// session_id はログインセッション（Cookie）、game_session_id はゲームセッション UUID（仕様 20.1）。
// extra は domain 固有のキー（audit_event, connection_epoch 等）をマージする。
func BuildEntry(
	ts time.Time,
	reqID, actionID, roomID string,
	sessionID, gameSessionID any,
	userID, actorType, requestType string,
	sessionVersionBefore, sessionVersionAfter *int64,
	latencyMs int64,
	result, errorCode string,
	extra map[string]any,
) map[string]any {
	e := map[string]any{
		"timestamp":              ts.UTC().Format(time.RFC3339Nano),
		"level":                  "INFO",
		"request_id":             reqID,
		"action_id":              actionID,
		"room_id":                roomID,
		"session_id":             sessionID,
		"game_session_id":        gameSessionID,
		"user_id":                userID,
		"actor_type":             actorType,
		"request_type":           requestType,
		"session_version_before": sessionVersionBefore,
		"session_version_after":  sessionVersionAfter,
		"latency_ms":             latencyMs,
		"result":                 result,
		"error_code":             errorCode,
	}
	for k, v := range extra {
		e[k] = v
	}
	return e
}

// Info は BuildEntry の JSON を 1 行で出力する。
func Info(logger echo.Logger, entry map[string]any) {
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	logger.Info(string(b))
}
