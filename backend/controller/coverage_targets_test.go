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
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

func TestTargetCoverage_ResetRoomDebug_StartRoom_TurnAction_GetRoom(t *testing.T) {
	t.Run("ResetRoomDebug all branches", func(t *testing.T) {
		t.Setenv("BLACKJACK_DEBUG_ROOM_RESET", "true")
		now := time.Now().UTC()
		cases := []struct {
			name string
			err  error
			want int
		}{
			{"unauthorized", usecase.ErrUnauthorizedUser, http.StatusUnauthorized},
			{"forbidden", usecase.ErrForbiddenAction, http.StatusForbidden},
			{"invalid_input", usecase.ErrInvalidInput, http.StatusBadRequest},
			{"not_found", repository.ErrNotFound, http.StatusNotFound},
			{"internal", context.DeadlineExceeded, http.StatusInternalServerError},
		}
		for _, tc := range cases {
			ctrl := NewRoomController(roomUsecaseControllerStub{
				resetRoomFn: func(context.Context, string, string) (*model.Room, error) { return nil, tc.err },
			}, nil, nil, nil)
			c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/reset", "/api/rooms/:id/reset", nil, "r1", "u1")
			_ = ctrl.ResetRoomDebug(c)
			if rec.Code != tc.want {
				t.Fatalf("%s: got=%d want=%d", tc.name, rec.Code, tc.want)
			}
		}
		ctrlOK := NewRoomController(roomUsecaseControllerStub{
			resetRoomFn: func(context.Context, string, string) (*model.Room, error) {
				return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}, nil
			},
		}, nil, nil, nil)
		cOK, recOK := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/reset", "/api/rooms/:id/reset", nil, "r1", "u1")
		_ = ctrlOK.ResetRoomDebug(cOK)
		if recOK.Code != http.StatusOK {
			t.Fatalf("reset success got=%d", recOK.Code)
		}
	})

	t.Run("StartRoom all branches", func(t *testing.T) {
		now := time.Now().UTC()
		td := now.Add(time.Minute)
		rd := now.Add(2 * time.Minute)
		sess := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1, TurnDeadlineAt: &td, RematchDeadlineAt: &rd, CreatedAt: now, UpdatedAt: now}
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CreatedAt: now, UpdatedAt: now}
		cases := []struct {
			name string
			err  error
			want int
		}{
			{"unauthorized", usecase.ErrUnauthorizedUser, http.StatusUnauthorized},
			{"forbidden", usecase.ErrForbiddenAction, http.StatusForbidden},
			{"invalid_input", usecase.ErrInvalidInput, http.StatusBadRequest},
			{"invalid_state", usecase.ErrInvalidGameState, http.StatusConflict},
			{"not_found", repository.ErrNotFound, http.StatusNotFound},
			{"internal", context.DeadlineExceeded, http.StatusInternalServerError},
		}
		for _, tc := range cases {
			ctrl := NewRoomController(roomUsecaseControllerStub{
				startRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
					return nil, nil, tc.err
				},
			}, nil, nil, nil)
			c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/start", "/api/rooms/:id/start", nil, "r1", "u1")
			_ = ctrl.StartRoom(c)
			if rec.Code != tc.want {
				t.Fatalf("%s: got=%d want=%d", tc.name, rec.Code, tc.want)
			}
		}
		ctrlOK := NewRoomController(roomUsecaseControllerStub{
			startRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) { return room, sess, nil },
		}, nil, nil, nil)
		cOK, recOK := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/start", "/api/rooms/:id/start", nil, "r1", "u1")
		_ = ctrlOK.StartRoom(cOK)
		if recOK.Code != http.StatusOK {
			t.Fatalf("start success got=%d", recOK.Code)
		}


		branchErrs := []error{
			usecase.ErrUnauthorizedUser,
			usecase.ErrForbiddenAction,
			usecase.ErrInvalidInput,
			usecase.ErrInvalidGameState,
			repository.ErrNotFound,
			context.Canceled,
		}
		for _, e := range branchErrs {
			ctrlB := NewRoomController(roomUsecaseControllerStub{
				startRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) { return nil, nil, e },
			}, nil, nil, nil)
			c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/start", "/api/rooms/:id/start", nil, "r1", "u1")
			_ = ctrlB.StartRoom(c)
			if rec.Code == 0 {
				t.Fatal("start branch should always write response")
			}
		}
	})

	t.Run("turnAction all branches", func(t *testing.T) {
		now := time.Now().UTC()
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CreatedAt: now, UpdatedAt: now}
		td := now.Add(time.Minute)
		rd := now.Add(2 * time.Minute)
		sess := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 2, TurnSeat: 1, TurnDeadlineAt: &td, RematchDeadlineAt: &rd, CreatedAt: now, UpdatedAt: now}

		ctrl := NewRoomController(roomUsecaseControllerStub{}, nil, nil, nil)
		cBadV, recBadV := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/hit", "/api/rooms/:id/hit", map[string]any{"expected_version": 0, "action_id": "a1"}, "r1", "u1")
		_ = ctrl.Hit(cBadV)
		if recBadV.Code != http.StatusBadRequest {
			t.Fatalf("expected bad request for version, got=%d", recBadV.Code)
		}
		cBadA, recBadA := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/hit", "/api/rooms/:id/hit", map[string]any{"expected_version": 1, "action_id": ""}, "r1", "u1")
		_ = ctrl.Hit(cBadA)
		if recBadA.Code != http.StatusBadRequest {
			t.Fatalf("expected bad request for action id, got=%d", recBadA.Code)
		}
		e := echo.New()
		reqMalformed := httptest.NewRequest(http.MethodPost, "/api/rooms/r1/hit", strings.NewReader("{"))
		reqMalformed.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		recMalformed := httptest.NewRecorder()
		cMalformed := e.NewContext(reqMalformed, recMalformed)
		cMalformed.SetPath("/api/rooms/:id/hit")
		cMalformed.SetParamNames("id")
		cMalformed.SetParamValues("r1")
		cMalformed.Set("user_id", "u1")
		_ = ctrl.Hit(cMalformed)
		if recMalformed.Code != http.StatusBadRequest {
			t.Fatalf("malformed bind should be bad request, got=%d", recMalformed.Code)
		}

		ctrlStandErr := NewRoomController(roomUsecaseControllerStub{
			standFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
				return nil, nil, usecase.ErrInvalidGameState
			},
		}, nil, nil, nil)
		cSE, recSE := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/stand", "/api/rooms/:id/stand", map[string]any{"expected_version": 1, "action_id": "a1"}, "r1", "u1")
		_ = ctrlStandErr.Stand(cSE)
		if recSE.Code != http.StatusConflict {
			t.Fatalf("stand error status got=%d", recSE.Code)
		}

		ctrlHitOK := NewRoomController(roomUsecaseControllerStub{
			hitFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) { return room, sess, nil },
		}, nil, nil, nil)
		cHO, recHO := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/hit", "/api/rooms/:id/hit", map[string]any{"expected_version": 1, "action_id": "a1"}, "r1", "u1")
		_ = ctrlHitOK.Hit(cHO)
		if recHO.Code != http.StatusOK {
			t.Fatalf("hit success status got=%d", recHO.Code)
		}

		ctrlStandOK := NewRoomController(roomUsecaseControllerStub{
			standFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) { return room, sess, nil },
		}, nil, nil, nil)
		cSO, recSO := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/stand", "/api/rooms/:id/stand", map[string]any{"expected_version": 1, "action_id": "a1"}, "r1", "u1")
		_ = ctrlStandOK.Stand(cSO)
		if recSO.Code != http.StatusOK {
			t.Fatalf("stand success status got=%d", recSO.Code)
		}
	})

	t.Run("GetRoom nil and session branches", func(t *testing.T) {
		now := time.Now().UTC()
		room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}
		ctrlNilSess := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) { return room, nil, nil },
		}, nil, nil, nil)
		c1, rec1 := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1", "/api/rooms/:id", nil, "r1", "u1")
		_ = ctrlNilSess.GetRoom(c1)
		if rec1.Code != http.StatusOK {
			t.Fatalf("get room nil session status got=%d", rec1.Code)
		}

		ctrlErr := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) { return nil, nil, context.DeadlineExceeded },
		}, nil, nil, nil)
		c2, rec2 := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1", "/api/rooms/:id", nil, "r1", "u1")
		_ = ctrlErr.GetRoom(c2)
		if rec2.Code != http.StatusInternalServerError {
			t.Fatalf("get room internal status got=%d", rec2.Code)
		}

		td := now.Add(time.Minute)
		rd := now.Add(2 * time.Minute)
		sess := &model.GameSession{ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1, TurnDeadlineAt: &td, RematchDeadlineAt: &rd, CreatedAt: now, UpdatedAt: now}
		ctrlSess := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) { return room, sess, nil },
		}, nil, nil, nil)
		c3, rec3 := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1", "/api/rooms/:id", nil, "r1", "u1")
		_ = ctrlSess.GetRoom(c3)
		if rec3.Code != http.StatusOK {
			t.Fatalf("get room with session status got=%d", rec3.Code)
		}
	})
}

