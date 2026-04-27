package controller

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"blackjack/backend/dto"
	"blackjack/backend/model"
	"blackjack/backend/repository"
	"blackjack/backend/usecase"

	"github.com/labstack/echo/v4"

)

func TestAuthAndRoomRegisterAndLogout(t *testing.T) {
	e := echo.New()
	api := e.Group("/api")

	auth := NewAuthController(authUsecaseStub{
		logoutFn: func(context.Context) error { return errors.New("ignored") },
	})
	auth.Register(api)
	c, rec := newJSONContext(t, http.MethodPost, "/api/auth/logout", nil)
	if err := auth.Logout(c); err != nil {
		t.Fatalf("logout failed: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected logout status: %d", rec.Code)
	}

	room := NewRoomController(roomUsecaseControllerStub{}, nil, nil, nil)
	room.Register(api)
}

func TestMapWSErrorAndWSAuditNil(t *testing.T) {
	cases := []error{
		usecase.ErrUnauthorizedUser,
		usecase.ErrForbiddenAction,
		usecase.ErrInvalidInput,
		usecase.ErrInvalidGameState,
		model.ErrNotPlayerTurn,
		model.ErrNotYourTurn,
		model.ErrInvalidPlayerStatus,
		model.ErrRoomFull,
		model.ErrVersionConflict,
		model.ErrDuplicateAction,
		repository.ErrNotFound,
		errors.New("boom"),
	}
	for _, err := range cases {
		code, msg := mapWSError(err)
		if code == "" || msg == "" {
			t.Fatalf("unexpected empty ws error mapping for %v", err)
		}
	}
	logWSEvent(nil, dto.WSActionRequest{Type: dto.WSEventPing}, "r1", "u1", nil, nil, nil, time.Now(), "success", "", nil)
}

func TestRoomControllerResetAndAuthErrors(t *testing.T) {
	t.Setenv("BLACKJACK_DEBUG_ROOM_RESET", "true")
	ctrl := NewRoomController(roomUsecaseControllerStub{
		resetRoomFn: func(context.Context, string, string) (*model.Room, error) {
			return nil, usecase.ErrUnauthorizedUser
		},
	}, nil, nil, nil)
	c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/reset", "/api/rooms/:id/reset", nil, "r1", "u1")
	_ = ctrl.ResetRoomDebug(c)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected reset status: %d", rec.Code)
	}

	auth := NewAuthController(authUsecaseStub{
		signupFn: func(context.Context, string, string) (usecase.AuthResponse, error) {
			return nil, errors.New("internal")
		},
		loginFn: func(context.Context, string, string) (usecase.AuthResponse, error) {
			return nil, errors.New("internal")
		},
		meFn: func(context.Context, string) (*model.User, error) {
			return nil, errors.New("no")
		},
	})
	cLogin, recLogin := newJSONContext(t, http.MethodPost, "/api/auth/login", map[string]any{
		"username": "a", "password": "bbbbbbbb",
	})
	cSignup, recSignup := newJSONContext(t, http.MethodPost, "/api/auth/signup", map[string]any{
		"username": "alice", "password": "password12",
	})
	_ = auth.Signup(cSignup)
	if recSignup.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected signup status: %d", recSignup.Code)
	}
	_ = auth.Login(cLogin)
	if recLogin.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected login status: %d", recLogin.Code)
	}
	cMe, recMe := newJSONContext(t, http.MethodGet, "/api/me", nil)
	cMe.Set("user_id", "u1")
	_ = auth.Me(cMe)
	if recMe.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected me status: %d", recMe.Code)
	}
}

func TestGameControllerRemainingBranches(t *testing.T) {
	now := time.Now().UTC()
	ctrl := NewRoomController(roomUsecaseControllerStub{
		getRoomHistoryFn: func(context.Context, string, string) ([]*model.RoundLog, error) {
			return []*model.RoundLog{{SessionID: "s1", RoundNo: 1, ResultPayload: "{}", CreatedAt: now}}, nil
		},
		resetRoomFn: func(context.Context, string, string) (*model.Room, error) {
			return nil, usecase.ErrForbiddenAction
		},
	}, nil, nil, nil)
	cH, recH := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1/history", "/api/rooms/:id/history", nil, "r1", "u1")
	_ = ctrl.GetRoomHistory(cH)
	if recH.Code != http.StatusOK {
		t.Fatalf("unexpected history status: %d", recH.Code)
	}

	t.Setenv("BLACKJACK_DEBUG_ROOM_RESET", "true")
	cR, recR := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/reset", "/api/rooms/:id/reset", nil, "r1", "u1")
	_ = ctrl.ResetRoomDebug(cR)
	if recR.Code != http.StatusForbidden {
		t.Fatalf("unexpected reset branch status: %d", recR.Code)
	}
}

func TestMoreGameControllerBranches(t *testing.T) {
	now := time.Now().UTC()
	ctrl := NewRoomController(roomUsecaseControllerStub{
		startRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
			return nil, nil, repository.ErrNotFound
		},
		resetRoomFn: func(context.Context, string, string) (*model.Room, error) {
			return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}, nil
		},
		getRoomFn: func(context.Context, string, string) (*model.Room, *model.GameSession, error) {
			return &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now}, nil, nil
		},
	}, nil, nil, nil)

	cS, recS := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/start", "/api/rooms/:id/start", nil, "r1", "u1")
	_ = ctrl.StartRoom(cS)
	if recS.Code != http.StatusNotFound {
		t.Fatalf("unexpected start status: %d", recS.Code)
	}

	t.Setenv("BLACKJACK_DEBUG_ROOM_RESET", "true")
	cR, recR := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/reset", "/api/rooms/:id/reset", nil, "r1", "u1")
	_ = ctrl.ResetRoomDebug(cR)
	if recR.Code != http.StatusOK {
		t.Fatalf("unexpected reset success status: %d", recR.Code)
	}

	cG, recG := newRoomControllerContext(t, http.MethodGet, "/api/rooms/r1", "/api/rooms/:id", nil, "r1", "u1")
	_ = ctrl.GetRoom(cG)
	if recG.Code != http.StatusOK {
		t.Fatalf("unexpected get room status: %d", recG.Code)
	}
}

func TestRoomControllerCreateAndListInternal(t *testing.T) {
	ctrl := NewRoomController(roomUsecaseControllerStub{
		createRoomFn: func(context.Context, string) (*model.Room, error) { return nil, errors.New("boom") },
		listRoomsFn:  func(context.Context, string) ([]*model.Room, error) { return nil, errors.New("boom") },
	}, nil, nil, nil)
	c1, rec1 := newRoomControllerContext(t, http.MethodPost, "/api/rooms", "/api/rooms", nil, "", "u1")
	_ = ctrl.CreateRoom(c1)
	if rec1.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected create status: %d", rec1.Code)
	}
	c2, rec2 := newRoomControllerContext(t, http.MethodGet, "/api/rooms", "/api/rooms", nil, "", "u1")
	_ = ctrl.ListRooms(c2)
	if rec2.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected list status: %d", rec2.Code)
	}
}

