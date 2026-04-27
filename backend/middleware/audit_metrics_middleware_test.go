package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

func TestAuditHelpersAndMiddleware(t *testing.T) {
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/rooms/r1", nil), httptest.NewRecorder())
	c.SetPath("/rooms/:id")
	c.SetParamNames("id")
	c.SetParamValues("r1")

	before, after := int64(1), int64(2)
	SetAuditSessionVersions(c, &before, &after)
	sid := "gs1"
	SetAuditGameSessionID(c, &sid)
	SetAuditExtra(c, map[string]any{"k": "v"})
	SetAuditExtra(c, nil)

	if auditGameSessionIDFromContext(c) != "gs1" {
		t.Fatal("game session id should be resolved")
	}
	if auditExtraFromContext(c)["k"] != "v" {
		t.Fatal("extra should be resolved")
	}

	c2 := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	if auditGameSessionIDFromContext(c2) != nil || auditExtraFromContext(c2) != nil {
		t.Fatal("missing context values should resolve nil")
	}
	c2.Set(AuditGameSessionIDKey, "wrong")
	c2.Set(AuditExtraKey, "wrong")
	if auditGameSessionIDFromContext(c2) != nil || auditExtraFromContext(c2) != nil {
		t.Fatal("wrong context types should resolve nil")
	}

	reqH := httptest.NewRequest(http.MethodGet, "/", nil)
	reqH.Header.Set("X-Action-Id", "hdr")
	c3 := e.NewContext(reqH, httptest.NewRecorder())
	if resolveActionID(c3) != "hdr" {
		t.Fatal("header action id should be used")
	}
	c3.Set(ActionIDContextKey, "ctx")
	if resolveActionID(c3) != "ctx" {
		t.Fatal("context action id should be prioritized")
	}

	auditOK := AuditLogMiddleware()(func(c echo.Context) error { return nil })
	if err := auditOK(c); err != nil {
		t.Fatalf("audit middleware success failed: %v", err)
	}

	cFail := e.NewContext(httptest.NewRequest(http.MethodGet, "/rooms/r1", nil), httptest.NewRecorder())
	cFail.SetPath("/rooms/:id")
	cFail.SetParamNames("id")
	cFail.SetParamValues("r1")
	cFail.Response().Status = http.StatusBadRequest
	auditFail := AuditLogMiddleware()(func(c echo.Context) error { return echo.NewHTTPError(http.StatusBadRequest, "bad") })
	_ = auditFail(cFail)
}

func TestRequestIDAndTelemetry(t *testing.T) {
	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := RequestIDMiddleware()(func(c echo.Context) error { return nil })(c); err != nil {
		t.Fatalf("request id middleware failed: %v", err)
	}
	if c.Get(RequestIDContextKey) == nil || rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("request id should be generated")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req2.Header.Set("X-Request-Id", "rid")
	c2 := e.NewContext(req2, httptest.NewRecorder())
	_ = RequestIDMiddleware()(func(c echo.Context) error { return nil })(c2)

	tm := HTTPTelemetryMiddleware()(func(c echo.Context) error { return nil })
	c3 := e.NewContext(httptest.NewRequest(http.MethodGet, "/x", nil), httptest.NewRecorder())
	c3.SetPath("/x")
	if err := tm(c3); err != nil {
		t.Fatalf("telemetry middleware failed: %v", err)
	}
}

func TestSetAuthContextFromToken(t *testing.T) {
	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	setAuthContextFromToken(c)

	c.Set("user", "bad")
	setAuthContextFromToken(c)

	c.Set("user", &jwt.Token{Claims: jwt.MapClaims{}})
	setAuthContextFromToken(c)

	c.Set("user", &jwt.Token{Claims: &jwt.RegisteredClaims{Subject: ""}})
	setAuthContextFromToken(c)

	c.Set("user", &jwt.Token{Claims: &jwt.RegisteredClaims{Subject: "u1"}})
	setAuthContextFromToken(c)
	if got, _ := c.Get("user_id").(string); got != "u1" {
		t.Fatalf("unexpected user id: %s", got)
	}

	c2 := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c2.Set("user", &jwt.Token{Claims: &jwt.RegisteredClaims{Subject: "u1", ID: "s1"}})
	setAuthContextFromToken(c2)
	if got, _ := c2.Get("session_id").(string); got != "s1" {
		t.Fatalf("unexpected session id: %s", got)
	}
}
