package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"blackjack/backend/model"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"
)

type roomUsecaseControllerStub struct {
	createRoomFn        func(context.Context, string) (*model.Room, error)
	joinRoomFn          func(context.Context, string, string) (*model.Room, error)
	getRoomFn           func(context.Context, string, string) (*model.Room, *model.GameSession, error)
	listRoomsFn         func(context.Context, string) ([]*model.Room, error)
	getRoomHistoryFn    func(context.Context, string, string) ([]*model.RoundLog, error)
	startRoomFn         func(context.Context, string, string) (*model.Room, *model.GameSession, error)
	hitFn               func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error)
	standFn             func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error)
	suggestPlayerAction func(context.Context, string, string) (*usecase.PlayHint, error)
	resetRoomFn         func(context.Context, string, string) (*model.Room, error)
	leaveRoomFn         func(context.Context, string, string) (*model.Room, *usecase.HostTransfer, error)
	voteRematchFn       func(context.Context, string, string, bool, int64, string) (*model.Room, *model.GameSession, error)
}

func (s roomUsecaseControllerStub) CreateRoom(ctx context.Context, hostUserID string) (*model.Room, error) {
	if s.createRoomFn != nil {
		return s.createRoomFn(ctx, hostUserID)
	}
	return nil, nil
}
func (s roomUsecaseControllerStub) JoinRoom(ctx context.Context, roomID, userID string) (*model.Room, error) {
	if s.joinRoomFn != nil {
		return s.joinRoomFn(ctx, roomID, userID)
	}
	return nil, nil
}
func (s roomUsecaseControllerStub) GetRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error) {
	if s.getRoomFn != nil {
		return s.getRoomFn(ctx, roomID, userID)
	}
	return nil, nil, nil
}
func (s roomUsecaseControllerStub) GetRoomState(context.Context, string, string) (*usecase.RoomState, error) {
	return nil, nil
}
func (s roomUsecaseControllerStub) ListRooms(ctx context.Context, userID string) ([]*model.Room, error) {
	if s.listRoomsFn != nil {
		return s.listRoomsFn(ctx, userID)
	}
	return nil, nil
}
func (s roomUsecaseControllerStub) GetRoomHistory(ctx context.Context, roomID, userID string) ([]*model.RoundLog, error) {
	if s.getRoomHistoryFn != nil {
		return s.getRoomHistoryFn(ctx, roomID, userID)
	}
	return nil, nil
}
func (s roomUsecaseControllerStub) LeaveRoom(ctx context.Context, roomID, userID string) (*model.Room, *usecase.HostTransfer, error) {
	if s.leaveRoomFn != nil {
		return s.leaveRoomFn(ctx, roomID, userID)
	}
	return nil, nil, nil
}
func (s roomUsecaseControllerStub) StartRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error) {
	if s.startRoomFn != nil {
		return s.startRoomFn(ctx, roomID, userID)
	}
	return nil, nil, nil
}
func (s roomUsecaseControllerStub) Hit(ctx context.Context, roomID, userID string, expectedVersion int64, actionID string) (*model.Room, *model.GameSession, error) {
	if s.hitFn != nil {
		return s.hitFn(ctx, roomID, userID, expectedVersion, actionID)
	}
	return nil, nil, nil
}
func (s roomUsecaseControllerStub) Stand(ctx context.Context, roomID, userID string, expectedVersion int64, actionID string) (*model.Room, *model.GameSession, error) {
	if s.standFn != nil {
		return s.standFn(ctx, roomID, userID, expectedVersion, actionID)
	}
	return nil, nil, nil
}
func (s roomUsecaseControllerStub) VoteRematch(ctx context.Context, roomID, userID string, agree bool, expectedVersion int64, actionID string) (*model.Room, *model.GameSession, error) {
	if s.voteRematchFn != nil {
		return s.voteRematchFn(ctx, roomID, userID, agree, expectedVersion, actionID)
	}
	return nil, nil, nil
}
func (s roomUsecaseControllerStub) MarkConnected(context.Context, string, string) error { return nil }
func (s roomUsecaseControllerStub) MarkDisconnected(context.Context, string, string) error {
	return nil
}
func (s roomUsecaseControllerStub) AutoStandDueSessions(context.Context) ([]string, error) {
	return nil, nil
}
func (s roomUsecaseControllerStub) SuggestPlayerAction(ctx context.Context, roomID, userID string) (*usecase.PlayHint, error) {
	if s.suggestPlayerAction != nil {
		return s.suggestPlayerAction(ctx, roomID, userID)
	}
	return nil, nil
}
func (s roomUsecaseControllerStub) ResetRoomForDebug(ctx context.Context, roomID, userID string) (*model.Room, error) {
	if s.resetRoomFn != nil {
		return s.resetRoomFn(ctx, roomID, userID)
	}
	return nil, nil
}

