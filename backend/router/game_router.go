package router

import (
	"blackjack/backend/controller"
	"blackjack/backend/middleware"
	"blackjack/backend/realtime"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)

// Register wires HTTP/WS routes and returns RoomController for broadcasts.
func Register(
	e *echo.Echo,
	store repository.Store,
	limiter middleware.RateLimiter,
	authUC usecase.AuthUsecase,
	roomUC usecase.RoomUsecase,
	roomSync *realtime.RoomSyncBroker,
) *controller.RoomController {
	api := e.Group("/api")
	api.Use(middleware.HTTPTelemetryMiddleware())
	api.Use(middleware.RequestIDMiddleware())
	api.Use(middleware.AuthMiddleware(store))
	api.Use(middleware.RateLimitMiddleware(limiter))
	api.Use(middleware.CSRFMiddleware())
	api.Use(middleware.AuditLogMiddleware())

	controller.NewAuthController(authUC).Register(api)
	roomController := controller.NewRoomController(roomUC, limiter, roomSync)
	roomController.Register(api)

	ws := e.Group("/ws")
	ws.Use(middleware.HTTPTelemetryMiddleware())
	ws.Use(middleware.RequestIDMiddleware())
	ws.Use(middleware.AuthMiddleware(store))
	ws.Use(middleware.AuditLogMiddleware())
	ws.GET("/rooms/:id", roomController.RoomWS)

	return roomController
}
