package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackjack/backend/controller"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/alicebob/miniredis/v2"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestParseWSAllowedOrigins(t *testing.T) {
	t.Setenv("WS_ALLOWED_ORIGINS", "")
	if v := parseWSAllowedOrigins(); v != nil {
		t.Fatalf("empty env: got %#v", v)
	}
	t.Setenv("WS_ALLOWED_ORIGINS", " , , ")
	if v := parseWSAllowedOrigins(); v != nil {
		t.Fatalf("only commas/spaces: got %#v", v)
	}
	t.Setenv("WS_ALLOWED_ORIGINS", "https://a.example, https://b.example ")
	v := parseWSAllowedOrigins()
	if len(v) != 2 || v[0] != "https://a.example" || v[1] != "https://b.example" {
		t.Fatalf("got %#v", v)
	}
}

func TestParseWSConnectionEpochTTL(t *testing.T) {
	t.Setenv("WS_CONNECTION_EPOCH_TTL", "")
	if d := parseWSConnectionEpochTTL(); d != 2*time.Minute {
		t.Fatalf("default: %v", d)
	}
	t.Setenv("WS_CONNECTION_EPOCH_TTL", "not-a-duration")
	if d := parseWSConnectionEpochTTL(); d != 2*time.Minute {
		t.Fatalf("invalid: %v", d)
	}
	t.Setenv("WS_CONNECTION_EPOCH_TTL", "-5s")
	if d := parseWSConnectionEpochTTL(); d != 2*time.Minute {
		t.Fatalf("non-positive: %v", d)
	}
	t.Setenv("WS_CONNECTION_EPOCH_TTL", "45s")
	if d := parseWSConnectionEpochTTL(); d != 45*time.Second {
		t.Fatalf("45s: %v", d)
	}
}

func TestParseRedisAddr(t *testing.T) {
	t.Setenv("REDIS_ROOM_ADDR", "room:6379")
	t.Setenv("REDIS_ADDR", "legacy:6379")
	if got := parseRedisAddr("REDIS_ROOM_ADDR"); got != "room:6379" {
		t.Fatalf("explicit: %q", got)
	}
	t.Setenv("REDIS_ROOM_ADDR", "")
	if got := parseRedisAddr("REDIS_ROOM_ADDR"); got != "legacy:6379" {
		t.Fatalf("fallback REDIS_ADDR: %q", got)
	}
	t.Setenv("REDIS_ADDR", "")
	if got := parseRedisAddr("REDIS_ROOM_ADDR"); got != "localhost:6379" {
		t.Fatalf("default: %q", got)
	}
}

