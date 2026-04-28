package usecase

import (
	crand "crypto/rand"
	"encoding/json"
	"math/big"

	"blackjack/backend/model"
)


var (
	shuffleIntn = func(max *big.Int) (*big.Int, error) {
		return crand.Int(crand.Reader, max)
	}
	marshalGameJSON = json.Marshal

	newRoomForCreate     = model.NewRoom
	newRoomPlayerForJoin = model.NewRoomPlayer
	newGameSessionUC     = model.NewGameSession
	newDealerStateUC     = model.NewDealerState
	newPlayerStateUC     = model.NewPlayerState

	roomRecalculateStatus = func(r *model.Room, activeHumanPlayers int, hasActiveSession bool) error {
		return r.RecalculateStatus(activeHumanPlayers, hasActiveSession)
	}

	gameSessionTransition = func(s *model.GameSession, next model.SessionStatus) error {
		return s.TransitionTo(next)
	}

	playerStateSetStatus = func(p *model.PlayerState, st model.PlayerStatus) error {
		return p.SetStatus(st)
	}

	roomPlayerSetStatusUC = func(p *model.RoomPlayer, st model.RoomPlayerStatus) error {
		return p.SetStatus(st)
	}

	playerSetOutcomeUC = func(p *model.PlayerState, score int, o model.Outcome) error {
		return p.SetOutcome(score, o)
	}
)
