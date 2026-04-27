package controller

import (
	"testing"
	"time"

	"blackjack/backend/dto"

	"github.com/labstack/echo/v4"
)

func TestLogWSEvent_WithContext(t *testing.T) {
	e := echo.New()
	ws := &WsAuditLogContext{
		Logger:    e.Logger,
		RequestID: "req-1",
		SessionID: "sess-1",
	}
	gid := "g1"
	before, after := int64(1), int64(2)
	logWSEvent(
		ws,
		dto.WSActionRequest{Type: dto.WSEventHit, ActionID: "a1"},
		"r1",
		"u1",
		&gid,
		&before,
		&after,
		time.Now().Add(-10*time.Millisecond),
		"success",
		"",
		map[string]any{"k": "v"},
	)
}
