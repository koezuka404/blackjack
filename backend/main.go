








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

	"blackjack/backend/controller"
	"blackjack/backend/db"
	"blackjack/backend/observability"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

var (
	_ = websocket.ErrBadHandshake


	mainEntryFn = defaultMainEntry


	fatalLogFn = log.Fatal
)

func defaultMainEntry() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := runApp(ctx, os.Getenv); err != nil {
		fatalLogFn(err)
	}
}

func main() {
	mainEntryFn()
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

var (
	autoStandWorkerInterval = 1 * time.Second
	snapshotWorkerInterval  = 10 * time.Second


	autoStandDueSessionsFn = func(ctx context.Context, uc usecase.RoomUsecase) ([]string, error) {
		return uc.AutoStandDueSessions(ctx)
	}


	broadcastRoomSyncFn = func(ctx context.Context, rc *controller.RoomController, roomID string) {
		rc.BroadcastRoomSync(ctx, roomID)
	}

	snapshotCountRoomsFn    = func(ctx context.Context, s repository.Store) (int64, error) { return s.CountRooms(ctx) }
	snapshotCountSessionsFn = func(ctx context.Context, s repository.Store) (int64, error) { return s.CountSessions(ctx) }
)

func runAutoStandWorker(ctx context.Context, e *echo.Echo, roomUC usecase.RoomUsecase, roomController *controller.RoomController) {
	ticker := time.NewTicker(autoStandWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			roomIDs, err := autoStandDueSessionsFn(ctx, roomUC)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					e.Logger.Errorf("auto-stand worker error: %v", err)
				}
				continue
			}
			for _, roomID := range roomIDs {
				broadcastRoomSyncFn(ctx, roomController, roomID)
			}
		}
	}
}

func runSnapshotMetricsWorker(ctx context.Context, e *echo.Echo, store repository.Store) {
	ticker := time.NewTicker(snapshotWorkerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rooms, err := snapshotCountRoomsFn(ctx, store)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					e.Logger.Errorf("metrics room count error: %v", err)
				}
				continue
			}
			sessions, err := snapshotCountSessionsFn(ctx, store)
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
