package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"blackjack/backend/db"

	_ "github.com/ethanefung/blackjack"
	_ "github.com/ethanefung/cards"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

var (
	_ = websocket.ErrBadHandshake
	_ = redis.NewClient(&redis.Options{})
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

	e := echo.New()
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
