module blackjack/backend

go 1.24

require (
	github.com/ethanefung/blackjack v0.2.1
	github.com/ethanefung/cards v0.1.0
	github.com/golang-jwt/jwt/v5 v5.2.2
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/labstack/echo-jwt/v4 v4.4.0
	github.com/jackc/pgx/v5 v5.5.5
	github.com/labstack/echo/v4 v4.13.3
	github.com/prometheus/client_golang v1.23.2
	github.com/redis/go-redis/v9 v9.7.0
	golang.org/x/crypto v0.41.0
	gorm.io/driver/postgres v1.5.11
	gorm.io/gorm v1.25.12
)

replace github.com/ethanefung/blackjack => github.com/ethanefung/blackjack v0.2.1

replace github.com/ethanefung/cards => github.com/ethanefung/cards v0.1.0