func TestHealthHandler_AllPaths(t *testing.T) {
	mr := miniredis.RunT(t)
	roomR := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rateR := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer roomR.Close()
	defer rateR.Close()

	gdb, err := gorm.Open(sqlite.Open("file:healthdb?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := healthHandler(gdb, roomR, rateR)(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	badGDB := &gorm.DB{}
	req2 := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec2 := httptest.NewRecorder()
	c2 := e.NewContext(req2, rec2)
	_ = healthHandler(badGDB, roomR, rateR)(c2)
	if rec2.Code != http.StatusServiceUnavailable {
		t.Fatalf("bad db: want 503 got %d", rec2.Code)
	}

	roomDown := redis.NewClient(&redis.Options{Addr: "127.0.0.1:59998"})
	defer roomDown.Close()
	req3 := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec3 := httptest.NewRecorder()
	c3 := e.NewContext(req3, rec3)
	_ = healthHandler(gdb, roomDown, rateR)(c3)
	if rec3.Code != http.StatusServiceUnavailable {
		t.Fatalf("room redis down: want 503 got %d", rec3.Code)
	}

	rateDown := redis.NewClient(&redis.Options{Addr: "127.0.0.1:59997"})
	defer rateDown.Close()
	req4 := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec4 := httptest.NewRecorder()
	c4 := e.NewContext(req4, rec4)
	_ = healthHandler(gdb, roomR, rateDown)(c4)
	if rec4.Code != http.StatusServiceUnavailable {
		t.Fatalf("rate redis down: want 503 got %d", rec4.Code)
	}
}

func TestRunAutoStandWorker_CanceledContext(t *testing.T) {
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runAutoStandWorker(ctx, e, nil, nil)
}

func TestRunSnapshotMetricsWorker_CanceledContext(t *testing.T) {
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runSnapshotMetricsWorker(ctx, e, nil)
}

func TestMain_dispatchesToMainEntry(t *testing.T) {
	prev := mainEntryFn
	mainEntryFn = func() {}
	defer func() { mainEntryFn = prev }()
	main()
}

func TestDefaultMainEntry_runAppReturnsFatalHook(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")

	prevFatal := fatalLogFn
	var got interface{}
	fatalLogFn = func(v ...interface{}) { got = v[0] }
	t.Cleanup(func() { fatalLogFn = prevFatal })

	defaultMainEntry()
	if got == nil {
		t.Fatal("expected fatalLogFn to be called")
	}
}

func TestBroadcastRoomSyncFn_nilReceiver(t *testing.T) {
	defer func() { recover() }()
	broadcastRoomSyncFn(context.Background(), nil, "r1")
}

func TestRunAutoStandWorker_nonEmptyRoomIDs(t *testing.T) {
	prevI, prevFn, prevB := autoStandWorkerInterval, autoStandDueSessionsFn, broadcastRoomSyncFn
	autoStandWorkerInterval = 20 * time.Millisecond
	autoStandDueSessionsFn = func(context.Context, usecase.RoomUsecase) ([]string, error) {
		return []string{"r1", "r2"}, nil
	}
	var syncCalls int
	broadcastRoomSyncFn = func(context.Context, *controller.RoomController, string) { syncCalls++ }
	t.Cleanup(func() {
		autoStandWorkerInterval = prevI
		autoStandDueSessionsFn = prevFn
		broadcastRoomSyncFn = prevB
	})
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runAutoStandWorker(ctx, e, nil, nil)
		close(done)
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done
	if syncCalls < 1 {
		t.Fatalf("expected broadcastRoomSyncFn to be called, got %d", syncCalls)
	}
}

func TestRunAutoStandWorker_successEmptyRooms(t *testing.T) {
	prevI, prevFn, prevB := autoStandWorkerInterval, autoStandDueSessionsFn, broadcastRoomSyncFn
	autoStandWorkerInterval = 20 * time.Millisecond
	autoStandDueSessionsFn = func(context.Context, usecase.RoomUsecase) ([]string, error) {
		return nil, nil
	}
	broadcastRoomSyncFn = func(context.Context, *controller.RoomController, string) {}
	t.Cleanup(func() {
		autoStandWorkerInterval = prevI
		autoStandDueSessionsFn = prevFn
		broadcastRoomSyncFn = prevB
	})
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runAutoStandWorker(ctx, e, nil, nil)
		close(done)
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done
}

func TestRunAutoStandWorker_canceledErrorSkipped(t *testing.T) {
	prevI, prevFn := autoStandWorkerInterval, autoStandDueSessionsFn
	autoStandWorkerInterval = 20 * time.Millisecond
	autoStandDueSessionsFn = func(ctx context.Context, _ usecase.RoomUsecase) ([]string, error) {
		return nil, context.Canceled
	}
	t.Cleanup(func() {
		autoStandWorkerInterval = prevI
		autoStandDueSessionsFn = prevFn
	})
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runAutoStandWorker(ctx, e, nil, nil)
		close(done)
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done
}

func TestRunAutoStandWorker_nonCanceledErrorPath(t *testing.T) {
	prevI, prevFn := autoStandWorkerInterval, autoStandDueSessionsFn
	autoStandWorkerInterval = 25 * time.Millisecond
	autoStandDueSessionsFn = func(context.Context, usecase.RoomUsecase) ([]string, error) {
		return nil, errors.New("auto-stand boom")
	}
	t.Cleanup(func() {
		autoStandWorkerInterval = prevI
		autoStandDueSessionsFn = prevFn
	})

	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runAutoStandWorker(ctx, e, nil, nil)
		close(done)
	}()
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done
}

func TestRunSnapshotMetricsWorker_countRoomsError(t *testing.T) {
	prevI, prevR := snapshotWorkerInterval, snapshotCountRoomsFn
	snapshotWorkerInterval = 30 * time.Millisecond
	snapshotCountRoomsFn = func(context.Context, repository.Store) (int64, error) {
		return 0, errors.New("room count err")
	}
	t.Cleanup(func() {
		snapshotWorkerInterval = prevI
		snapshotCountRoomsFn = prevR
	})
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runSnapshotMetricsWorker(ctx, e, nil)
		close(done)
	}()
	time.Sleep(70 * time.Millisecond)
	cancel()
	<-done
}

