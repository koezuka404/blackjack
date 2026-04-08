package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"strconv"
	"time"

	"blackjack/backend/model"
	"blackjack/backend/repository"

	"github.com/google/uuid"
)

var ErrUnauthorizedUser = errors.New("unauthorized")
var ErrInvalidGameState = errors.New("invalid_game_state")
var ErrForbiddenAction = errors.New("forbidden")

type RoomUsecase interface {
	CreateRoom(ctx context.Context, hostUserID string) (*model.Room, error)
	JoinRoom(ctx context.Context, roomID, userID string) (*model.Room, error)
	GetRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error)
	ListRooms(ctx context.Context, userID string) ([]*model.Room, error)
	GetRoomHistory(ctx context.Context, roomID, userID string) ([]*model.RoundLog, error)
	StartRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error)
	Hit(ctx context.Context, roomID, userID string, expectedVersion int64) (*model.Room, *model.GameSession, error)
	Stand(ctx context.Context, roomID, userID string, expectedVersion int64) (*model.Room, *model.GameSession, error)
}

type roomService struct {
	store repository.Store
}

func NewRoomUsecase(store repository.Store) RoomUsecase {
	return &roomService{store: store}
}

func (u *roomService) CreateRoom(ctx context.Context, hostUserID string) (*model.Room, error) {
	if hostUserID == "" {
		return nil, ErrUnauthorizedUser
	}
	now := time.Now().UTC()
	roomID := uuid.NewString()

	room, err := model.NewRoom(roomID, hostUserID, now)
	if err != nil {
		return nil, err
	}
	if err := room.RecalculateStatus(0, false); err != nil {
		return nil, err
	}
	room.Touch(now)

	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		return tx.CreateRoom(ctx, room)
	}); err != nil {
		return nil, err
	}
	return room, nil
}

func (u *roomService) JoinRoom(ctx context.Context, roomID, userID string) (*model.Room, error) {
	if userID == "" {
		return nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if !model.CanJoinAsHumanPlayer(room.Status) {
		return nil, ErrInvalidGameState
	}
	if room.HostUserID != userID {
		return nil, ErrForbiddenAction
	}
	players, err := u.store.ListRoomPlayersByRoomID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	activeHuman := 0
	for _, p := range players {
		if p.Status == model.RoomPlayerActive || p.Status == model.RoomPlayerDisconnected {
			activeHuman++
		}
	}
	if activeHuman >= 1 {
		return nil, model.ErrRoomFull
	}

	now := time.Now().UTC()
	joiner, err := model.NewRoomPlayer(roomID, userID, 1, now)
	if err != nil {
		return nil, err
	}
	if err := room.RecalculateStatus(activeHuman+1, room.CurrentSessionID != nil); err != nil {
		return nil, err
	}
	room.Touch(now)

	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		if err := tx.CreateRoomPlayer(ctx, joiner); err != nil {
			if err == repository.ErrAlreadyExists {
				return model.ErrRoomFull
			}
			return err
		}
		return tx.UpdateRoom(ctx, room)
	}); err != nil {
		return nil, err
	}
	return room, nil
}

