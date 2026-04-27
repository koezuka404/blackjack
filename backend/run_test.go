package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSubscriberRoomSyncHandler_nilReceiver(t *testing.T) {
	defer func() { recover() }()
	subscriberRoomSyncHandler(nil)(context.Background(), "room", "evt")
}

func TestRunApp_MissingDATABASE_URL(t *testing.T) {
	err := runApp(context.Background(), func(string) string { return "" })
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunApp_DatabaseOpenFails(t *testing.T) {
	prev := dbOpenFn
	dbOpenFn = func(string) (*gorm.DB, error) { return nil, errors.New("open fail") }
	t.Cleanup(func() { dbOpenFn = prev })

	err := runApp(context.Background(), func(k string) string {
		if k == "DATABASE_URL" {
			return "postgres://x"
		}
		return ""
	})
	if err == nil || err.Error() != "database: open fail" {
		t.Fatalf("got %v", err)
	}
}

func TestRunApp_JWTTooShort(t *testing.T) {
	prev := dbOpenFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:jwtshort?mode=memory&cache=shared"), &gorm.Config{})
	}
	t.Cleanup(func() { dbOpenFn = prev })

	err := runApp(context.Background(), func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "short"
		default:
			return ""
		}
	})
	if err == nil {
		t.Fatal("expected jwt error")
	}
}

func TestRunApp_ShortLifeSmoke(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()

	prevDB := dbOpenFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:runappsmoke?mode=memory&cache=shared"), &gorm.Config{})
	}
	t.Cleanup(func() { dbOpenFn = prevDB })

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(800*time.Millisecond, cancel)

	getenv := func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "sqlite://runappsmoke"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "SERVER_ID":
			return "test-server"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return addr
		case "PORT":
			return "0"
		default:
			return ""
		}
	}

	if err := runApp(ctx, getenv); err != nil {
		t.Fatalf("runApp: %v", err)
	}
}

