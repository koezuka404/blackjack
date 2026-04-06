FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/server /app/server
USER nobody
EXPOSE 8080
ENTRYPOINT ["/app/server"]
