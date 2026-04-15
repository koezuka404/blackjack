// 運用関連の環境変数: PORT（既定 8080）, WS_ALLOWED_ORIGINS（カンマ区切り・空なら WS Origin 許可は開発モード）,
// BLACKJACK_WS_MARK_DISCONNECTED（true/false・WS 切断時に DISCONNECTED を DB 反映するか、既定 true）。
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
	"blackjack/backend/middleware"
	"blackjack/backend/repository"
	"blackjack/backend/router"
	"blackjack/backend/usecase"

	_ "github.com/ethanefung/blackjack"
	_ "github.com/ethanefung/cards"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
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
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	limiter := middleware.NewRedisTokenBucketLimiter(rdb, 20, 5.0)
	authUC := usecase.NewAuthUsecase(store)
	roomUC := usecase.NewRoomUsecase(store, blackjackadapter.NewHandEvaluator(), blackjackadapter.NewRoundEngine())

	controller.ConfigureWebSocketAllowedOrigins(parseWSAllowedOrigins())

	e := echo.New()
	roomController := router.Register(e, store, limiter, authUC, roomUC)

	workerCtx, workerCancel := context.WithCancel(context.Background())
	go runAutoStandWorker(workerCtx, e, roomUC, roomController)

	e.GET("/health", healthHandler(gdb, rdb))

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

	workerCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := e.Shutdown(shutdownCtx); err != nil {
		e.Logger.Errorf("server shutdown: %v", err)
	}

	if err := rdb.Close(); err != nil {
		log.Printf("redis close: %v", err)
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

func healthHandler(gdb *gorm.DB, rdb *redis.Client) echo.HandlerFunc {
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
		if err := rdb.Ping(ctx).Err(); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]any{
				"status":   "unhealthy",
				"database": "up",
				"redis":    "down",
				"error":    err.Error(),
			})
		}
		return c.JSON(http.StatusOK, map[string]string{
			"status":   "healthy",
			"database": "up",
			"redis":    "up",
		})
	}
}
