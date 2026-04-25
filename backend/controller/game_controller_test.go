package controller

import (
	"context"
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

func TestWriteTurnMutationError_StatusBranches(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "400 invalid_input", err: usecase.ErrInvalidInput, want: http.StatusBadRequest},
		{name: "401 unauthorized", err: usecase.ErrUnauthorizedUser, want: http.StatusUnauthorized},
		{name: "403 forbidden", err: usecase.ErrForbiddenAction, want: http.StatusForbidden},
		{name: "404 not_found", err: repository.ErrNotFound, want: http.StatusNotFound},
		{name: "409 invalid_game_state", err: usecase.ErrInvalidGameState, want: http.StatusConflict},
		{name: "409 version_conflict", err: model.ErrVersionConflict, want: http.StatusConflict},
		{name: "500 internal", err: errors.New("boom"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodPost, "/api/rooms/x/hit", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			if err := writeTurnMutationError(c, tt.err, "invalid payload", "state error"); err != nil {
				t.Fatalf("unexpected handler error: %v", err)
			}
			if rec.Code != tt.want {
				t.Fatalf("unexpected status: got=%d want=%d", rec.Code, tt.want)
			}
		})
	}
}

func TestTurnActionAndRematchVote_StatusBranches(t *testing.T) {
	now := time.Now().UTC()
	okRoom := &model.Room{ID: "r1", HostUserID: "u1", Status: model.RoomStatusPlaying, CreatedAt: now, UpdatedAt: now}
	okSession := &model.GameSession{
		ID: "s1", RoomID: "r1", RoundNo: 1, Status: model.SessionStatusPlayerTurn, Version: 2, TurnSeat: 1,
		Deck: []model.StoredCard{}, DrawIndex: 0, CreatedAt: now, UpdatedAt: now,
	}

	t.Run("hit and stand error mapping", func(t *testing.T) {
		cases := []struct {
			name string
			op   string
			err  error
			want int
		}{
			{"hit version conflict", "hit", model.ErrVersionConflict, http.StatusConflict},
			{"stand duplicate action", "stand", model.ErrDuplicateAction, http.StatusConflict},
			{"hit not found", "hit", repository.ErrNotFound, http.StatusNotFound},
		}
		for _, tc := range cases {
			stub := roomUsecaseControllerStub{
				hitFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
					if tc.op == "hit" {
						return nil, nil, tc.err
					}
					return okRoom, okSession, nil
				},
				standFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) {
					if tc.op == "stand" {
						return nil, nil, tc.err
					}
					return okRoom, okSession, nil
				},
			}
			ctrl := NewRoomController(stub, nil, nil, nil)
			body := map[string]any{"action_id": "a1", "expected_version": 1}

			if tc.op == "hit" {
				c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/hit", "/api/rooms/:id/hit", body, "r1", "u1")
				_ = ctrl.Hit(c)
				if rec.Code != tc.want {
					t.Fatalf("%s: got=%d want=%d", tc.name, rec.Code, tc.want)
				}
				continue
			}
			c, rec := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/stand", "/api/rooms/:id/stand", body, "r1", "u1")
			_ = ctrl.Stand(c)
			if rec.Code != tc.want {
				t.Fatalf("%s: got=%d want=%d", tc.name, rec.Code, tc.want)
			}
		}
	})

	t.Run("hit and stand success", func(t *testing.T) {
		ctrl := NewRoomController(roomUsecaseControllerStub{
			hitFn:   func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) { return okRoom, okSession, nil },
			standFn: func(context.Context, string, string, int64, string) (*model.Room, *model.GameSession, error) { return okRoom, okSession, nil },
		}, nil, nil, nil)

		body := map[string]any{"action_id": "a1", "expected_version": 1}
		cHit, recHit := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/hit", "/api/rooms/:id/hit", body, "r1", "u1")
		_ = ctrl.Hit(cHit)
		if recHit.Code != http.StatusOK {
			t.Fatalf("unexpected hit status: %d", recHit.Code)
		}

		cStand, recStand := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/stand", "/api/rooms/:id/stand", body, "r1", "u1")
		_ = ctrl.Stand(cStand)
		if recStand.Code != http.StatusOK {
			t.Fatalf("unexpected stand status: %d", recStand.Code)
		}
	})

	t.Run("rematch vote validation and status mapping", func(t *testing.T) {
		ctrl := NewRoomController(roomUsecaseControllerStub{}, nil, nil, nil)
		cBad, recBad := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/rematch_vote", "/api/rooms/:id/rematch_vote", map[string]any{
			"agree":            true,
			"expected_version": 0,
		}, "r1", "u1")
		_ = ctrl.RematchVote(cBad)
		if recBad.Code != http.StatusBadRequest {
			t.Fatalf("unexpected rematch bad status: %d", recBad.Code)
		}

		ctrlErr := NewRoomController(roomUsecaseControllerStub{
			voteRematchFn: func(context.Context, string, string, bool, int64, string) (*model.Room, *model.GameSession, error) {
				return nil, nil, model.ErrVersionConflict
			},
		}, nil, nil, nil)
		cErr, recErr := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/rematch_vote", "/api/rooms/:id/rematch_vote", map[string]any{
			"agree":            true,
			"action_id":        "rv1",
			"expected_version": 1,
		}, "r1", "u1")
		_ = ctrlErr.RematchVote(cErr)
		if recErr.Code != http.StatusConflict {
			t.Fatalf("unexpected rematch conflict status: %d", recErr.Code)
		}

		ctrlOK := NewRoomController(roomUsecaseControllerStub{
			voteRematchFn: func(context.Context, string, string, bool, int64, string) (*model.Room, *model.GameSession, error) {
				return okRoom, okSession, nil
			},
		}, nil, nil, nil)
		cOK, recOK := newRoomControllerContext(t, http.MethodPost, "/api/rooms/r1/rematch_vote", "/api/rooms/:id/rematch_vote", map[string]any{
			"agree":            true,
			"action_id":        "rv1",
			"expected_version": 1,
		}, "r1", "u1")
		_ = ctrlOK.RematchVote(cOK)
		if recOK.Code != http.StatusOK {
			t.Fatalf("unexpected rematch success status: %d", recOK.Code)
		}
	})
}