func (u *roomService) GetRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error) {
	if userID == "" {
		return nil, nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	if room.HostUserID != userID {
		p, err := u.store.GetRoomPlayer(ctx, roomID, userID)
		if err != nil {
			if err == repository.ErrNotFound {
				return nil, nil, ErrForbiddenAction
			}
			return nil, nil, err
		}
		if p.Status == model.RoomPlayerLeft {
			return nil, nil, ErrForbiddenAction
		}
	}
	if room.CurrentSessionID == nil {
		return room, nil, nil
	}
	sess, err := u.store.GetSession(ctx, *room.CurrentSessionID)
	if err != nil {
		if err == repository.ErrNotFound {
			return room, nil, nil
		}
		return nil, nil, err
	}
	return room, sess, nil
}

func (u *roomService) ListRooms(ctx context.Context, userID string) ([]*model.Room, error) {
	if userID == "" {
		return nil, ErrUnauthorizedUser
	}
	return u.store.ListRoomsByUserID(ctx, userID)
}

func (u *roomService) GetRoomHistory(ctx context.Context, roomID, userID string) ([]*model.RoundLog, error) {
	if userID == "" {
		return nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if room.HostUserID != userID {
		p, err := u.store.GetRoomPlayer(ctx, roomID, userID)
		if err != nil {
			if err == repository.ErrNotFound {
				return nil, ErrForbiddenAction
			}
			return nil, err
		}
		if p.Status == model.RoomPlayerLeft {
			return nil, ErrForbiddenAction
		}
	}
	return u.store.ListRoundLogsByRoomID(ctx, roomID)
}

func (u *roomService) StartRoom(ctx context.Context, roomID, userID string) (*model.Room, *model.GameSession, error) {
	if userID == "" {
		return nil, nil, ErrUnauthorizedUser
	}
	if roomID == "" {
		return nil, nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	if room.HostUserID != userID {
		return nil, nil, ErrForbiddenAction
	}
	if room.Status != model.RoomStatusReady {
		return nil, nil, ErrInvalidGameState
	}
	player, err := u.store.GetRoomPlayer(ctx, roomID, userID)
	if err != nil {
		return nil, nil, err
	}
	if player.Status != model.RoomPlayerActive && player.Status != model.RoomPlayerDisconnected {
		return nil, nil, ErrInvalidGameState
	}
	if room.CurrentSessionID != nil {
		return nil, nil, ErrInvalidGameState
	}
	roundNo := 1
	latest, err := u.store.GetLatestSessionByRoomID(ctx, roomID)
	if err != nil && err != repository.ErrNotFound {
		return nil, nil, err
	}
	if latest != nil {
		roundNo = latest.RoundNo + 1
	}

	now := time.Now().UTC()
	sess, err := model.NewGameSession(uuid.NewString(), roomID, roundNo, now)
	if err != nil {
		return nil, nil, err
	}
	sess.SetDeck(newShuffledDeck(now.UnixNano()))
	dealer, err := model.NewDealerState(sess.ID)
	if err != nil {
		return nil, nil, err
	}
	pstate, err := model.NewPlayerState(sess.ID, userID, 1)
	if err != nil {
		return nil, nil, err
	}

	if err := initialDeal(sess, pstate, dealer); err != nil {
		return nil, nil, err
	}
	if err := sess.TransitionTo(model.SessionStatusPlayerTurn); err != nil {
		return nil, nil, err
	}

	needsSettle := isBlackjack(pstate.Hand)
	var roundLog *model.RoundLog
	if needsSettle {
		if err := pstate.SetStatus(model.PlayerStatusBlackjack); err != nil {
			return nil, nil, err
		}
		if err := sess.TransitionTo(model.SessionStatusDealerTurn); err != nil {
			return nil, nil, err
		}
		roundLog, err = settleDealerAndResult(sess, pstate, dealer, now)
		if err != nil {
			return nil, nil, err
		}
		room.CurrentSessionID = nil
	} else {
		room.CurrentSessionID = &sess.ID
	}
	if err := room.RecalculateStatus(1, room.CurrentSessionID != nil); err != nil {
		return nil, nil, err
	}
	room.Touch(now)
	sess.Touch(now)

	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		if err := tx.CreateSession(ctx, sess); err != nil {
			return err
		}
		if err := tx.CreatePlayerState(ctx, pstate); err != nil {
			return err
		}
		if err := tx.CreateDealerState(ctx, dealer); err != nil {
			return err
		}
		if roundLog != nil {
			if err := tx.CreateRoundLog(ctx, roundLog); err != nil {
				return err
			}
		}
		return tx.UpdateRoom(ctx, room)
	}); err != nil {
		return nil, nil, err
	}
	return room, sess, nil
}

func (u *roomService) Hit(ctx context.Context, roomID, userID string, expectedVersion int64) (*model.Room, *model.GameSession, error) {
	return u.playAction(ctx, roomID, userID, expectedVersion, true)
}

func (u *roomService) Stand(ctx context.Context, roomID, userID string, expectedVersion int64) (*model.Room, *model.GameSession, error) {
	return u.playAction(ctx, roomID, userID, expectedVersion, false)
}

func (u *roomService) playAction(ctx context.Context, roomID, userID string, expectedVersion int64, hit bool) (*model.Room, *model.GameSession, error) {
	if userID == "" {
		return nil, nil, ErrUnauthorizedUser
	}
	if roomID == "" || expectedVersion <= 0 {
		return nil, nil, ErrInvalidInput
	}
	room, err := u.store.GetRoom(ctx, roomID)
	if err != nil {
		return nil, nil, err
	}
	if room.CurrentSessionID == nil {
		return nil, nil, ErrInvalidGameState
	}
	sess, err := u.store.GetSession(ctx, *room.CurrentSessionID)
	if err != nil {
		return nil, nil, err
	}
	if err := sess.CheckVersion(expectedVersion); err != nil {
		return nil, nil, err
	}
	player, err := u.store.GetPlayerState(ctx, sess.ID, userID)
	if err != nil {
		if err == repository.ErrNotFound {
			return nil, nil, ErrForbiddenAction
		}
		return nil, nil, err
	}
	dealer, err := u.store.GetDealerState(ctx, sess.ID)
	if err != nil {
		return nil, nil, err
	}
	if err := player.AssertCanHitOrStand(sess, userID); err != nil {
		return nil, nil, err
	}

	now := time.Now().UTC()
	var roundLog *model.RoundLog
	if hit {
		card, err := sess.DrawCard()
		if err != nil {
			return nil, nil, err
		}
		player.AppendCard(card)
		v := handValue(player.Hand)
		if v > 21 {
			if err := player.SetStatus(model.PlayerStatusBust); err != nil {
				return nil, nil, err
			}
			if err := sess.TransitionTo(model.SessionStatusDealerTurn); err != nil {
				return nil, nil, err
			}
			roundLog, err = settleDealerAndResult(sess, player, dealer, now)
			if err != nil {
				return nil, nil, err
			}
		} else if isBlackjack(player.Hand) {
			if err := player.SetStatus(model.PlayerStatusBlackjack); err != nil {
				return nil, nil, err
			}
			if err := sess.TransitionTo(model.SessionStatusDealerTurn); err != nil {
				return nil, nil, err
			}
			roundLog, err = settleDealerAndResult(sess, player, dealer, now)
			if err != nil {
				return nil, nil, err
			}
		}
	} else {
		if err := player.SetStatus(model.PlayerStatusStand); err != nil {
			return nil, nil, err
		}
		if err := sess.TransitionTo(model.SessionStatusDealerTurn); err != nil {
			return nil, nil, err
		}
		roundLog, err = settleDealerAndResult(sess, player, dealer, now)
		if err != nil {
			return nil, nil, err
		}
	}

	if roundLog != nil {
		room.CurrentSessionID = nil
		if err := room.RecalculateStatus(1, false); err != nil {
			return nil, nil, err
		}
		room.Touch(now)
	}
	sess.IncrementVersion()
	sess.Touch(now)
	if err := u.store.Transaction(ctx, func(tx repository.Store) error {
		ok, err := tx.UpdateSessionIfVersion(ctx, sess, expectedVersion)
		if err != nil {
			return err
		}
		if !ok {
			return model.ErrVersionConflict
		}
		if err := tx.UpdatePlayerState(ctx, player); err != nil {
			return err
		}
		if err := tx.UpdateDealerState(ctx, dealer); err != nil {
			return err
		}
		if roundLog != nil {
			if err := tx.CreateRoundLog(ctx, roundLog); err != nil {
				return err
			}
			if err := tx.UpdateRoom(ctx, room); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return room, sess, nil
}

func initialDeal(sess *model.GameSession, p *model.PlayerState, d *model.DealerState) error {
	c1, err := sess.DrawCard()
	if err != nil {
		return err
	}
	p.AppendCard(c1)
	c2, err := sess.DrawCard()
	if err != nil {
		return err
	}
	d.AppendCard(c2)
	c3, err := sess.DrawCard()
	if err != nil {
		return err
	}
	p.AppendCard(c3)
	c4, err := sess.DrawCard()
	if err != nil {
		return err
	}
	d.AppendCard(c4)
	return nil
}

func settleDealerAndResult(sess *model.GameSession, p *model.PlayerState, d *model.DealerState, now time.Time) (*model.RoundLog, error) {
	d.RevealHole()
	for {
		action, terminal := model.NextDealerAction(simpleHandEvaluator{}, d.Hand)
		if terminal || action == model.DealerActionStand {
			break
		}
		card, err := sess.DrawCard()
		if err != nil {
			return nil, err
		}
		d.AppendCard(card)
	}
	if err := sess.TransitionTo(model.SessionStatusResult); err != nil {
		return nil, err
	}
	pScore := handValue(p.Hand)
	dScore := handValue(d.Hand)
	outcome, err := model.ResolveRoundOutcome(simpleHandEvaluator{}, p.Hand, d.Hand)
	if err != nil {
		return nil, err
	}
	if err := p.SetOutcome(pScore, outcome); err != nil {
		return nil, err
	}
	d.SetFinalScore(dScore)
	payloadBytes, err := json.Marshal(map[string]any{
		"player_score": pScore,
		"dealer_score": dScore,
		"outcome":      outcome,
	})
	if err != nil {
		return nil, err
	}
	return &model.RoundLog{
		SessionID:     sess.ID,
		RoundNo:       sess.RoundNo,
		ResultPayload: string(payloadBytes),
		CreatedAt:     now,
	}, nil
}

type simpleHandEvaluator struct{}

func (simpleHandEvaluator) Value(hand []model.StoredCard) int        { return handValue(hand) }
func (simpleHandEvaluator) IsBlackjack(hand []model.StoredCard) bool { return isBlackjack(hand) }
func (simpleHandEvaluator) IsBust(hand []model.StoredCard) bool      { return handValue(hand) > 21 }
func (simpleHandEvaluator) IsSoft(hand []model.StoredCard) bool      { return isSoftHand(hand) }

func handValue(hand []model.StoredCard) int {
	total := 0
	aces := 0
	for _, c := range hand {
		switch c.Rank {
		case "A":
			total += 11
			aces++
		case "K", "Q", "J", "10":
			total += 10
		default:
			n, err := strconv.Atoi(c.Rank)
			if err != nil || n < 2 || n > 9 {
				continue
			}
			total += n
		}
	}
	for total > 21 && aces > 0 {
		total -= 10
		aces--
	}
	return total
}

func isBlackjack(hand []model.StoredCard) bool {
	return len(hand) == 2 && handValue(hand) == 21
}

func isSoftHand(hand []model.StoredCard) bool {
	total := 0
	aces := 0
	for _, c := range hand {
		switch c.Rank {
		case "A":
			total += 11
			aces++
		case "K", "Q", "J", "10":
			total += 10
		default:
			n, err := strconv.Atoi(c.Rank)
			if err != nil || n < 2 || n > 9 {
				continue
			}
			total += n
		}
	}
	for total > 21 && aces > 0 {
		total -= 10
		aces--
	}
	return aces > 0
}

func newShuffledDeck(seed int64) []model.StoredCard {
	suits := []string{"S", "H", "D", "C"}
	ranks := []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	deck := make([]model.StoredCard, 0, 52)
	for _, s := range suits {
		for _, r := range ranks {
			deck = append(deck, model.StoredCard{Rank: r, Suit: s})
		}
	}
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(deck), func(i, j int) {
		deck[i], deck[j] = deck[j], deck[i]
	})
	return deck
}
