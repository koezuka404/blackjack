package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/jwtauth"
	"blackjack/backend/model"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

func startRoomWSServer(t *testing.T, rc *RoomController) (*httptest.Server, string) {
	t.Helper()
	e := echo.New()
	e.GET("/ws/rooms/:id", rc.RoomWS)
	s := httptest.NewServer(e)
	t.Cleanup(s.Close)
	return s, "ws" + strings.TrimPrefix(s.URL, "http") + "/ws/rooms/r1"
}

func dialWS(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	return c
}

func TestRoomWS_AuthAndEarlyErrors(t *testing.T) {
	globalRoomHub = &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}, latest: map[string]*websocket.Conn{}}
	ConfigureWebSocketConnectionEpochStore(nil, 0)
	ConfigureWebSocketAllowedOrigins(nil)

	t.Run("invalid first message", func(t *testing.T) {
		rc := NewRoomController(roomUsecaseControllerStub{}, nil, nil, []byte("this-is-a-very-long-secret"))
		_, url := startRoomWSServer(t, rc)
		c := dialWS(t, url)
		defer c.Close()
		_ = c.WriteMessage(websocket.TextMessage, []byte(`{"type":"PING"}`))
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = c.ReadMessage()
	})

	t.Run("invalid jwt", func(t *testing.T) {
		rc := NewRoomController(roomUsecaseControllerStub{}, nil, nil, []byte("this-is-a-very-long-secret"))
		_, url := startRoomWSServer(t, rc)
		c := dialWS(t, url)
		defer c.Close()
		msg, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: "bad"})
		_ = c.WriteMessage(websocket.TextMessage, msg)
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = c.ReadMessage()
	})

	t.Run("forbidden room access", func(t *testing.T) {
		secret := []byte("this-is-a-very-long-secret")
		token, _, _, _ := jwtauth.SignAccessToken(secret, "u1", time.Hour)
		rc := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
				return nil, nil, usecase.ErrForbiddenAction
			},
		}, nil, nil, secret)
		_, url := startRoomWSServer(t, rc)
		c := dialWS(t, url)
		defer c.Close()
		msg, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: token})
		_ = c.WriteMessage(websocket.TextMessage, msg)
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = c.ReadMessage()
	})

	t.Run("post-auth limiter denied", func(t *testing.T) {
		secret := []byte("this-is-a-very-long-secret")
		token, _, _, _ := jwtauth.SignAccessToken(secret, "u1", time.Hour)
		rc := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
				return &model.Room{ID: "r1", Status: model.RoomStatusWaiting}, nil, nil
			},
		}, limiterStub{
			allowFn: func(ctx context.Context, key string) (usecase.RateLimitResult, error) {
				if strings.HasPrefix(key, "ws-open-pre:") {
					return usecase.RateLimitResult{Allowed: true}, nil
				}
				return usecase.RateLimitResult{Allowed: false, RetryAfterMS: 100}, nil
			},
		}, nil, secret)
		_, url := startRoomWSServer(t, rc)
		c := dialWS(t, url)
		defer c.Close()
		msg, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: token})
		_ = c.WriteMessage(websocket.TextMessage, msg)
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = c.ReadMessage()
	})
}

