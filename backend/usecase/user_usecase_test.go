package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"blackjack/backend/model"
	"blackjack/backend/repository"

	"golang.org/x/crypto/bcrypt"
)

type authStoreStub struct {
	createUserFn         func(ctx context.Context, user *model.User) error
	getUserByUsernameFn  func(ctx context.Context, username string) (*model.User, error)
	getUserByIDFn        func(ctx context.Context, userID string) (*model.User, error)
	transactionFn        func(ctx context.Context, fn func(txStore repository.Store) error) error
	createActionLogFn    func(ctx context.Context, actionLog *model.ActionLog) error
	getActionLogByIDFn   func(ctx context.Context, sessionID, actorUserID, actionID string) (*model.ActionLog, error)
	createRoomFn         func(ctx context.Context, room *model.Room) error
	updateRoomFn         func(ctx context.Context, room *model.Room) error
	getRoomFn            func(ctx context.Context, id string) (*model.Room, error)
	listRoomsByUserIDFn  func(ctx context.Context, userID string) ([]*model.Room, error)
	createRoomPlayerFn   func(ctx context.Context, p *model.RoomPlayer) error
	updateRoomPlayerFn   func(ctx context.Context, p *model.RoomPlayer) error
	getRoomPlayerFn      func(ctx context.Context, roomID, userID string) (*model.RoomPlayer, error)
	listRoomPlayersFn    func(ctx context.Context, roomID string) ([]*model.RoomPlayer, error)
	getSessionFn         func(ctx context.Context, id string) (*model.GameSession, error)
	getSessionForUpdateFn func(ctx context.Context, id string) (*model.GameSession, error)
	getLatestSessionFn   func(ctx context.Context, roomID string) (*model.GameSession, error)
	updateSessionIfVersionFn func(ctx context.Context, sess *model.GameSession, expectedVersion int64) (bool, error)
	createSessionFn      func(ctx context.Context, sess *model.GameSession) error
	getPlayerStateFn     func(ctx context.Context, sessionID, userID string) (*model.PlayerState, error)
	updatePlayerStateFn  func(ctx context.Context, p *model.PlayerState) error
	createPlayerStateFn  func(ctx context.Context, p *model.PlayerState) error
	listPlayerStatesFn   func(ctx context.Context, sessionID string) ([]*model.PlayerState, error)
	getDealerStateFn     func(ctx context.Context, sessionID string) (*model.DealerState, error)
	updateDealerStateFn  func(ctx context.Context, d *model.DealerState) error
	createDealerStateFn  func(ctx context.Context, d *model.DealerState) error
	upsertRematchVoteFn  func(ctx context.Context, vote *model.RematchVote) error
	listRematchVotesFn   func(ctx context.Context, sessionID string) ([]*model.RematchVote, error)
	listSessionsByStatusAndDeadlineBeforeFn func(ctx context.Context, status model.SessionStatus, before time.Time) ([]*model.GameSession, error)
	listResettingSessionsDueByFn func(ctx context.Context, due time.Time) ([]*model.GameSession, error)
	listSessionsByStatusFn func(ctx context.Context, status model.SessionStatus) ([]*model.GameSession, error)
	deleteGameSessionsByRoomIDFn func(ctx context.Context, roomID string) error
	deleteRoomPlayersByRoomIDFn  func(ctx context.Context, roomID string) error
	createRoundLogFn     func(ctx context.Context, log *model.RoundLog) error
	listRoundLogsByRoomIDFn func(ctx context.Context, roomID string) ([]*model.RoundLog, error)
}

