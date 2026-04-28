package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"blackjack/backend/adapter/blackjackadapter"
	"blackjack/backend/controller"
	"blackjack/backend/db"
	"blackjack/backend/realtime"
	"blackjack/backend/repository"
	"blackjack/backend/router"
	"blackjack/backend/usecase"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)


var (
	dbOpenFn = db.Open
	newRedisClientFn = func(addr string) *redis.Client {
		return redis.NewClient(&redis.Options{Addr: addr})
	}
	newUUIDStringFn = uuid.NewString
	echoStartFn = func(e *echo.Echo, addr string) error {
		return e.Start(addr)
	}
	echoShutdownFn = func(e *echo.Echo, ctx context.Context) error {
		return e.Shutdown(ctx)
	}
	redisCloseFn = func(c *redis.Client) error {
		if c == nil {
			return nil
		}
		return c.Close()
	}
	gormSQLDBFn = func(g *gorm.DB) (*sql.DB, error) {
		return g.DB()
	}
	sqlDBCloseFn = func(sdb *sql.DB) error {
		if sdb == nil {
			return nil
		}
		return sdb.Close()
	}
	echoStartFatalFn = echoStartFatalDefault
)

func echoStartFatalDefault(e *echo.Echo, err error) {
	_ = e
	panic(err)
}

func runApp(ctx context.Context, getenv func(string) string) error {
	dsn := getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL is required (set in .env or export in shell)")
	}

	gdb, err := dbOpenFn(dsn)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	store := repository.NewPostgreSQLStore(gdb)
	roomRedisAddr := parseRedisAddr("REDIS_ROOM_ADDR")
	rateLimitRedisAddr := parseRedisAddr("REDIS_RATE_LIMIT_ADDR")

	roomRedis := newRedisClientFn(roomRedisAddr)
	rateLimitRedis := newRedisClientFn(rateLimitRedisAddr)

	rateLimitRepo := repository.NewRedisTokenBucketRepository(rateLimitRedis, 20, 5.0)
	limiter := usecase.NewRateLimitUsecase(rateLimitRepo)

	jwtSecret := []byte(strings.TrimSpace(getenv("JWT_SECRET")))
	if len(jwtSecret) < 16 {
		return fmt.Errorf("JWT_SECRET is required and must be at least 16 bytes")
	}
	authUC := usecase.NewAuthUsecase(store, jwtSecret)
	roomUC := usecase.NewRoomUsecase(store, blackjackadapter.NewHandEvaluator(), blackjackadapter.NewRoundEngine())

	serverID := strings.TrimSpace(getenv("SERVER_ID"))
	if serverID == "" {
		serverID = newUUIDStringFn()
	}
	roomSyncBroker := realtime.NewRoomSyncBroker(roomRedis, serverID)
	roomSyncCtx, roomSyncCancel := context.WithCancel(context.Background())

	controller.ConfigureWebSocketAllowedOrigins(parseWSAllowedOrigins())
	controller.ConfigureWebSocketConnectionEpochStore(roomRedis, parseWSConnectionEpochTTL())

	e := echo.New()
	roomController := router.Register(e, store, limiter, authUC, roomUC, roomSyncBroker, jwtSecret)

	go func() {
		_ = roomSyncBroker.RunSubscriber(roomSyncCtx, subscriberRoomSyncHandler(roomController))
	}()

	workerCtx, workerCancel := context.WithCancel(context.Background())
	go runAutoStandWorker(workerCtx, e, roomUC, roomController)
	go runSnapshotMetricsWorker(workerCtx, e, store)

	e.GET("/health", healthHandler(gdb, roomRedis, rateLimitRedis))
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	port := getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	go func() {
		if err := echoStartFn(e, addr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			echoStartFatalFn(e, err)
		}
	}()

	<-ctx.Done()

	roomSyncCancel()
	workerCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := echoShutdownFn(e, shutdownCtx); err != nil {
		e.Logger.Errorf("server shutdown: %v", err)
	}

	if err := redisCloseFn(roomRedis); err != nil {
		log.Printf("room redis close: %v", err)
	}
	if err := redisCloseFn(rateLimitRedis); err != nil {
		log.Printf("rate-limit redis close: %v", err)
	}
	sqlDB, err := gormSQLDBFn(gdb)
	if err != nil {
		log.Printf("database sql handle: %v", err)
	} else if err := sqlDBCloseFn(sqlDB); err != nil {
		log.Printf("database close: %v", err)
	}
	return nil
}

func subscriberRoomSyncHandler(rc *controller.RoomController) func(context.Context, string, string) {
	return func(c context.Context, roomID, eventType string) {
		rc.BroadcastRoomStateFromPeer(c, roomID, eventType)
	}
}