func TestRoomWS_SuccessAndMessageLoopBranches(t *testing.T) {
	globalRoomHub = &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}, latest: map[string]*websocket.Conn{}}
	ConfigureWebSocketAllowedOrigins(nil)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ConfigureWebSocketConnectionEpochStore(rdb, time.Minute)
	t.Cleanup(func() {
		ConfigureWebSocketConnectionEpochStore(nil, 0)
		_ = rdb.Close()
	})

	secret := []byte("this-is-a-very-long-secret")
	token, _, _, _ := jwtauth.SignAccessToken(secret, "u1", time.Hour)
	rc := NewRoomController(roomUsecaseControllerStub{
		getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
			return &model.Room{ID: "r1", Status: model.RoomStatusPlaying}, &model.GameSession{ID: "s1", Status: model.SessionStatusPlayerTurn, Version: 1}, nil
		},
		getRoomStateFn: func(context.Context, string, string) (*usecase.RoomState, error) {
			return &usecase.RoomState{
				Room:    &model.Room{ID: "r1", Status: model.RoomStatusPlaying},
				Session: &model.GameSession{ID: "s1", Status: model.SessionStatusPlayerTurn, Version: 1},
			}, nil
		},
		hitFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
			return nil, nil, model.ErrVersionConflict
		},
		markDisconnectedFn: func(context.Context, string, string) error { return repository.ErrNotFound },
	}, limiterStub{
		allowFn: func(context.Context, string) (usecase.RateLimitResult, error) {
			return usecase.RateLimitResult{Allowed: true}, nil
		},
	}, nil, secret)

	_, url := startRoomWSServer(t, rc)
	c := dialWS(t, url)
	defer c.Close()
	auth, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: token})
	_ = c.WriteMessage(websocket.TextMessage, auth)
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	_, _, _ = c.ReadMessage()


	_ = c.WriteMessage(websocket.TextMessage, []byte("{"))
	_, _, _ = c.ReadMessage()


	hit, _ := json.Marshal(dto.WSActionRequest{Type: dto.WSEventHit, ActionID: "a1", ExpectedVersion: 1})
	_ = c.WriteMessage(websocket.TextMessage, hit)
	_, _, _ = c.ReadMessage()


	_ = rdb.Set(context.Background(), wsEpochLatestKey("r1", "u1"), int64(999), time.Minute).Err()
	ping, _ := json.Marshal(dto.WSActionRequest{Type: dto.WSEventPing})
	_ = c.WriteMessage(websocket.TextMessage, ping)
	_, _, _ = c.ReadMessage()
}

func TestRoomWS_MessageLoopRateLimitAndEpochErrors(t *testing.T) {
	globalRoomHub = &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}, latest: map[string]*websocket.Conn{}}
	ConfigureWebSocketAllowedOrigins(nil)
	ConfigureWebSocketConnectionEpochStore(nil, 0)
	defer ConfigureWebSocketConnectionEpochStore(nil, 0)

	secret := []byte("this-is-a-very-long-secret")
	token, _, _, _ := jwtauth.SignAccessToken(secret, "u1", time.Hour)
	call := 0
	rc := NewRoomController(roomUsecaseControllerStub{
		getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
			return &model.Room{ID: "r1", Status: model.RoomStatusPlaying}, nil, nil
		},
		getRoomStateFn: func(context.Context, string, string) (*usecase.RoomState, error) {
			return &usecase.RoomState{Room: &model.Room{ID: "r1", Status: model.RoomStatusPlaying}}, nil
		},
		markDisconnectedFn: func(context.Context, string, string) error { return nil },
	}, limiterStub{
		allowFn: func(ctx context.Context, key string) (usecase.RateLimitResult, error) {
			if strings.HasPrefix(key, "ws-open-pre:") || strings.HasPrefix(key, "ws-open:") {
				return usecase.RateLimitResult{Allowed: true}, nil
			}
			call++
			if call == 1 {
				return usecase.RateLimitResult{}, context.DeadlineExceeded
			}
			if call == 2 {
				return usecase.RateLimitResult{Allowed: false, RetryAfterMS: 20}, nil
			}
			return usecase.RateLimitResult{Allowed: true}, nil
		},
	}, nil, secret)
	_, url := startRoomWSServer(t, rc)
	c := dialWS(t, url)
	defer c.Close()
	auth, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: token})
	_ = c.WriteMessage(websocket.TextMessage, auth)
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	_, _, _ = c.ReadMessage()

	msg, _ := json.Marshal(dto.WSActionRequest{Type: dto.WSEventPing})
	_ = c.WriteMessage(websocket.TextMessage, msg)
	_, _, _ = c.ReadMessage()
	_ = c.WriteMessage(websocket.TextMessage, msg)
	_, _, _ = c.ReadMessage()
}