func newRoomControllerContext(t *testing.T, method, target, routePath string, body any, roomID, userID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal failed: %v", err)
		}
		payload = b
	}
	req := httptest.NewRequest(method, target, bytes.NewReader(payload))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if routePath != "" {
		c.SetPath(routePath)
	}
	if roomID != "" {
		c.SetParamNames("id")
		c.SetParamValues(roomID)
	}
	if userID != "" {
		c.Set("user_id", userID)
	}
	return c, rec
}

func TestRoomController_StartRoom_StatusBranches(t *testing.T) {
	errCases := []struct {
		name string
		err  error
		want int
	}{
		{"unauthorized", usecase.ErrUnauthorizedUser, http.StatusUnauthorized},
		{"forbidden", usecase.ErrForbiddenAction, http.StatusForbidden},
		{"invalid_input", usecase.ErrInvalidInput, http.StatusBadRequest},
		{"invalid_state", usecase.ErrInvalidGameState, http.StatusConflict},
		{"not_found", repository.ErrNotFound, http.StatusNotFound},
		{"internal", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range errCases {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := NewRoomController(roomUsecaseControllerStub{
				startRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
					return nil, nil, tt.err
				},
			}, nil, nil, nil)
			c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/start", "/api/rooms/:id/start", nil, "r1", "u1")
			_ = ctrl.StartRoom(c)
			if rec.Code != tt.want {
				t.Fatalf("unexpected status: got=%d want=%d", rec.Code, tt.want)
			}
		})
	}

	t.Run("success 200", func(t *testing.T) {
		ctrl := NewRoomController(roomUsecaseControllerStub{
			startRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
				now := time.Now().UTC()
				return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CreatedAt: now, UpdatedAt: now}, &model.GameSession{
					ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1, Deck: []model.StoredCard{}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
				}, nil
			},
		}, nil, nil, nil)
		c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/start", "/api/rooms/:id/start", nil, "r1", "u1")
		_ = ctrl.StartRoom(c)
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})
}

func TestRoomController_GetPlayHint_StatusBranches(t *testing.T) {
	errCases := []struct {
		name string
		err  error
		want int
	}{
		{"unauthorized", usecase.ErrUnauthorizedUser, http.StatusUnauthorized},
		{"forbidden", usecase.ErrForbiddenAction, http.StatusForbidden},
		{"invalid_input", usecase.ErrInvalidInput, http.StatusBadRequest},
		{"invalid_state", usecase.ErrInvalidGameState, http.StatusConflict},
		{"not_found", repository.ErrNotFound, http.StatusNotFound},
		{"internal", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range errCases {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := NewRoomController(roomUsecaseControllerStub{
				suggestPlayerAction: func(context.Context, string, string) (*usecase.PlayHint, error) {
					return nil, tt.err
				},
			}, nil, nil, nil)
			c, rec := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1/play_hint", "/api/rooms/:id/play_hint", nil, "r1", "u1")
			_ = ctrl.GetPlayHint(c)
			if rec.Code != tt.want {
				t.Fatalf("unexpected status: got=%d want=%d", rec.Code, tt.want)
			}
		})
	}

	t.Run("success 200", func(t *testing.T) {
		ctrl := NewRoomController(roomUsecaseControllerStub{
			suggestPlayerAction: func(context.Context, string, string) (*usecase.PlayHint, error) {
				return &usecase.PlayHint{Recommendation: "HIT", SessionVersion: 1, Rationale: "test"}, nil
			},
		}, nil, nil, nil)
		c, rec := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1/play_hint", "/api/rooms/:id/play_hint", nil, "r1", "u1")
		_ = ctrl.GetPlayHint(c)
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})
}