func (s *authStoreStub) CreateUser(ctx context.Context, user *model.User) error {
	if s.createUserFn != nil {
		return s.createUserFn(ctx, user)
	}
	return nil
}
func (s *authStoreStub) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	if s.getUserByUsernameFn != nil {
		return s.getUserByUsernameFn(ctx, username)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) GetUserByID(ctx context.Context, userID string) (*model.User, error) {
	if s.getUserByIDFn != nil {
		return s.getUserByIDFn(ctx, userID)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) Transaction(ctx context.Context, fn func(txStore repository.Store) error) error {
	if s.transactionFn != nil {
		return s.transactionFn(ctx, fn)
	}
	return fn(s)
}

// Unused Store methods (not needed for auth usecase tests).
func (s *authStoreStub) UpdateRoom(ctx context.Context, room *model.Room) error {
	if s.updateRoomFn != nil {
		return s.updateRoomFn(ctx, room)
	}
	return nil
}
func (s *authStoreStub) GetRoom(ctx context.Context, id string) (*model.Room, error) {
	if s.getRoomFn != nil {
		return s.getRoomFn(ctx, id)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) ListRoomsByUserID(ctx context.Context, userID string) ([]*model.Room, error) {
	if s.listRoomsByUserIDFn != nil {
		return s.listRoomsByUserIDFn(ctx, userID)
	}
	return nil, nil
}
func (s *authStoreStub) DeleteRoomPlayersByRoomID(ctx context.Context, roomID string) error {
	if s.deleteRoomPlayersByRoomIDFn != nil {
		return s.deleteRoomPlayersByRoomIDFn(ctx, roomID)
	}
	return nil
}
func (s *authStoreStub) CountRooms(context.Context) (int64, error)                { return 0, nil }
func (s *authStoreStub) CreateRoom(ctx context.Context, room *model.Room) error {
	if s.createRoomFn != nil {
		return s.createRoomFn(ctx, room)
	}
	return nil
}
func (s *authStoreStub) UpdateSession(context.Context, *model.GameSession) error  { return nil }
func (s *authStoreStub) UpdateSessionIfVersion(ctx context.Context, sess *model.GameSession, expectedVersion int64) (bool, error) {
	if s.updateSessionIfVersionFn != nil {
		return s.updateSessionIfVersionFn(ctx, sess, expectedVersion)
	}
	return false, nil
}
func (s *authStoreStub) GetSession(ctx context.Context, id string) (*model.GameSession, error) {
	if s.getSessionFn != nil {
		return s.getSessionFn(ctx, id)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) GetSessionForUpdate(ctx context.Context, id string) (*model.GameSession, error) {
	if s.getSessionForUpdateFn != nil {
		return s.getSessionForUpdateFn(ctx, id)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) GetLatestSessionByRoomID(ctx context.Context, roomID string) (*model.GameSession, error) {
	if s.getLatestSessionFn != nil {
		return s.getLatestSessionFn(ctx, roomID)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) ListSessionsByStatusAndDeadlineBefore(ctx context.Context, status model.SessionStatus, before time.Time) ([]*model.GameSession, error) {
	if s.listSessionsByStatusAndDeadlineBeforeFn != nil {
		return s.listSessionsByStatusAndDeadlineBeforeFn(ctx, status, before)
	}
	return nil, nil
}
func (s *authStoreStub) ListResettingSessionsDueBy(ctx context.Context, due time.Time) ([]*model.GameSession, error) {
	if s.listResettingSessionsDueByFn != nil {
		return s.listResettingSessionsDueByFn(ctx, due)
	}
	return nil, nil
}
func (s *authStoreStub) ListSessionsByStatus(ctx context.Context, status model.SessionStatus) ([]*model.GameSession, error) {
	if s.listSessionsByStatusFn != nil {
		return s.listSessionsByStatusFn(ctx, status)
	}
	return nil, nil
}
func (s *authStoreStub) DeleteGameSessionsByRoomID(ctx context.Context, roomID string) error {
	if s.deleteGameSessionsByRoomIDFn != nil {
		return s.deleteGameSessionsByRoomIDFn(ctx, roomID)
	}
	return nil
}
func (s *authStoreStub) CountSessions(context.Context) (int64, error)              { return 0, nil }
func (s *authStoreStub) CreateRoomPlayer(ctx context.Context, p *model.RoomPlayer) error {
	if s.createRoomPlayerFn != nil {
		return s.createRoomPlayerFn(ctx, p)
	}
	return nil
}
func (s *authStoreStub) UpdateRoomPlayer(ctx context.Context, p *model.RoomPlayer) error {
	if s.updateRoomPlayerFn != nil {
		return s.updateRoomPlayerFn(ctx, p)
	}
	return nil
}
func (s *authStoreStub) GetRoomPlayer(ctx context.Context, roomID, userID string) (*model.RoomPlayer, error) {
	if s.getRoomPlayerFn != nil {
		return s.getRoomPlayerFn(ctx, roomID, userID)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) ListRoomPlayersByRoomID(ctx context.Context, roomID string) ([]*model.RoomPlayer, error) {
	if s.listRoomPlayersFn != nil {
		return s.listRoomPlayersFn(ctx, roomID)
	}
	return nil, nil
}
func (s *authStoreStub) CreateSession(ctx context.Context, sess *model.GameSession) error {
	if s.createSessionFn != nil {
		return s.createSessionFn(ctx, sess)
	}
	return nil
}
func (s *authStoreStub) CreatePlayerState(ctx context.Context, p *model.PlayerState) error {
	if s.createPlayerStateFn != nil {
		return s.createPlayerStateFn(ctx, p)
	}
	return nil
}
func (s *authStoreStub) UpdatePlayerState(ctx context.Context, p *model.PlayerState) error {
	if s.updatePlayerStateFn != nil {
		return s.updatePlayerStateFn(ctx, p)
	}
	return nil
}
func (s *authStoreStub) GetPlayerState(ctx context.Context, sessionID, userID string) (*model.PlayerState, error) {
	if s.getPlayerStateFn != nil {
		return s.getPlayerStateFn(ctx, sessionID, userID)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) ListPlayerStatesBySessionID(ctx context.Context, sessionID string) ([]*model.PlayerState, error) {
	if s.listPlayerStatesFn != nil {
		return s.listPlayerStatesFn(ctx, sessionID)
	}
	return nil, nil
}
func (s *authStoreStub) CreateDealerState(ctx context.Context, d *model.DealerState) error {
	if s.createDealerStateFn != nil {
		return s.createDealerStateFn(ctx, d)
	}
	return nil
}
func (s *authStoreStub) UpdateDealerState(ctx context.Context, d *model.DealerState) error {
	if s.updateDealerStateFn != nil {
		return s.updateDealerStateFn(ctx, d)
	}
	return nil
}
func (s *authStoreStub) GetDealerState(ctx context.Context, sessionID string) (*model.DealerState, error) {
	if s.getDealerStateFn != nil {
		return s.getDealerStateFn(ctx, sessionID)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) UpsertSession(context.Context, *model.Session) error      { return nil }
func (s *authStoreStub) GetAuthSession(context.Context, string) (*model.Session, error) {
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) DeleteSession(context.Context, string) error          { return nil }
func (s *authStoreStub) DeleteSessionsByUserID(context.Context, string) error { return nil }
func (s *authStoreStub) DeleteExpiredSessions(context.Context) error          { return nil }
func (s *authStoreStub) CreateActionLog(ctx context.Context, actionLog *model.ActionLog) error {
	if s.createActionLogFn != nil {
		return s.createActionLogFn(ctx, actionLog)
	}
	return nil
}
func (s *authStoreStub) GetActionLogByActionID(ctx context.Context, sessionID, actorUserID, actionID string) (*model.ActionLog, error) {
	if s.getActionLogByIDFn != nil {
		return s.getActionLogByIDFn(ctx, sessionID, actorUserID, actionID)
	}
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) UpsertRematchVote(ctx context.Context, vote *model.RematchVote) error {
	if s.upsertRematchVoteFn != nil {
		return s.upsertRematchVoteFn(ctx, vote)
	}
	return nil
}
func (s *authStoreStub) ListRematchVotes(ctx context.Context, sessionID string) ([]*model.RematchVote, error) {
	if s.listRematchVotesFn != nil {
		return s.listRematchVotesFn(ctx, sessionID)
	}
	return nil, nil
}
func (s *authStoreStub) CreateRoundLog(ctx context.Context, log *model.RoundLog) error {
	if s.createRoundLogFn != nil {
		return s.createRoundLogFn(ctx, log)
	}
	return nil
}
func (s *authStoreStub) GetRoundLog(context.Context, string, int) (*model.RoundLog, error) {
	return nil, repository.ErrNotFound
}
func (s *authStoreStub) ListRoundLogsByRoomID(ctx context.Context, roomID string) ([]*model.RoundLog, error) {
	if s.listRoundLogsByRoomIDFn != nil {
		return s.listRoundLogsByRoomIDFn(ctx, roomID)
	}
	return nil, nil
}

func TestAuthUsecase_SignupValidation(t *testing.T) {
	uc := NewAuthUsecase(&authStoreStub{}, []byte("this-is-a-very-long-secret"))
	_, err := uc.Signup(context.Background(), "ab", "short")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAuthUsecase_SignupUsernameTaken(t *testing.T) {
	store := &authStoreStub{
		createUserFn: func(context.Context, *model.User) error {
			return repository.ErrAlreadyExists
		},
	}
	uc := NewAuthUsecase(store, []byte("this-is-a-very-long-secret"))
	_, err := uc.Signup(context.Background(), "validname", "password12")
	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
}

func TestAuthUsecase_SignupSuccess(t *testing.T) {
	uc := NewAuthUsecase(&authStoreStub{}, []byte("this-is-a-very-long-secret"))
	res, err := uc.Signup(context.Background(), "validname", "password12")
	if err != nil {
		t.Fatalf("signup failed: %v", err)
	}
	if res.SessionToken() == "" {
		t.Fatal("token must not be empty")
	}
	if res.ExpiresAt().IsZero() {
		t.Fatal("expiresAt must not be zero")
	}
	if res.User().Username != "validname" {
		t.Fatalf("unexpected username: %s", res.User().Username)
	}
}

func TestAuthUsecase_LoginAndMe(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password12"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	store := &authStoreStub{
		getUserByUsernameFn: func(context.Context, string) (*model.User, error) {
			return &model.User{ID: "user-1", Username: "validname", PasswordHash: string(hash)}, nil
		},
		getUserByIDFn: func(context.Context, string) (*model.User, error) {
			return &model.User{ID: "user-1", Username: "validname"}, nil
		},
	}
	uc := NewAuthUsecase(store, []byte("this-is-a-very-long-secret"))

	if _, err := uc.Login(context.Background(), "validname", "wrongpass"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized for wrong password, got %v", err)
	}

	loginRes, err := uc.Login(context.Background(), "validname", "password12")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if loginRes.SessionToken() == "" {
		t.Fatal("token must not be empty")
	}
	if loginRes.ExpiresAt().IsZero() {
		t.Fatal("expiresAt must not be zero")
	}

	if _, err := uc.Me(context.Background(), ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized for empty user id, got %v", err)
	}
	me, err := uc.Me(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("me failed: %v", err)
	}
	if me.ID != "user-1" {
		t.Fatalf("unexpected me id: %s", me.ID)
	}

	store.getUserByIDFn = func(context.Context, string) (*model.User, error) {
		return nil, repository.ErrNotFound
	}
	if _, err := uc.Me(context.Background(), "user-1"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized for repository error, got %v", err)
	}
}

func TestAuthUsecase_Logout_NoOp(t *testing.T) {
	uc := NewAuthUsecase(&authStoreStub{}, []byte("this-is-a-very-long-secret"))
	if err := uc.Logout(context.Background()); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestAuthUsecase_Signup_TransactionGenericError(t *testing.T) {
	store := &authStoreStub{
		createUserFn: func(context.Context, *model.User) error { return errors.New("db write failed") },
	}
	uc := NewAuthUsecase(store, []byte("this-is-a-very-long-secret"))
	_, err := uc.Signup(context.Background(), "goodname", "password12")
	if err == nil || err.Error() != "db write failed" {
		t.Fatalf("expected db write failed, got %v", err)
	}
}

func TestAuthUsecase_Signup_JWTSecretTooShort(t *testing.T) {
	uc := NewAuthUsecase(&authStoreStub{}, []byte("short"))
	_, err := uc.Signup(context.Background(), "goodname", "password12")
	if err == nil || !strings.Contains(err.Error(), "jwt secret") {
		t.Fatalf("expected jwt secret error, got %v", err)
	}
}

func TestAuthUsecase_Login_JWTSecretTooShort(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password12"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	store := &authStoreStub{
		getUserByUsernameFn: func(context.Context, string) (*model.User, error) {
			return &model.User{ID: "user-1", Username: "goodname", PasswordHash: string(hash)}, nil
		},
	}
	uc := NewAuthUsecase(store, []byte("short"))
	_, err = uc.Login(context.Background(), "goodname", "password12")
	if err == nil || !strings.Contains(err.Error(), "jwt secret") {
		t.Fatalf("expected jwt secret error, got %v", err)
	}
}

