// 運用関連の環境変数: PORT（既定 8080）, WS_ALLOWED_ORIGINS（カンマ区切り・空なら WS Origin 許可は開発モード）,
// BLACKJACK_WS_MARK_DISCONNECTED（true/false・WS 切断時に DISCONNECTED を DB 反映するか、既定 true）,
// WS_CONNECTION_EPOCH_TTL（例: 120s。複数インスタンス用 connection_epoch TTL、既定 120s）,
// SERVER_ID（複数インスタンス時の Pub/Sub 重複防止用。未設定は起動ごとに UUID）,
// BLACKJACK_PLAYER_TIMEOUT_POLICY（空または未設定: タイムアウトは自動スタンド。heuristic: 中級向けヒューリスティックでヒット可能なら Hit を試みる）,
// REDIS_ROOM_ADDR（ゲームルーム同期/WS connection_epoch 用。未設定時 REDIS_ADDR を参照、既定 localhost:6379）,
// REDIS_RATE_LIMIT_ADDR（レートリミット用。未設定時 REDIS_ADDR を参照、既定 localhost:6379）。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"blackjack/backend/adapter/blackjackadapter"
	"blackjack/backend/controller"
	"blackjack/backend/db"
	"blackjack/backend/observability"
	"blackjack/backend/realtime"
	"blackjack/backend/repository"
	"blackjack/backend/router"
	"blackjack/backend/usecase"

	_ "github.com/ethanefung/blackjack"
	_ "github.com/ethanefung/cards"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	_ = websocket.ErrBadHandshake
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL is required (set in .env or export in shell)")
	}

	gdb, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	store := repository.NewPostgreSQLStore(gdb)
	roomRedisAddr := parseRedisAddr("REDIS_ROOM_ADDR")
	rateLimitRedisAddr := parseRedisAddr("REDIS_RATE_LIMIT_ADDR")

	roomRedis := redis.NewClient(&redis.Options{Addr: roomRedisAddr})
	rateLimitRedis := redis.NewClient(&redis.Options{Addr: rateLimitRedisAddr})

	rateLimitRepo := repository.NewRedisTokenBucketRepository(rateLimitRedis, 20, 5.0)
	limiter := usecase.NewRateLimitUsecase(rateLimitRepo)
	authUC := usecase.NewAuthUsecase(store)
	roomUC := usecase.NewRoomUsecase(store, blackjackadapter.NewHandEvaluator(), blackjackadapter.NewRoundEngine())

	serverID := strings.TrimSpace(os.Getenv("SERVER_ID"))
	if serverID == "" {
		serverID = uuid.NewString()
	}
	roomSyncBroker := realtime.NewRoomSyncBroker(roomRedis, serverID)
	roomSyncCtx, roomSyncCancel := context.WithCancel(context.Background())

	controller.ConfigureWebSocketAllowedOrigins(parseWSAllowedOrigins())
	controller.ConfigureWebSocketConnectionEpochStore(roomRedis, parseWSConnectionEpochTTL())

	e := echo.New()
	roomController := router.Register(e, store, limiter, authUC, roomUC, roomSyncBroker)

	go func() {
		err := roomSyncBroker.RunSubscriber(roomSyncCtx, func(ctx context.Context, roomID, eventType string) {
			roomController.BroadcastRoomStateFromPeer(ctx, roomID, eventType)
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			e.Logger.Errorf("room sync subscriber: %v", err)
		}
	}()

	workerCtx, workerCancel := context.WithCancel(context.Background())
	go runAutoStandWorker(workerCtx, e, roomUC, roomController)
	go runSnapshotMetricsWorker(workerCtx, e, store)

	e.GET("/health", healthHandler(gdb, roomRedis, rateLimitRedis))
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	addr := ":" + port

	go func() {
		if err := e.Start(addr); err != nil && err != http.ErrServerClosed {
			e.Logger.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	roomSyncCancel()
	workerCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		e.Logger.Errorf("server shutdown: %v", err)
	}

	if err := roomRedis.Close(); err != nil {
		log.Printf("room redis close: %v", err)
	}
	if err := rateLimitRedis.Close(); err != nil {
		log.Printf("rate-limit redis close: %v", err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		log.Printf("database sql handle: %v", err)
	} else if err := sqlDB.Close(); err != nil {
		log.Printf("database close: %v", err)
	}
}

func parseWSAllowedOrigins() []string {
	s := os.Getenv("WS_ALLOWED_ORIGINS")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseWSConnectionEpochTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("WS_CONNECTION_EPOCH_TTL"))
	if raw == "" {
		return 2 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 2 * time.Minute
	}
	return d
}

func parseRedisAddr(targetEnv string) string {
	if addr := strings.TrimSpace(os.Getenv(targetEnv)); addr != "" {
		return addr
	}
	if legacy := strings.TrimSpace(os.Getenv("REDIS_ADDR")); legacy != "" {
		return legacy
	}
	return "localhost:6379"
}

func runAutoStandWorker(ctx context.Context, e *echo.Echo, roomUC usecase.RoomUsecase, roomController *controller.RoomController) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			roomIDs, err := roomUC.AutoStandDueSessions(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					e.Logger.Errorf("auto-stand worker error: %v", err)
				}
				continue
			}
			for _, roomID := range roomIDs {
				roomController.BroadcastRoomSync(ctx, roomID)
			}
		}
	}
}

func runSnapshotMetricsWorker(ctx context.Context, e *echo.Echo, store repository.Store) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rooms, err := store.CountRooms(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					e.Logger.Errorf("metrics room count error: %v", err)
				}
				continue
			}
			sessions, err := store.CountSessions(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					e.Logger.Errorf("metrics session count error: %v", err)
				}
				continue
			}
			observability.SetRoomCount(float64(rooms))
			observability.SetSessionCount(float64(sessions))
		}
	}
}

func healthHandler(gdb *gorm.DB, roomRedis, rateLimitRedis *redis.Client) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx, gdb); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"status":   "unhealthy",
				"database": "down",
				"redis":    "unknown",
				"error":    err.Error(),
			})
		}
		if err := roomRedis.Ping(ctx).Err(); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"status":           "unhealthy",
				"database":         "up",
				"room_redis":       "down",
				"rate_limit_redis": "unknown",
				"error":            err.Error(),
			})
		}
		if err := rateLimitRedis.Ping(ctx).Err(); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"status":           "unhealthy",
				"database":         "up",
				"room_redis":       "up",
				"rate_limit_redis": "down",
				"error":            err.Error(),
			})
		}
		return c.JSON(http.StatusOK, map[string]string{
			"status":           "healthy",
			"database":         "up",
			"room_redis":       "up",
			"rate_limit_redis": "up",
		})
	}
}