func TestRunSnapshotMetricsWorker_canceledErrorsSkipped(t *testing.T) {
	prevI, prevR, prevS := snapshotWorkerInterval, snapshotCountRoomsFn, snapshotCountSessionsFn
	snapshotWorkerInterval = 25 * time.Millisecond
	snapshotCountRoomsFn = func(ctx context.Context, _ repository.Store) (int64, error) {
		return 0, context.Canceled
	}
	snapshotCountSessionsFn = func(ctx context.Context, _ repository.Store) (int64, error) {
		return 0, context.Canceled
	}
	t.Cleanup(func() {
		snapshotWorkerInterval = prevI
		snapshotCountRoomsFn = prevR
		snapshotCountSessionsFn = prevS
	})
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runSnapshotMetricsWorker(ctx, e, nil)
		close(done)
	}()
	time.Sleep(60 * time.Millisecond)
	cancel()
	<-done
}

func TestRunSnapshotMetricsWorker_countSessionsError(t *testing.T) {
	prevI, prevR, prevS := snapshotWorkerInterval, snapshotCountRoomsFn, snapshotCountSessionsFn
	snapshotWorkerInterval = 30 * time.Millisecond
	snapshotCountRoomsFn = func(context.Context, repository.Store) (int64, error) { return 0, nil }
	snapshotCountSessionsFn = func(context.Context, repository.Store) (int64, error) {
		return 0, errors.New("session count err")
	}
	t.Cleanup(func() {
		snapshotWorkerInterval = prevI
		snapshotCountRoomsFn = prevR
		snapshotCountSessionsFn = prevS
	})
	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runSnapshotMetricsWorker(ctx, e, nil)
		close(done)
	}()
	time.Sleep(70 * time.Millisecond)
	cancel()
	<-done
}

func TestRunSnapshotMetricsWorker_successTick(t *testing.T) {
	prev := snapshotWorkerInterval
	snapshotWorkerInterval = 60 * time.Millisecond
	t.Cleanup(func() { snapshotWorkerInterval = prev })

	dsn := fmt.Sprintf("file:snapw_%d?mode=memory&cache=shared", time.Now().UnixNano())
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := gdb.AutoMigrate(
		&repository.RoomRecord{}, &repository.RoomPlayerRecord{}, &repository.GameSessionRecord{}, &repository.PlayerStateRecord{},
		&repository.DealerStateRecord{}, &repository.ActionLogRecord{}, &repository.RematchVoteRecord{}, &repository.RoundLogRecord{},
		&repository.UserRecord{}, &repository.SessionRecord{},
	); err != nil {
		t.Fatal(err)
	}
	store := repository.NewPostgreSQLStore(gdb)

	e := echo.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runSnapshotMetricsWorker(ctx, e, store)
		close(done)
	}()
	time.Sleep(150 * time.Millisecond)
	cancel()
	<-done
}
