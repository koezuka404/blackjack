FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server . \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/migrate ./migrations

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata wget
COPY --from=build /out/server /app/server
COPY --from=build /out/migrate /app/migrate
USER nobody
EXPOSE 8080
ENTRYPOINT ["/app/server"]