func TestRoomWS_AdditionalErrorBranches(t *testing.T) {
	globalRoomHub = &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}, latest: map[string]*websocket.Conn{}}
	ConfigureWebSocketAllowedOrigins(nil)

	secret := []byte("this-is-a-very-long-secret")
	token, _, _, _ := jwtauth.SignAccessToken(secret, "u1", time.Hour)

	t.Run("auth read deadline/close path", func(t *testing.T) {
		rc := NewRoomController(roomUsecaseControllerStub{}, nil, nil, secret)
		_, url := startRoomWSServer(t, rc)
		c := dialWS(t, url)
		_ = c.Close()
	})

	t.Run("post auth limiter error", func(t *testing.T) {
		rc := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
				return &model.Room{ID: "r1", Status: model.RoomStatusPlaying}, nil, nil
			},
		}, limiterStub{
			allowFn: func(ctx context.Context, key string) (usecase.RateLimitResult, error) {
				if strings.HasPrefix(key, "ws-open-pre:") {
					return usecase.RateLimitResult{Allowed: true}, nil
				}
				return usecase.RateLimitResult{}, context.DeadlineExceeded
			},
		}, nil, secret)
		_, url := startRoomWSServer(t, rc)
		c := dialWS(t, url)
		defer c.Close()
		auth, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: token})
		_ = c.WriteMessage(websocket.TextMessage, auth)
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = c.ReadMessage()
	})

	t.Run("upgrade error on plain http request", func(t *testing.T) {
		rc := NewRoomController(roomUsecaseControllerStub{}, nil, nil, secret)
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/ws/rooms/r1", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath("/ws/rooms/:id")
		c.SetParamNames("id")
		c.SetParamValues("r1")
		if err := rc.RoomWS(c); err == nil {
			t.Fatal("expected websocket upgrade error")
		}
	})

	t.Run("register epoch error", func(t *testing.T) {
		rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, ReadTimeout: 100 * time.Millisecond})
		defer rdb.Close()
		ConfigureWebSocketConnectionEpochStore(rdb, time.Second)
		defer ConfigureWebSocketConnectionEpochStore(nil, 0)

		rc := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
				return &model.Room{ID: "r1", Status: model.RoomStatusPlaying}, nil, nil
			},
		}, limiterStub{
			allowFn: func(context.Context, string) (usecase.RateLimitResult, error) { return usecase.RateLimitResult{Allowed: true}, nil },
		}, nil, secret)
		_, url := startRoomWSServer(t, rc)
		c := dialWS(t, url)
		defer c.Close()
		auth, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: token})
		_ = c.WriteMessage(websocket.TextMessage, auth)
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = c.ReadMessage()
	})

	t.Run("mark connected internal error", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer rdb.Close()
		ConfigureWebSocketConnectionEpochStore(rdb, time.Minute)
		defer ConfigureWebSocketConnectionEpochStore(nil, 0)

		rc := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
				return &model.Room{ID: "r1", Status: model.RoomStatusPlaying}, nil, nil
			},
			markConnectedFn: func(context.Context, string, string) error { return context.DeadlineExceeded },
		}, limiterStub{
			allowFn: func(context.Context, string) (usecase.RateLimitResult, error) { return usecase.RateLimitResult{Allowed: true}, nil },
		}, nil, secret)
		_, url := startRoomWSServer(t, rc)
		c := dialWS(t, url)
		defer c.Close()
		auth, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: token})
		_ = c.WriteMessage(websocket.TextMessage, auth)
		_ = c.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = c.ReadMessage()
	})

	t.Run("old connection replacement and loop epoch branches", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		defer rdb.Close()
		ConfigureWebSocketConnectionEpochStore(rdb, time.Minute)
		defer ConfigureWebSocketConnectionEpochStore(nil, 0)

		prevSet := wsEpochSetFn
		prevGet := wsEpochGetInt64Fn
		t.Cleanup(func() {
			wsEpochSetFn = prevSet
			wsEpochGetInt64Fn = prevGet
		})

		setCall := 0
		wsEpochSetFn = func(ctx context.Context, rdb *redis.Client, key string, value any, ttl time.Duration) error {
			setCall++

			if setCall >= 2 {
				return context.DeadlineExceeded
			}
			return prevSet(ctx, rdb, key, value, ttl)
		}

		rc := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
				return &model.Room{ID: "r1", Status: model.RoomStatusPlaying}, nil, nil
			},
			getRoomStateFn: func(context.Context, string, string) (*usecase.RoomState, error) {
				return &usecase.RoomState{Room: &model.Room{ID: "r1", Status: model.RoomStatusPlaying}}, nil
			},
			markDisconnectedFn: func(context.Context, string, string) error { return nil },
		}, limiterStub{
			allowFn: func(context.Context, string) (usecase.RateLimitResult, error) { return usecase.RateLimitResult{Allowed: true}, nil },
		}, nil, secret)
		_, url := startRoomWSServer(t, rc)
		c1 := dialWS(t, url)
		defer c1.Close()
		auth, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: token})
		_ = c1.WriteMessage(websocket.TextMessage, auth)
		_ = c1.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = c1.ReadMessage()


		wsEpochSetFn = prevSet
		c2 := dialWS(t, url)
		defer c2.Close()
		_ = c2.WriteMessage(websocket.TextMessage, auth)
		_ = c2.SetReadDeadline(time.Now().Add(time.Second))
		_, _, _ = c2.ReadMessage()


		wsEpochSetFn = func(context.Context, *redis.Client, string, any, time.Duration) error { return context.DeadlineExceeded }
		ping, _ := json.Marshal(dto.WSActionRequest{Type: dto.WSEventPing})
		_ = c2.WriteMessage(websocket.TextMessage, ping)
		_, _, _ = c2.ReadMessage()


		wsEpochSetFn = prevSet
		wsEpochGetInt64Fn = func(context.Context, *redis.Client, string) (int64, error) { return 0, context.DeadlineExceeded }
		_ = c2.WriteMessage(websocket.TextMessage, ping)
		_, _, _ = c2.ReadMessage()


		wsEpochGetInt64Fn = func(context.Context, *redis.Client, string) (int64, error) { return 999, nil }
		_ = c2.WriteMessage(websocket.TextMessage, ping)
		_, _, _ = c2.ReadMessage()
	})
}



