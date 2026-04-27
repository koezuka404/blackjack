package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"blackjack/backend/realtime"
	"blackjack/backend/model"
	"blackjack/backend/usecase"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

type limiterStub struct {
	allowFn func(context.Context, string) (usecase.RateLimitResult, error)
}

func (l limiterStub) Allow(ctx context.Context, key string) (usecase.RateLimitResult, error) {
	if l.allowFn != nil {
		return l.allowFn(ctx, key)
	}
	return usecase.RateLimitResult{Allowed: true}, nil
}
func (limiterStub) AllowSignup(context.Context, string, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{}, nil
}
func (limiterStub) AllowLogin(context.Context, string, string) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{}, nil
}
func (limiterStub) AllowTasks(context.Context, uint) (usecase.RateLimitDecision, error) {
	return usecase.RateLimitDecision{}, nil
}

func TestRoomWS_EarlyBranches(t *testing.T) {
	e := echo.New()
	rc := NewRoomController(roomUsecaseControllerStub{}, nil, nil, []byte("this-is-a-very-long-secret"))

	req := httptest.NewRequest(http.MethodGet, "/ws/rooms/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/ws/rooms/:id")
	c.SetParamNames("id")
	c.SetParamValues("")
	_ = rc.RoomWS(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected room ws bad request status: %d", rec.Code)
	}

	rcErr := NewRoomController(roomUsecaseControllerStub{}, limiterStub{
		allowFn: func(context.Context, string) (usecase.RateLimitResult, error) {
			return usecase.RateLimitResult{}, context.DeadlineExceeded
		},
	}, nil, []byte("this-is-a-very-long-secret"))
	req2 := httptest.NewRequest(http.MethodGet, "/ws/rooms/r1", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	c2.SetPath("/ws/rooms/:id")
	c2.SetParamNames("id")
	c2.SetParamValues("r1")
	_ = rcErr.RoomWS(c2)
	if rec2.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected room ws error status: %d", rec2.Code)
	}

	rcDeny := NewRoomController(roomUsecaseControllerStub{}, limiterStub{
		allowFn: func(context.Context, string) (usecase.RateLimitResult, error) {
			return usecase.RateLimitResult{Allowed: false, RetryAfterMS: 10}, nil
		},
	}, nil, []byte("this-is-a-very-long-secret"))
	req3 := httptest.NewRequest(http.MethodGet, "/ws/rooms/r1", nil)
	rec3 := httptest.NewRecorder()
	c3 := e.NewContext(req3, rec3)
	c3.SetPath("/ws/rooms/:id")
	c3.SetParamNames("id")
	c3.SetParamValues("r1")
	_ = rcDeny.RoomWS(c3)
	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("unexpected room ws denied status: %d", rec3.Code)
	}
}

func TestBroadcastHelpers(t *testing.T) {
	now := time.Now().UTC()
	rc := NewRoomController(roomUsecaseControllerStub{
		getRoomStateFn: func(context.Context, string, string) (*usecase.RoomState, error) {
			return &usecase.RoomState{
				Room:    &model.Room{ID: "r1", Status: model.RoomStatusPlaying},
				Session: &model.GameSession{ID: "s1", Status: model.SessionStatusPlayerTurn, Version: 1, UpdatedAt: now},
			}, nil
		},
	}, nil, nil, nil)
	globalRoomHub = &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}, latest: map[string]*websocket.Conn{}}
	rc.BroadcastRoomStateFromPeer(context.Background(), "r1", "ROOM_STATE_SYNC")
	rc.BroadcastRoomSync(context.Background(), "r1")
	rc.broadcastRoomState(context.Background(), "r1", "u1", "ROOM_STATE_SYNC")

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	broker := realtime.NewRoomSyncBroker(rdb, "srv-1")
	rc2 := NewRoomController(roomUsecaseControllerStub{
		getRoomStateFn: func(context.Context, string, string) (*usecase.RoomState, error) {
			return &usecase.RoomState{Room: &model.Room{ID: "r1", Status: model.RoomStatusPlaying}}, nil
		},
	}, nil, broker, nil)
	rc2.broadcastRoomState(context.Background(), "r1", "u1", "ROOM_STATE_SYNC")

	globalRoomHub = &roomHub{
		rooms:  map[string]map[*websocket.Conn]wsConnMeta{"r1": {&websocket.Conn{}: {userID: "u1", writeMu: &sync.Mutex{}}}},
		latest: map[string]*websocket.Conn{},
	}
	rcErr := NewRoomController(roomUsecaseControllerStub{
		getRoomStateFn: func(context.Context, string, string) (*usecase.RoomState, error) {
			return nil, errors.New("boom")
		},
	}, nil, nil, nil)
	rcErr.broadcastRoomStateLocal(context.Background(), "r1", "u1", "ROOM_STATE_SYNC")
}

func TestAllowAllWSOrigins(t *testing.T) {
	if !allowAllWSOrigins(&http.Request{}) {
		t.Fatal("expected allowAllWSOrigins true")
	}
}
