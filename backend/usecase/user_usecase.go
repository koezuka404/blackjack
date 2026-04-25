package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"blackjack/backend/jwtauth"
	"blackjack/backend/model"
	"blackjack/backend/repository"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var ErrUnauthorized = errors.New("unauthorized")
var ErrInvalidInput = errors.New("invalid_input")
var ErrUsernameTaken = errors.New("username_taken")

var signupHashPassword = bcrypt.GenerateFromPassword

type AuthResponse interface {
	SessionToken() string
	ExpiresAt() time.Time
	User() *model.User
}

type AuthUsecase interface {
	Signup(ctx context.Context, username, password string) (AuthResponse, error)
	Login(ctx context.Context, username, password string) (AuthResponse, error)
	Logout(ctx context.Context) error
	Me(ctx context.Context, userID string) (*model.User, error)
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
	jwtSecret  []byte
	sessionTTL time.Duration
}

// NewAuthUsecase は認証（JWT アクセストークン）を組み立てる。
func NewAuthUsecase(store repository.Store, jwtSecret []byte) AuthUsecase {
	return &authService{
		store:      store,
		jwtSecret:  jwtSecret,
		sessionTTL: 24 * time.Hour,
	}
}

// Signup はユーザーを作成し JWT を返す。
func (u *authService) Signup(ctx context.Context, username, password string) (AuthResponse, error) {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 100 || len(password) < 8 {
		return nil, ErrInvalidInput
	}
	pwHash, err := signupHashPassword([]byte(password), bcrypt.DefaultCost)
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

	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		return tx.CreateUser(ctx, user)
	}); err != nil {
		if err == repository.ErrAlreadyExists {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}

	token, exp, _, err := jwtauth.SignAccessToken(u.jwtSecret, user.ID, u.sessionTTL)
	if err != nil {
		return nil, err
	}
	return authResponse{token: token, expiresAt: exp, user: user}, nil
}

// Login は資格情報を検証し JWT を発行する（ステートレス。サーバー側セッション行は作らない）。
func (u *authService) Login(ctx context.Context, username, password string) (AuthResponse, error) {
	user, err := u.store.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, ErrUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrUnauthorized
	}
	token, exp, _, err := jwtauth.SignAccessToken(u.jwtSecret, user.ID, u.sessionTTL)
	if err != nil {
		return nil, err
	}
	return authResponse{token: token, expiresAt: exp, user: user}, nil
}

// Logout はステートレス JWT のためサーバー側では無効化しない（クライアントが破棄する）。
func (u *authService) Logout(ctx context.Context) error {
	_ = ctx
	return nil
}

// Me は user_id に紐づくユーザーを返す。
func (u *authService) Me(ctx context.Context, userID string) (*model.User, error) {
	if userID == "" {
		return nil, ErrUnauthorized
	}
	user, err := u.store.GetUserByID(ctx, userID)
	if err != nil {
		return nil, ErrUnauthorized
	}
	return user, nil
}
