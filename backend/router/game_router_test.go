package router

import (
	"context"
	"testing"

	"blackjack/backend/model"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)

type authUsecaseStub struct{}

func (authUsecaseStub) Signup(context.Context, string, string, string) (usecase.AuthResponse, error) {
	return nil, nil
}
func (authUsecaseStub) Login(context.Context, string, string) (usecase.AuthResponse, error) { return nil, nil }
func (authUsecaseStub) Logout(context.Context) error                                          { return nil }
func (authUsecaseStub) Me(context.Context, string) (*model.User, error)                       { return nil, nil }

type roomUsecaseStub struct{}

func (roomUsecaseStub) CreateRoom(context.Context, string) (*model.Room, error) { return nil, nil }
func (roomUsecaseStub) JoinRoom(context.Context, string, string) (*model.Room, error) {
	return nil, nil
}
func (roomUsecaseStub) GetRoom(context.Context, string, string) (*model.Room, *model.GameSession, error) {
	return nil, nil, nil
}
func (roomUsecaseStub) GetRoomState(context.Context, string, string) (*usecase.RoomState, error) {
	return nil, nil
}
func (roomUsecaseStub) ListRooms(context.Context, string) ([]*model.Room, error) { return nil, nil }
func (roomUsecaseStub) GetRoomHistory(context.Context, string, string) ([]*model.RoundLog, error) {
	return nil, nil
}
func (roomUsecaseStub) LeaveRoom(context.Context, string, string) (*model.Room, *usecase.HostTransfer, error) {
	return nil, nil, nil
}
func (roomUsecaseStub) StartRoom(context.Context, string, string) (*model.Room, *model.GameSession, error) {
	return nil, nil, nil
}
func (roomUsecaseStub) Hit(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
	return nil, nil, nil
}
func (roomUsecaseStub) Stand(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
	return nil, nil, nil
}
func (roomUsecaseStub) VoteRematch(context.Context, string, string, bool, int64, string) (*model.Room, *model.GameSession, error) {
	return nil, nil, nil
}
func (roomUsecaseStub) MarkConnected(context.Context, string, string) error       { return nil }
func (roomUsecaseStub) MarkDisconnected(context.Context, string, string) error    { return nil }
func (roomUsecaseStub) AutoStandDueSessions(context.Context) ([]string, error)    { return nil, nil }
func (roomUsecaseStub) SuggestPlayerAction(context.Context, string, string) (*usecase.PlayHint, error) {
	return nil, nil
}
func (roomUsecaseStub) ResetRoomForDebug(context.Context, string, string) (*model.Room, error) {
	return nil, nil
}

type rateLimitUsecaseStub struct{}

func (rateLimitUsecaseStub) Allow(context.Context, string) (usecase.RateLimitResult, error) {
	return usecase.RateLimitResult{Allowed: true}, nil
}
func (rateLimitUsecaseStub) AllowSignup(context.Context, string, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{Allowed: true}, nil
}
func (rateLimitUsecaseStub) AllowLogin(context.Context, string, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{Allowed: true}, nil
}
func (rateLimitUsecaseStub) AllowTasks(context.Context, uint) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{Allowed: true}, nil
}

func TestRegister_AddsExpectedHTTPAndWSRoutes(t *testing.T) {
	e := echo.New()
	controller := Register(
		e,
		nil,
		rateLimitUsecaseStub{},
		authUsecaseStub{},
		roomUsecaseStub{},
		nil,
		[]byte("this-is-a-very-long-secret"),
	)
	if controller == nil {
		t.Fatal("expected room controller")
	}

	want := map[string]bool{
		"POST /api/auth/signup":      false,
		"POST /api/auth/login":       false,
		"POST /api/auth/logout":      false,
		"GET /api/me":                false,
		"POST /api/rooms":            false,
		"GET /api/rooms":             false,
		"POST /api/rooms/:id/join":   false,
		"POST /api/rooms/:id/leave":  false,
		"GET /api/rooms/:id":         false,
		"GET /api/rooms/:id/history": false,
		"GET /api/rooms/:id/play_hint": false,
		"POST /api/rooms/:id/start":  false,
		"POST /api/rooms/:id/hit":    false,
		"POST /api/rooms/:id/stand":  false,
		"POST /api/rooms/:id/reset":  false,
		"GET /ws/rooms/:id":          false,
	}

	for _, r := range e.Routes() {
		key := r.Method + " " + r.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Fatalf("expected route not registered: %s", key)
		}
	}
}