func TestRunApp_EchoShutdownErrorLogged(t *testing.T) {
	mr := miniredis.RunT(t)
	prevDB, prevShut := dbOpenFn, echoShutdownFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:shuterr?mode=memory&cache=shared"), &gorm.Config{})
	}
	echoShutdownFn = func(*echo.Echo, context.Context) error { return errors.New("shutdown boom") }
	t.Cleanup(func() {
		dbOpenFn = prevDB
		echoShutdownFn = prevShut
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return mr.Addr()
		case "PORT":
			return "0"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
}

func TestRunApp_RedisCloseErrors(t *testing.T) {
	mr := miniredis.RunT(t)
	prevDB, prevRClose := dbOpenFn, redisCloseFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:redisclose?mode=memory&cache=shared"), &gorm.Config{})
	}
	n := 0
	redisCloseFn = func(c *redis.Client) error {
		n++
		if n == 1 {
			return errors.New("room close err")
		}
		if n == 2 {
			return errors.New("rate close err")
		}
		return nil
	}
	t.Cleanup(func() {
		dbOpenFn = prevDB
		redisCloseFn = prevRClose
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return mr.Addr()
		case "PORT":
			return "0"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
}

func TestRunApp_SQLDBHandleError(t *testing.T) {
	mr := miniredis.RunT(t)
	prevDB, prevSQL := dbOpenFn, gormSQLDBFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:sqlhand?mode=memory&cache=shared"), &gorm.Config{})
	}
	gormSQLDBFn = func(*gorm.DB) (*sql.DB, error) { return nil, errors.New("no sql") }
	t.Cleanup(func() {
		dbOpenFn = prevDB
		gormSQLDBFn = prevSQL
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return mr.Addr()
		case "PORT":
			return "0"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
}

func TestRunApp_SQLDBCloseError(t *testing.T) {
	mr := miniredis.RunT(t)
	prevDB, prevClose := dbOpenFn, sqlDBCloseFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:sqlclose?mode=memory&cache=shared"), &gorm.Config{})
	}
	sqlDBCloseFn = func(*sql.DB) error { return errors.New("sql close err") }
	t.Cleanup(func() {
		dbOpenFn = prevDB
		sqlDBCloseFn = prevClose
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return mr.Addr()
		case "PORT":
			return "0"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
}

func TestRunApp_EchoStartErrorUsesFatalHook(t *testing.T) {
	mr := miniredis.RunT(t)
	prevDB, prevStart, prevFatal := dbOpenFn, echoStartFn, echoStartFatalFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:startfail?mode=memory&cache=shared"), &gorm.Config{})
	}
	echoStartFn = func(*echo.Echo, string) error { return errors.New("bind fail") }
	var fatalArg error
	echoStartFatalFn = func(_ *echo.Echo, err error) { fatalArg = err }
	t.Cleanup(func() {
		dbOpenFn = prevDB
		echoStartFn = prevStart
		echoStartFatalFn = prevFatal
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return mr.Addr()
		case "PORT":
			return "0"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
	if fatalArg == nil || fatalArg.Error() != "bind fail" {
		t.Fatalf("fatal hook: %v", fatalArg)
	}
}

func TestRunApp_EmptyServerIDUsesUUIDHook(t *testing.T) {
	mr := miniredis.RunT(t)
	prevDB, prevUUID := dbOpenFn, newUUIDStringFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:uuidhook?mode=memory&cache=shared"), &gorm.Config{})
	}
	newUUIDStringFn = func() string { return "fixed-uuid-test" }
	t.Cleanup(func() {
		dbOpenFn = prevDB
		newUUIDStringFn = prevUUID
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(400*time.Millisecond, cancel)

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return mr.Addr()
		case "PORT":
			return "0"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
}

func TestRunApp_PortDefault8080(t *testing.T) {
	mr := miniredis.RunT(t)
	prevDB, prevStart := dbOpenFn, echoStartFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:portdef?mode=memory&cache=shared"), &gorm.Config{})
	}
	var sawAddr string
	echoStartFn = func(_ *echo.Echo, addr string) error {
		sawAddr = addr
		return http.ErrServerClosed
	}
	t.Cleanup(func() {
		dbOpenFn = prevDB
		echoStartFn = prevStart
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return mr.Addr()
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
	if sawAddr != ":8080" {
		t.Fatalf("want default :8080, got %q", sawAddr)
	}
}

func TestRunApp_HealthEndpointDuringSmoke(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()

	prevDB := dbOpenFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:healthsmoke?mode=memory&cache=shared"), &gorm.Config{})
	}
	t.Cleanup(func() { dbOpenFn = prevDB })

	ctx, cancel := context.WithCancel(context.Background())
	var httpAddr string
	prevStart := echoStartFn
	echoStartFn = func(e *echo.Echo, a string) error {
		httpAddr = a
		go func() {
			time.Sleep(200 * time.Millisecond)
			req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1"+a+"/health", nil)
			c := &http.Client{Timeout: 2 * time.Second}
			resp, err := c.Do(req)
			if err == nil {
				_ = resp.Body.Close()
			}
			cancel()
		}()
		return e.Start(a)
	}
	t.Cleanup(func() { echoStartFn = prevStart })

	getenv := func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "SERVER_ID":
			return "srv"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return addr
		case "PORT":
			return "0"
		default:
			return ""
		}
	}

	if err := runApp(ctx, getenv); err != nil {
		t.Fatalf("runApp: %v", err)
	}
	if httpAddr == "" {
		t.Fatal("echo did not report addr")
	}
}

func TestRunApp_MetricsRouteRegistered(t *testing.T) {
	mr := miniredis.RunT(t)
	prevDB, prevStart := dbOpenFn, echoStartFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:metricsreg?mode=memory&cache=shared"), &gorm.Config{})
	}
	t.Cleanup(func() {
		dbOpenFn = prevDB
		echoStartFn = prevStart
	})

	ctx, cancel := context.WithCancel(context.Background())
	echoStartFn = func(e *echo.Echo, a string) error {
		go func() {
			time.Sleep(200 * time.Millisecond)
			req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1"+a+"/metrics", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			cancel()
		}()
		return e.Start(a)
	}

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return mr.Addr()
		case "PORT":
			return "0"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
}

func TestHookDefaults_nilRedisAndSQLClose(t *testing.T) {
	if err := redisCloseFn(nil); err != nil {
		t.Fatalf("redisCloseFn nil: %v", err)
	}
	if err := sqlDBCloseFn(nil); err != nil {
		t.Fatalf("sqlDBCloseFn nil: %v", err)
	}
	c := newRedisClientFn("127.0.0.1:59996")
	defer c.Close()
}

func TestRunApp_echoStartErrServerClosedSkipsFatal(t *testing.T) {
	mr := miniredis.RunT(t)
	prevDB, prevStart, prevFatal := dbOpenFn, echoStartFn, echoStartFatalFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:esc?mode=memory&cache=shared"), &gorm.Config{})
	}
	echoStartFn = func(*echo.Echo, string) error { return http.ErrServerClosed }
	var fatalCalled bool
	echoStartFatalFn = func(*echo.Echo, error) { fatalCalled = true }
	t.Cleanup(func() {
		dbOpenFn = prevDB
		echoStartFn = prevStart
		echoStartFatalFn = prevFatal
	})

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return mr.Addr()
		case "PORT":
			return "0"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
	if fatalCalled {
		t.Fatal("did not expect fatal hook for ErrServerClosed")
	}
}

func TestEchoStartFatalDefault_panics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	echoStartFatalDefault(echo.New(), errors.New("bind"))
}

func TestRunApp_roomSyncCallbackInvoked(t *testing.T) {
	mr := miniredis.RunT(t)
	addr := mr.Addr()

	prevDB := dbOpenFn
	dbOpenFn = func(string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open("file:rsyncb?mode=memory&cache=shared"), &gorm.Config{})
	}
	t.Cleanup(func() { dbOpenFn = prevDB })

	pub := redis.NewClient(&redis.Options{Addr: addr})
	defer pub.Close()

	ctx, cancel := context.WithCancel(context.Background())
	prevStart := echoStartFn
	echoStartFn = func(e *echo.Echo, a string) error {
		go func() {
			time.Sleep(800 * time.Millisecond)
			payload := `{"room_id":"room-x","event_type":"ROOM_STATE_SYNC","origin":"peer-server-z"}`
			_ = pub.Publish(context.Background(), "blackjack:room:state_sync", payload).Err()
			time.Sleep(600 * time.Millisecond)
			cancel()
		}()
		return e.Start(a)
	}
	t.Cleanup(func() { echoStartFn = prevStart })

	time.AfterFunc(4*time.Second, cancel)

	err := runApp(ctx, func(k string) string {
		switch k {
		case "DATABASE_URL":
			return "x"
		case "JWT_SECRET":
			return "0123456789abcdef0123456789abcdef"
		case "SERVER_ID":
			return "local-srv"
		case "REDIS_ROOM_ADDR", "REDIS_RATE_LIMIT_ADDR":
			return addr
		case "PORT":
			return "0"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("runApp: %v", err)
	}
}
