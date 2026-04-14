package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"blackjack/backend/adapter/blackjackadapter"
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

	e := echo.New()
	roomController := router.Register(e, store, limiter, authUC, roomUC)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			roomIDs, err := roomUC.AutoStandDueSessions(context.Background())
			if err != nil {
				e.Logger.Errorf("auto-stand worker error: %v", err)
				continue
			}
			for _, roomID := range roomIDs {
				roomController.BroadcastRoomSync(context.Background(), roomID)
			}
		}
	}()
	e.GET("/health", func(c echo.Context) error {
		ctx, cancel := context.WithTimeout(c.Request().Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx, gdb); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{
				"status":   "unhealthy",
				"database": "down",
				"error":    err.Error(),
			})
		}
		return c.JSON(http.StatusOK, map[string]string{
			"status":   "healthy",
			"database": "up",
		})
	})

	e.Logger.Fatal(e.Start(":8080"))
}
