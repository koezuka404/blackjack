package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/model"
	"blackjack/backend/usecase"

	"github.com/alicebob/miniredis/v2"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

func TestWSConfigAndEpochHelpers(t *testing.T) {
	prevIncr := wsEpochIncrFn
	prevSet := wsEpochSetFn
	prevGet := wsEpochGetInt64Fn
	t.Cleanup(func() {
		wsEpochIncrFn = prevIncr
		wsEpochSetFn = prevSet
		wsEpochGetInt64Fn = prevGet
	})

	ConfigureWebSocketAllowedOrigins(nil)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if !wsUpgrader.CheckOrigin(req) {
		t.Fatal("origin should be allowed when no configured origins")
	}

	wsUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	if !wsUpgrader.CheckOrigin(req) {
		t.Fatal("default check origin should allow")
	}

	ConfigureWebSocketAllowedOrigins([]string{" https://a.example ", ""})
	req.Header.Set("Origin", "https://a.example")
	if !wsUpgrader.CheckOrigin(req) {
		t.Fatal("configured origin should be allowed")
	}
	req.Header.Set("Origin", "https://b.example")
	if wsUpgrader.CheckOrigin(req) {
		t.Fatal("unknown origin should be rejected")
	}

	t.Setenv("BLACKJACK_WS_MARK_DISCONNECTED", "")
	if !wsShouldMarkDisconnected() {
		t.Fatal("default should be true")
	}
	t.Setenv("BLACKJACK_WS_MARK_DISCONNECTED", "false")
	if wsShouldMarkDisconnected() {
		t.Fatal("env false should disable")
	}
	t.Setenv("BLACKJACK_WS_MARK_DISCONNECTED", "bad")
	if !wsShouldMarkDisconnected() {
		t.Fatal("invalid value should fallback true")
	}

	t.Setenv("WS_AUTH_DEADLINE", "")
	if wsAuthReadDeadline() != 15*time.Second {
		t.Fatal("default auth deadline mismatch")
	}
	t.Setenv("WS_AUTH_DEADLINE", "20s")
	if wsAuthReadDeadline() != 20*time.Second {
		t.Fatal("configured auth deadline mismatch")
	}
	t.Setenv("WS_AUTH_DEADLINE", "bad")
	if wsAuthReadDeadline() != 15*time.Second {
		t.Fatal("invalid auth deadline should fallback")
	}

	e := echo.New()
	c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.Request().RemoteAddr = "127.0.0.1:1234"
	if got := preWSConnectionKey(c); !strings.HasPrefix(got, "ws-open-pre:") {
		t.Fatalf("unexpected pre ws key: %s", got)
	}
	eFallback := echo.New()
	eFallback.IPExtractor = func(*http.Request) string { return "" }
	cFallback := eFallback.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	cFallback.Request().RemoteAddr = "198.51.100.9:8080"
	if got := preWSConnectionKey(cFallback); !strings.Contains(got, "198.51.100.9:8080") {
		t.Fatalf("expected remote addr fallback, got: %s", got)
	}
	cReal := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	e.IPExtractor = echo.ExtractIPFromRealIPHeader()
	cReal.Request().Header.Set(echo.HeaderXRealIP, "203.0.113.1")
	if got := preWSConnectionKey(cReal); !strings.HasPrefix(got, "ws-open-pre:") || got == "ws-open-pre:"+cReal.Request().RemoteAddr {
		t.Fatalf("expected real ip based key, got: %s", got)
	}

	ConfigureWebSocketConnectionEpochStore(nil, 0)
	if epoch, err := registerConnectionEpoch(context.Background(), "r1", "u1"); err != nil || epoch != 0 {
		t.Fatalf("register with nil redis: epoch=%d err=%v", epoch, err)
	}
	if err := refreshConnectionEpoch(context.Background(), "r1", "u1", 0); err != nil {
		t.Fatalf("refresh with nil redis should pass: %v", err)
	}
	ok, err := isCurrentConnectionEpoch(context.Background(), "r1", "u1", 0)
	if err != nil || !ok {
		t.Fatalf("current epoch with nil redis should be true, ok=%v err=%v", ok, err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, ReadTimeout: 100 * time.Millisecond})
	ConfigureWebSocketConnectionEpochStore(rdb, 2*time.Second)
	_, _ = registerConnectionEpoch(context.Background(), "r1", "u1")
	_ = refreshConnectionEpoch(context.Background(), "r1", "u1", 1)
	_, _ = isCurrentConnectionEpoch(context.Background(), "r1", "u1", 1)
	_ = rdb.Close()

	mr := miniredis.RunT(t)
	rdb2 := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	ConfigureWebSocketConnectionEpochStore(rdb2, time.Minute)
	epoch, err := registerConnectionEpoch(context.Background(), "r1", "u1")
	if err != nil || epoch <= 0 {
		t.Fatalf("register epoch should succeed: epoch=%d err=%v", epoch, err)
	}
	ok, err = isCurrentConnectionEpoch(context.Background(), "r1", "u1", epoch)
	if err != nil || !ok {
		t.Fatalf("current epoch should match: ok=%v err=%v", ok, err)
	}
	ok, err = isCurrentConnectionEpoch(context.Background(), "r1", "u2", 1)
	if err != nil || ok {
		t.Fatalf("missing latest key should return false,nil; ok=%v err=%v", ok, err)
	}
	_ = rdb2.Close()


	ConfigureWebSocketConnectionEpochStore(&redis.Client{}, time.Minute)
	wsEpochIncrFn = func(context.Context, *redis.Client, string) (int64, error) { return 0, errors.New("incr") }
	if _, err := registerConnectionEpoch(context.Background(), "r1", "u1"); err == nil {
		t.Fatal("expected incr error")
	}
	wsEpochIncrFn = func(context.Context, *redis.Client, string) (int64, error) { return 1, nil }
	wsEpochSetFn = func(context.Context, *redis.Client, string, any, time.Duration) error { return errors.New("set") }
	if _, err := registerConnectionEpoch(context.Background(), "r1", "u1"); err == nil {
		t.Fatal("expected set error")
	}
	if err := refreshConnectionEpoch(context.Background(), "r1", "u1", 1); err == nil {
		t.Fatal("expected refresh set error")
	}
	wsEpochSetFn = func(context.Context, *redis.Client, string, any, time.Duration) error { return nil }
	wsEpochGetInt64Fn = func(context.Context, *redis.Client, string) (int64, error) { return 0, errors.New("get") }
	if _, err := isCurrentConnectionEpoch(context.Background(), "r1", "u1", 1); err == nil {
		t.Fatal("expected get error")
	}
	wsEpochGetInt64Fn = func(context.Context, *redis.Client, string) (int64, error) { return 2, nil }
	ok2, err := isCurrentConnectionEpoch(context.Background(), "r1", "u1", 1)
	if err != nil || ok2 {
		t.Fatalf("expected false for stale epoch; ok=%v err=%v", ok2, err)
	}

	if wsEpochCounterKey("r1", "u1") == "" || wsEpochLatestKey("r1", "u1") == "" {
		t.Fatal("epoch keys must not be empty")
	}
}

