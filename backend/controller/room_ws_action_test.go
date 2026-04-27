package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/model"
	"blackjack/backend/usecase"

	"github.com/gorilla/websocket"
)

func newWSConnPair(t *testing.T, onServer func(*websocket.Conn)) *websocket.Conn {
	t.Helper()
	up := websocket.Upgrader{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		onServer(conn)
	}))
	t.Cleanup(s.Close)
	url := "ws" + strings.TrimPrefix(s.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestHandleGameWSAction_Branches(t *testing.T) {
	now := time.Now().UTC()
	okRoom := &model.Room{ID: "r1", Status: model.RoomStatusPlaying}
	okSess := &model.GameSession{ID: "s1", Version: 2, UpdatedAt: now}

	tests := []struct {
		name string
		req  dto.WSActionRequest
		room roomUsecaseControllerStub
	}{
		{name: "invalid hit", req: dto.WSActionRequest{Type: dto.WSEventHit}},
		{name: "invalid stand", req: dto.WSActionRequest{Type: dto.WSEventStand}},
		{name: "invalid rematch", req: dto.WSActionRequest{Type: dto.WSEventRematchVote}},
		{name: "sync req", req: dto.WSActionRequest{Type: dto.WSEventRoomSyncReq}},
		{name: "ping", req: dto.WSActionRequest{Type: dto.WSEventPing}},
		{name: "unsupported", req: dto.WSActionRequest{Type: "UNKNOWN"}},
		{
			name: "hit error",
			req:  dto.WSActionRequest{Type: dto.WSEventHit, ActionID: "a1", ExpectedVersion: 1},
			room: roomUsecaseControllerStub{hitFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
				return nil, nil, model.ErrVersionConflict
			}},
		},
		{
			name: "stand success",
			req:  dto.WSActionRequest{Type: dto.WSEventStand, ActionID: "a1", ExpectedVersion: 1},
			room: roomUsecaseControllerStub{standFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
				return okRoom, okSess, nil
			}},
		},
		{
			name: "stand error",
			req:  dto.WSActionRequest{Type: dto.WSEventStand, ActionID: "a1", ExpectedVersion: 1},
			room: roomUsecaseControllerStub{standFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
				return nil, nil, usecase.ErrForbiddenAction
			}},
		},
		{
			name: "rematch success",
			req:  dto.WSActionRequest{Type: dto.WSEventRematchVote, ActionID: "a1", ExpectedVersion: 1, Agree: boolPtr(true)},
			room: roomUsecaseControllerStub{voteRematchFn: func(context.Context, string, string, bool, int64, string) (*model.Room, *model.GameSession, error) {
				return okRoom, okSess, nil
			}},
		},
		{
			name: "hit success",
			req:  dto.WSActionRequest{Type: dto.WSEventHit, ActionID: "a1", ExpectedVersion: 1},
			room: roomUsecaseControllerStub{hitFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
				return okRoom, okSess, nil
			}},
		},
		{
			name: "rematch error",
			req:  dto.WSActionRequest{Type: dto.WSEventRematchVote, ActionID: "a1", ExpectedVersion: 1, Agree: boolPtr(true)},
			room: roomUsecaseControllerStub{voteRematchFn: func(context.Context, string, string, bool, int64, string) (*model.Room, *model.GameSession, error) {
				return nil, nil, usecase.ErrForbiddenAction
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := NewRoomController(tt.room, nil, nil, nil)
			meta := wsConnMeta{userID: "u1", writeMu: &sync.Mutex{}}
			client := newWSConnPair(t, func(serverConn *websocket.Conn) {
				rc.handleGameWSAction(nil, tt.req, "r1", "u1", serverConn, meta, time.Now())
			})
			_ = client.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
			_, _, _ = client.ReadMessage()
		})
	}
}

func boolPtr(v bool) *bool { return &v }