func TestRoomController_HitAndResetInputValidation(t *testing.T) {
	ctrl := NewRoomController(roomUsecaseControllerStub{}, nil, nil, nil)

	c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/hit", "/api/rooms/:id/hit", map[string]any{
		"action_id":        "a1",
		"expected_version": 0,
	}, "r1", "u1")
	_ = ctrl.Hit(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected hit status: %d", rec.Code)
	}

	t.Setenv("BLACKJACK_DEBUG_ROOM_RESET", "false")
	c2, rec2 := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/reset", "/api/rooms/:id/reset", nil, "r1", "u1")
	_ = ctrl.ResetRoomDebug(c2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("unexpected reset status: %d", rec2.Code)
	}
}

func TestRoomController_CreateJoinLeaveGetListHistory_StatusBranches(t *testing.T) {
	now := time.Now().UTC()

	t.Run("create room unauthorized and success", func(t *testing.T) {
		ctrl := NewRoomController(roomUsecaseControllerStub{
			createRoomFn: func(context.Context, string) (*model.Room, error) {
				return nil, usecase.ErrUnauthorizedUser
			},
		}, nil, nil, nil)
		c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms", "/api/rooms", nil, "", "u1")
		_ = ctrl.CreateRoom(c)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d", rec.Code)
		}

		ctrl2 := NewRoomController(roomUsecaseControllerStub{
			createRoomFn: func(context.Context, string) (*model.Room, error) {
				return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}, nil
			},
		}, nil, nil, nil)
		c2, rec2 := newRoomControllerContext(t, http.MethodPost, "/api/rooms", "/api/rooms", nil, "", "u1")
		_ = ctrl2.CreateRoom(c2)
		if rec2.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec2.Code)
		}
	})

	t.Run("join room status matrix", func(t *testing.T) {
		cases := []struct {
			name string
			err  error
			want int
		}{
			{"unauthorized", usecase.ErrUnauthorizedUser, http.StatusUnauthorized},
			{"forbidden", usecase.ErrForbiddenAction, http.StatusForbidden},
			{"room_full", model.ErrRoomFull, http.StatusConflict},
			{"invalid_state", usecase.ErrInvalidGameState, http.StatusConflict},
			{"invalid_input", usecase.ErrInvalidInput, http.StatusBadRequest},
			{"internal", errors.New("boom"), http.StatusInternalServerError},
		}
		for _, tc := range cases {
			ctrl := NewRoomController(roomUsecaseControllerStub{
				joinRoomFn: func(context.Context, string, string) (*model.Room, error) {
					return nil, tc.err
				},
			}, nil, nil, nil)
			c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/join", "/api/rooms/:id/join", nil, "r1", "u1")
			_ = ctrl.JoinRoom(c)
			if rec.Code != tc.want {
				t.Fatalf("%s: got=%d want=%d", tc.name, rec.Code, tc.want)
			}
		}

		ctrl := NewRoomController(roomUsecaseControllerStub{
			joinRoomFn: func(context.Context, string, string) (*model.Room, error) {
				sid := "s1"
				return &model.Room{
					ID:               "r1",
					HostUserID:       "u1",
					Status:           model.RoomStatusReady,
					CurrentSessionID: &sid,
					CreatedAt:        now,
					UpdatedAt:        now,
				}, nil
			},
		}, nil, nil, nil)
		c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/join", "/api/rooms/:id/join", nil, "r1", "u1")
		_ = ctrl.JoinRoom(c)
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("leave room status matrix and success", func(t *testing.T) {
		cases := []struct {
			name string
			err  error
			want int
		}{
			{"unauthorized", usecase.ErrUnauthorizedUser, http.StatusUnauthorized},
			{"invalid_input", usecase.ErrInvalidInput, http.StatusBadRequest},
			{"invalid_state", usecase.ErrInvalidGameState, http.StatusConflict},
			{"not_found", repository.ErrNotFound, http.StatusNotFound},
			{"internal", errors.New("boom"), http.StatusInternalServerError},
		}
		for _, tc := range cases {
			ctrl := NewRoomController(roomUsecaseControllerStub{
				leaveRoomFn: func(context.Context, string, string) (*model.Room, *usecase.HostTransfer, error) {
					return nil, nil, tc.err
				},
			}, nil, nil, nil)
			c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/leave", "/api/rooms/:id/leave", nil, "r1", "u1")
			_ = ctrl.LeaveRoom(c)
			if rec.Code != tc.want {
				t.Fatalf("%s: got=%d want=%d", tc.name, rec.Code, tc.want)
			}
		}

		ctrl := NewRoomController(roomUsecaseControllerStub{
			leaveRoomFn: func(context.Context, string, string) (*model.Room, *usecase.HostTransfer, error) {
				return &model.Room{ID: "r1", HostUserID: "u2", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}, &usecase.HostTransfer{
					RoomID:             "r1",
					PreviousHostUserID: "u1",
					NewHostUserID:      "u2",
				}, nil
			},
		}, nil, nil, nil)
		c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/leave", "/api/rooms/:id/leave", nil, "r1", "u1")
		_ = ctrl.LeaveRoom(c)
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})

	t.Run("get room/list room/history status matrix", func(t *testing.T) {
		getErrCases := []struct {
			err  error
			want int
		}{
			{usecase.ErrUnauthorizedUser, http.StatusUnauthorized},
			{usecase.ErrForbiddenAction, http.StatusForbidden},
			{usecase.ErrInvalidInput, http.StatusBadRequest},
			{repository.ErrNotFound, http.StatusNotFound},
			{errors.New("boom"), http.StatusInternalServerError},
		}
		for _, tc := range getErrCases {
			ctrl := NewRoomController(roomUsecaseControllerStub{
				getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
					return nil, nil, tc.err
				},
			}, nil, nil, nil)
			c, rec := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1", "/api/rooms/:id", nil, "r1", "u1")
			_ = ctrl.GetRoom(c)
			if rec.Code != tc.want {
				t.Fatalf("get room: got=%d want=%d", rec.Code, tc.want)
			}
		}

		ctrlGetOK := NewRoomController(roomUsecaseControllerStub{
			getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
				return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CreatedAt: now, UpdatedAt: now},
					&model.GameSession{
						ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 1, TurnSeat: 1, Deck: []model.StoredCard{}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
					}, nil
			},
		}, nil, nil, nil)
		cOK, recOK := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1", "/api/rooms/:id", nil, "r1", "u1")
		_ = ctrlGetOK.GetRoom(cOK)
		if recOK.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", recOK.Code)
		}

		ctrlList := NewRoomController(roomUsecaseControllerStub{
			listRoomsFn: func(context.Context, string) ([]*model.Room, error) {
				return nil, usecase.ErrUnauthorizedUser
			},
		}, nil, nil, nil)
		cList, recList := newRoomControllerContext(t, http.MethodGet, "/api/rooms", "/api/rooms", nil, "", "u1")
		_ = ctrlList.ListRooms(cList)
		if recList.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected list status: %d", recList.Code)
		}

		ctrlListOK := NewRoomController(roomUsecaseControllerStub{
			listRoomsFn: func(context.Context, string) ([]*model.Room, error) {
				return []*model.Room{{ID: "r1", HostUserID: "u1", Status: model.RoomStatusReady, CreatedAt: now, UpdatedAt: now}}, nil
			},
		}, nil, nil, nil)
		cListOK, recListOK := newRoomControllerContext(t, http.MethodGet, "/api/rooms", "/api/rooms", nil, "", "u1")
		_ = ctrlListOK.ListRooms(cListOK)
		if recListOK.Code != http.StatusOK {
			t.Fatalf("unexpected list status: %d", recListOK.Code)
		}

		historyErrCases := []struct {
			err  error
			want int
		}{
			{usecase.ErrUnauthorizedUser, http.StatusUnauthorized},
			{usecase.ErrForbiddenAction, http.StatusForbidden},
			{usecase.ErrInvalidInput, http.StatusBadRequest},
			{repository.ErrNotFound, http.StatusNotFound},
			{errors.New("boom"), http.StatusInternalServerError},
		}
		for _, tc := range historyErrCases {
			ctrl := NewRoomController(roomUsecaseControllerStub{
				getRoomHistoryFn: func(context.Context, string, string) ([]*model.RoundLog, error) {
					return nil, tc.err
				},
			}, nil, nil, nil)
			c, rec := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1/history", "/api/rooms/:id/history", nil, "r1", "u1")
			_ = ctrl.GetRoomHistory(c)
			if rec.Code != tc.want {
				t.Fatalf("history: got=%d want=%d", rec.Code, tc.want)
			}
		}
	})
}