func TestBuildRoomDTOAndRoomHub(t *testing.T) {
	now := time.Now().UTC()
	td := now.Add(time.Minute)
	rd := now.Add(2 * time.Minute)
	outcome := model.OutcomeWin
	score := 21
	state := &usecase.RoomState{
		Room: &model.Room{ID: "r1", Status: model.RoomStatusPlaying},
		Session: &model.GameSession{
			ID:                "s1",
			Status:            model.SessionStatusPlayerTurn,
			Version:           2,
			RoundNo:           1,
			TurnSeat:          1,
			TurnDeadlineAt:    &td,
			RematchDeadlineAt: &rd,
		},
		Dealer: &model.DealerState{
			Hand:       []model.StoredCard{{Rank: "A", Suit: "S"}, {Rank: "K", Suit: "D"}},
			HoleHidden: true,
		},
		Players: []*model.PlayerState{
			{UserID: "u1", SeatNo: 1, Status: model.PlayerStatusActive, Hand: []model.StoredCard{{Rank: "5", Suit: "H"}}, Outcome: &outcome, FinalScore: &score},
			{UserID: "u2", SeatNo: 1, Status: model.PlayerStatusStand, Hand: []model.StoredCard{{Rank: "9", Suit: "C"}}},
		},
		CanHit: true, CanStand: true, CanRematch: false,
	}
	me := buildRoomDTO(state, "u1")
	if len(me.Players) != 2 || me.Players[0].Hand == nil {
		t.Fatalf("unexpected player view: %+v", me.Players)
	}
	other := buildRoomDTO(state, "u2")
	if other.Players[0].Hand != nil {
		t.Fatal("other user must not see hand")
	}

	empty := buildRoomDTO(&usecase.RoomState{Room: &model.Room{ID: "r2", Status: model.RoomStatusWaiting}}, "u1")
	if empty.Room.ID != "r2" {
		t.Fatalf("unexpected empty dto room: %+v", empty.Room)
	}

	h := &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}, latest: map[string]*websocket.Conn{}}
	c1 := &websocket.Conn{}
	c2 := &websocket.Conn{}
	meta := wsConnMeta{userID: "u1", writeMu: &sync.Mutex{}}
	if old := h.add("r1", c1, meta); old != nil {
		t.Fatal("first add should not return old connection")
	}
	if old := h.add("r1", c2, meta); old != c1 {
		t.Fatal("second add should return old connection")
	}
	if !h.isLatest("r1", "u1", c2) {
		t.Fatal("c2 should be latest")
	}
	snap := h.snapshot("r1")
	if len(snap) != 1 {
		t.Fatalf("unexpected snapshot size: %d", len(snap))
	}
	h.remove("r1", c2)
	if h.isLatest("r1", "u1", c2) {
		t.Fatal("removed connection should not be latest")
	}
	h.remove("r1", c1)
}

func TestSendErrorAndPongHelpers(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		meta := wsConnMeta{writeMu: &sync.Mutex{}}
		sendWSError(conn, meta, dto.WSErrorInvalidInput, "bad")
		sendWSErrorWithRetry(conn, meta, dto.WSErrorRateLimited, "slow down", 123)
		sendWSPong(conn, meta)
	}))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial ws server: %v", err)
	}
	defer c.Close()

	for i := 0; i < 3; i++ {
		_, msg, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("read ws message: %v", err)
		}
		if i < 2 {
			var ev dto.WSErrorEvent
			if err := json.Unmarshal(msg, &ev); err != nil {
				t.Fatalf("unmarshal error event: %v", err)
			}
		}
	}
}
