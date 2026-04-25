package auditlog

import (
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestBuildEntryAndInfo(t *testing.T) {
	before := int64(1)
	after := int64(2)
	entry := BuildEntry(
		time.Unix(0, 0).UTC(),
		"req-1",
		"act-1",
		"room-1",
		"sess-1",
		"gs-1",
		"user-1",
		"USER",
		"HIT",
		&before,
		&after,
		123,
		"success",
		"",
		map[string]any{"audit_event": "TEST"},
	)
	if entry["request_id"] != "req-1" {
		t.Fatalf("unexpected request_id: %v", entry["request_id"])
	}
	if entry["audit_event"] != "TEST" {
		t.Fatalf("extra field missing: %#v", entry)
	}

	// Smoke test: no panic and marshal succeeds.
	e := echo.New()
	Info(e.Logger, entry)
	Info(e.Logger, map[string]any{
		"bad": func() {},
	})
}

