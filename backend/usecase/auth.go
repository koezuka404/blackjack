package usecase

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"blackjack/backend/model"
	"blackjack/backend/repository"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var ErrUnauthorized = errors.New("unauthorized")
var ErrInvalidInput = errors.New("invalid_input")
var ErrUsernameTaken = errors.New("username_taken")

type AuthResponse interface {
	SessionToken() string
	ExpiresAt() time.Time
	User() *model.User
}

type AuthUsecase interface {
	Signup(ctx context.Context, username, password string) (AuthResponse, error)
	Login(ctx context.Context, username, password string) (AuthResponse, error)
	Logout(ctx context.Context, sessionID string) error
	Me(ctx context.Context, sessionID string) (*model.User, error)
}

type authResponse struct {
	token     string
	expiresAt time.Time
	user      *model.User
}

func (r authResponse) SessionToken() string { return r.token }
func (r authResponse) ExpiresAt() time.Time { return r.expiresAt }
func (r authResponse) User() *model.User    { return r.user }

type authService struct {
	store      repository.Store
	sessionTTL time.Duration
}

func NewAuthUsecase(store repository.Store) AuthUsecase {
	return &authService{
		store:      store,
		sessionTTL: 24 * time.Hour,
	}
}

func (u *authService) Signup(ctx context.Context, username, password string) (AuthResponse, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 100 || len(password) < 8 {
		return nil, ErrInvalidInput
	}
	pwHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	user := &model.User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: string(pwHash),
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	token, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	expiresAt := now.Add(u.sessionTTL)
	sess := &model.Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}

	err = u.store.Transaction(ctx, func(tx repository.Store) error {
		if err := tx.CreateUser(ctx, user); err != nil {
			if err == repository.ErrAlreadyExists {
				return ErrUsernameTaken
			}
			return err
		}
		return tx.UpsertSession(ctx, sess)
	})
	if err != nil {
		return nil, err
	}
	_ = u.store.DeleteExpiredSessions(ctx)
	return authResponse{token: token, expiresAt: expiresAt, user: user}, nil
}

func (u *authService) Login(ctx context.Context, username, password string) (AuthResponse, error) {
	user, err := u.store.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrUnauthorized
	}
	token, err := generateSessionID()
	if err != nil {
		return nil, err
	}
	expiresAt := time.Now().UTC().Add(u.sessionTTL)
	sess := &model.Session{
		ID:        token,
		UserID:    user.ID,
		ExpiresAt: expiresAt,
		CreatedAt: time.Now().UTC(),
	}
	if err := u.store.UpsertSession(ctx, sess); err != nil {
		return nil, err
	}
	_ = u.store.DeleteExpiredSessions(ctx)
	return authResponse{token: token, expiresAt: expiresAt, user: user}, nil
}

func (u *authService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return u.store.DeleteSession(ctx, sessionID)
}

func (u *authService) Me(ctx context.Context, sessionID string) (*model.User, error) {
	if sessionID == "" {
		return nil, ErrUnauthorized
	}
	sess, err := u.store.GetAuthSession(ctx, sessionID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if sess.ExpiresAt.Before(time.Now().UTC()) {
		_ = u.store.DeleteSession(ctx, sessionID)
		return nil, ErrUnauthorized
	}
	user, err := u.store.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return user, nil
}

func generateSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