func TestRoomWS_DeferEpochRedisErrorTreatsAsCurrent(t *testing.T) {
	globalRoomHub = &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}, latest: map[string]*websocket.Conn{}}
	ConfigureWebSocketAllowedOrigins(nil)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ConfigureWebSocketConnectionEpochStore(rdb, time.Minute)
	t.Cleanup(func() {
		ConfigureWebSocketConnectionEpochStore(nil, 0)
		_ = rdb.Close()
	})

	prevGet := wsEpochGetInt64Fn
	t.Cleanup(func() { wsEpochGetInt64Fn = prevGet })

	secret := []byte("this-is-a-very-long-secret")
	token, _, _, _ := jwtauth.SignAccessToken(secret, "u1", time.Hour)

	var marked int
	rc := NewRoomController(roomUsecaseControllerStub{
		getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
			return &model.Room{ID: "r1", Status: model.RoomStatusPlaying}, nil, nil
		},
		getRoomStateFn: func(context.Context, string, string) (*usecase.RoomState, error) {
			return &usecase.RoomState{Room: &model.Room{ID: "r1", Status: model.RoomStatusPlaying}}, nil
		},
		markDisconnectedFn: func(context.Context, string, string) error {
			marked++
			return nil
		},
	}, limiterStub{
		allowFn: func(context.Context, string) (usecase.RateLimitResult, error) {
			return usecase.RateLimitResult{Allowed: true}, nil
		},
	}, nil, secret)

	_, url := startRoomWSServer(t, rc)
	c := dialWS(t, url)
	auth, _ := json.Marshal(dto.WSAuthMessage{Type: dto.WSEventAuth, AccessToken: token})
	_ = c.WriteMessage(websocket.TextMessage, auth)
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	_, _, _ = c.ReadMessage()

	wsEpochGetInt64Fn = func(context.Context, *redis.Client, string) (int64, error) {
		return 0, errors.New("epoch get transient")
	}
	_ = c.Close()
	time.Sleep(200 * time.Millisecond)
	if marked != 1 {
		t.Fatalf("expected MarkDisconnected once when epoch check errors in defer, got %d", marked)
	}
}