func TestTargetCoverage_RematchVoteTimeFormatter(t *testing.T) {
	now := time.Now().UTC()
	td := now.Add(time.Minute)
	rd := now.Add(2 * time.Minute)
	room := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CreatedAt: now, UpdatedAt: now}
	sess := &model.GameSession{
		ID:                "s1",
		RoomID:            "r1",
		RoundNo:           1,
		Status:            model.SessionStatusResetting,
		Version:           2,
		TurnSeat:          1,
		TurnDeadlineAt:    &td,
		RematchDeadlineAt: &rd,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	ctrl := NewRoomController(roomUsecaseControllerStub{
		voteRematchFn: func(context.Context, string, string, bool, int64, string) (*model.Room, *model.GameSession, error) {
			return room, sess, nil
		},
	}, nil, nil, nil)
	c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/rematch_vote", "/api/rooms/:id/rematch_vote", map[string]any{
		"agree": true, "action_id": "a1", "expected_version": 1,
	}, "r1", "u1")
	_ = ctrl.RematchVote(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("rematch vote success status got=%d", rec.Code)
	}
}

func TestTargetCoverage_BroadcastRoomStateLocal(t *testing.T) {
	globalRoomHub = &roomHub{rooms: map[string]map[*websocket.Conn]wsConnMeta{}, latest: map[string]*websocket.Conn{}}

	upgrader := websocket.Upgrader{}
	connCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		connCh <- c
	}))
	defer srv.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer client.Close()
	serverConn := <-connCh
	defer serverConn.Close()

	globalRoomHub.add("r1", serverConn, wsConnMeta{userID: "u1", writeMu: &sync.Mutex{}})
	ctrl := NewRoomController(roomUsecaseControllerStub{
		getRoomStateFn: func(context.Context, string, string) (*usecase.RoomState, error) {
			return &usecase.RoomState{
				Room:    &model.Room{ID: "r1", Status: model.RoomStatusPlaying},
				Session: &model.GameSession{ID: "s1", Status: model.SessionStatusPlayerTurn, Version: 1},
			}, nil
		},
	}, nil, nil, nil)

	ctrl.broadcastRoomStateLocal(context.Background(), "r1", "u1", dto.WSEventRoomSync)
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	_, _, _ = client.ReadMessage()


	_ = client.Close()
	time.Sleep(20 * time.Millisecond)
	ctrl.broadcastRoomStateLocal(context.Background(), "r1", "u1", dto.WSEventRoomSync)
}
