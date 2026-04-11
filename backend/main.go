package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"blackjack/backend/controller"
	"blackjack/backend/db"
	"blackjack/backend/middleware"
	"blackjack/backend/repository/gormrepo"
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
		log.Fatal("DATABASE_URL is required (see .env.example in repo root)")
	}

	gdb, err := db.Open(dsn)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	store := gormrepo.New(gdb)
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	limiter := middleware.NewRedisTokenBucketLimiter(rdb, 20, 5.0)
	authUC := usecase.NewAuthUsecase(store)

	e := echo.New()
	api := e.Group("/api")
	api.Use(middleware.RequestIDMiddleware())
	api.Use(middleware.AuthMiddleware(store))
	api.Use(middleware.RateLimitMiddleware(limiter))
	api.Use(middleware.CSRFMiddleware())
	api.Use(middleware.AuditLogMiddleware())
	controller.NewAuthController(authUC).Register(api)
	roomUC := usecase.NewRoomUsecase(store)
	roomController := controller.NewRoomController(roomUC, limiter)
	roomController.Register(api)
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
	ws := e.Group("/ws")
	ws.Use(middleware.RequestIDMiddleware())
	ws.Use(middleware.AuthMiddleware(store))
	ws.Use(middleware.AuditLogMiddleware())
	ws.GET("/rooms/:id", roomController.RoomWS)
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
