package model

import (
	"errors"
	"testing"
	"time"
)

type evalStub struct{ value int; bust, soft, bj bool }

func (e evalStub) Value([]StoredCard) int        { return e.value }
func (e evalStub) IsBlackjack([]StoredCard) bool { return e.bj }
func (e evalStub) IsBust([]StoredCard) bool      { return e.bust }
func (e evalStub) IsSoft([]StoredCard) bool      { return e.soft }
func (e evalStub) HardValue([]StoredCard) int    { return e.value }

type evalDealerBustByLen struct{}

func (evalDealerBustByLen) Value([]StoredCard) int        { return 18 }
func (evalDealerBustByLen) IsBlackjack([]StoredCard) bool { return false }
func (evalDealerBustByLen) IsBust(hand []StoredCard) bool { return len(hand) == 1 }
func (evalDealerBustByLen) IsSoft([]StoredCard) bool      { return false }
func (evalDealerBustByLen) HardValue([]StoredCard) int    { return 18 }

type evalOutcomeMatrix struct {
	pBust bool
	dBust bool
	pBJ   bool
	dBJ   bool
	pVal  int
	dVal  int
}

func (e evalOutcomeMatrix) Value(hand []StoredCard) int {
	if len(hand) == 2 {
		return e.pVal
	}
	return e.dVal
}
func (e evalOutcomeMatrix) IsBlackjack(hand []StoredCard) bool {
	if len(hand) == 2 {
		return e.pBJ
	}
	return e.dBJ
}
func (e evalOutcomeMatrix) IsBust(hand []StoredCard) bool {
	if len(hand) == 2 {
		return e.pBust
	}
	return e.dBust
}
func (e evalOutcomeMatrix) IsSoft([]StoredCard) bool   { return false }
func (e evalOutcomeMatrix) HardValue(hand []StoredCard) int { return e.Value(hand) }

func TestCoverageFull_Model(t *testing.T) {
	t.Run("validators", func(t *testing.T) {
		if !RoomStatusWaiting.IsValid() || RoomStatus("x").IsValid() ||
			!SessionStatusResult.IsValid() || SessionStatus("x").IsValid() ||
			!PlayerStatusStand.IsValid() || PlayerStatus("x").IsValid() ||
			!OutcomeWin.IsValid() || Outcome("x").IsValid() ||
			!ActorTypeUser.IsValid() || ActorType("x").IsValid() {
			t.Fatal("type validators failed")
		}
	})

	t.Run("room session and deck", func(t *testing.T) {
		if _, err := NewRoom("", "u", time.Now()); err == nil {
			t.Fatal("expected room validation")
		}
		r, err := NewRoom("r1", "u1", time.Time{})
		if err != nil {
			t.Fatalf("new room: %v", err)
		}
		r.Touch(time.Time{})
		r.Touch(time.Now())
		if err := r.RecalculateStatus(-1, false); err == nil {
			t.Fatal("expected negative active error")
		}
		if err := r.RecalculateStatus(2, false); !errors.Is(err, ErrRoomFull) {
			t.Fatalf("expected room full: %v", err)
		}
		_ = r.RecalculateStatus(0, true)
		_ = r.RecalculateStatus(1, false)
		_ = r.RecalculateStatus(0, false)

		if _, err := NewGameSession("", "r", 1, time.Now()); err == nil {
			t.Fatal("expected session validation")
		}
		if _, err := NewGameSession("s", "r", 0, time.Now()); err == nil {
			t.Fatal("expected round validation")
		}
		s, err := NewGameSession("s1", "r1", 1, time.Time{})
		if err != nil {
			t.Fatalf("new session: %v", err)
		}
		s.Touch(time.Time{})
		s.Touch(time.Now())
		td := time.Now().UTC()
		s.SetTurnDeadline(&td)
		if err := s.CheckVersion(0); !errors.Is(err, ErrInvalidVersion) {
			t.Fatalf("expected invalid version: %v", err)
		}
		if err := s.CheckVersion(9); !errors.Is(err, ErrVersionConflict) {
			t.Fatalf("expected version conflict: %v", err)
		}
		if err := s.CheckVersion(1); err != nil {
			t.Fatalf("expected version ok: %v", err)
		}
		s.IncrementVersion()

		if err := s.TransitionTo(SessionStatusDealerTurn); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("expected invalid transition: %v", err)
		}
		_ = s.TransitionTo(SessionStatusPlayerTurn)
		if err := s.TransitionTo(SessionStatusResult); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("player turn invalid next should fail: %v", err)
		}
		_ = s.TransitionTo(SessionStatusDealerTurn)
		if err := s.TransitionTo(SessionStatusResetting); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("dealer turn invalid next should fail: %v", err)
		}
		_ = s.TransitionTo(SessionStatusResult)
		if err := s.TransitionTo(SessionStatusDealerTurn); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("result invalid next should fail: %v", err)
		}
		_ = s.TransitionTo(SessionStatusResetting)
		if err := s.TransitionTo(SessionStatusPlayerTurn); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("expected resetting invalid transition: %v", err)
		}
		s.Status = SessionStatus("X")
		if err := s.TransitionTo(SessionStatusResult); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("expected invalid status: %v", err)
		}
		s.Status = SessionStatusDealing
		if err := s.TransitionTo(SessionStatus("X")); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("expected invalid next status: %v", err)
		}

		deck := []StoredCard{{Rank: "A", Suit: "S"}}
		s.SetDeck(deck)
		deck[0].Rank = "K"
		if s.Deck[0].Rank != "A" {
			t.Fatal("set deck should clone")
		}
		if _, err := s.DrawCard(); err != nil {
			t.Fatalf("draw should succeed: %v", err)
		}
		if _, err := s.DrawCard(); !errors.Is(err, ErrDeckExhausted) {
			t.Fatalf("expected exhausted: %v", err)
		}
		s.DrawIndex = -1
		if _, err := s.DrawCard(); !errors.Is(err, ErrInvalidDeck) {
			t.Fatalf("expected invalid deck: %v", err)
		}
		if s.RemainingDeckCards() != 0 {
			t.Fatal("remaining cards should be zero for invalid index")
		}
		s.SetDeck(nil)
		if s.Deck != nil || s.DrawIndex != 0 {
			t.Fatal("set nil deck should clear")
		}
		s.SetDeck([]StoredCard{{Rank: "2"}, {Rank: "3"}})
		if s.RemainingDeckCards() != 2 {
			t.Fatal("remaining cards should count undrawn cards")
		}
	})

	t.Run("player dealer and rules", func(t *testing.T) {
		if _, err := NewPlayerState("", "u", 1); err == nil {
			t.Fatal("expected validation")
		}
		if _, err := NewPlayerState("s", "u", 2); !errors.Is(err, ErrInvalidSeat) {
			t.Fatalf("expected invalid seat: %v", err)
		}
		p, _ := NewPlayerState("s", "u", 1)
		p.AppendCard(StoredCard{Rank: "5"})
		if err := p.SetStatus(PlayerStatus("X")); !errors.Is(err, ErrInvalidPlayerStatus) {
			t.Fatalf("expected invalid status: %v", err)
		}
		_ = p.SetStatus(PlayerStatusStand)
		if p.CanAct() {
			t.Fatal("stand should not act")
		}
		if err := p.SetOutcome(20, Outcome("X")); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("expected invalid outcome: %v", err)
		}
		_ = p.SetOutcome(20, OutcomeWin)

		sess := &GameSession{Status: SessionStatusPlayerTurn, TurnSeat: 1}
		p.Status = PlayerStatusActive
		if err := p.AssertCanHitOrStand(nil, "u"); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("nil session: %v", err)
		}
		sess.Status = SessionStatusDealerTurn
		if err := p.AssertCanHitOrStand(sess, "u"); !errors.Is(err, ErrNotPlayerTurn) {
			t.Fatalf("expected not player turn: %v", err)
		}
		sess.Status = SessionStatusPlayerTurn
		sess.TurnSeat = 2
		if err := p.AssertCanHitOrStand(sess, "u"); !errors.Is(err, ErrNotYourTurn) {
			t.Fatalf("expected not your turn seat: %v", err)
		}
		sess.TurnSeat = 1
		if err := p.AssertCanHitOrStand(sess, "other"); !errors.Is(err, ErrNotYourTurn) {
			t.Fatalf("expected not your turn user: %v", err)
		}
		p.Status = PlayerStatusStand
		if err := p.AssertCanHitOrStand(sess, "u"); !errors.Is(err, ErrInvalidPlayerStatus) {
			t.Fatalf("expected invalid player status: %v", err)
		}
		p.Status = PlayerStatusActive
		if err := p.AssertCanHitOrStand(sess, "u"); err != nil {
			t.Fatalf("expected can act: %v", err)
		}

		if _, err := NewDealerState(""); err == nil {
			t.Fatal("expected dealer validation")
		}
		d, _ := NewDealerState("s")
		d.AppendCard(StoredCard{Rank: "9"})
		d.RevealHole()
		d.SetFinalScore(17)
		if d.HoleHidden || d.FinalScore == nil {
			t.Fatal("dealer mutators failed")
		}
	})

	t.Run("other entities and outcome", func(t *testing.T) {
		if err := (ActionLog{}).Validate(); err == nil {
			t.Fatal("expected invalid action log")
		}
		if err := (ActionLog{SessionID: "s", ActionID: "a", RequestType: "H", RequestPayloadHash: "x", ActorType: ActorType("X")}).Validate(); err == nil {
			t.Fatal("expected actor invalid")
		}
		if err := (ActionLog{SessionID: "s", ActionID: "a", RequestType: "H", RequestPayloadHash: "x", ActorType: ActorTypeUser}).Validate(); err == nil {
			t.Fatal("expected actor user id required")
		}
		if err := (ActionLog{SessionID: "s", ActionID: "a", RequestType: "H", RequestPayloadHash: "x", ActorType: ActorTypeSystem}).Validate(); err != nil {
			t.Fatalf("expected valid action log: %v", err)
		}
		if err := (RematchVote{}).Validate(); err == nil {
			t.Fatal("expected invalid rematch vote")
		}
		if err := (RematchVote{SessionID: "s", UserID: "u"}).Validate(); err != nil {
			t.Fatalf("expected valid rematch vote: %v", err)
		}
		if err := (RoundLog{}).Validate(); err == nil {
			t.Fatal("expected invalid roundlog")
		}
		if err := (RoundLog{SessionID: "s", RoundNo: 1, ResultPayload: "{}"}).Validate(); err != nil {
			t.Fatalf("expected valid roundlog: %v", err)
		}

		if !RoomPlayerActive.IsValid() || RoomPlayerStatus("X").IsValid() {
			t.Fatal("room player status validation failed")
		}
		if _, err := NewRoomPlayer("", "u", 1, time.Now()); err == nil {
			t.Fatal("expected room player validation")
		}
		if _, err := NewRoomPlayer("r", "u", 2, time.Now()); !errors.Is(err, ErrInvalidSeat) {
			t.Fatalf("expected invalid seat: %v", err)
		}
		if _, err := NewRoomPlayer("r", "u", 1, time.Time{}); err == nil {
			t.Fatal("expected joinedAt validation")
		}
		rp, _ := NewRoomPlayer("r", "u", 1, time.Now())
		if err := rp.SetStatus(RoomPlayerStatus("X")); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("expected invalid status: %v", err)
		}
		_ = rp.SetStatus(RoomPlayerDisconnected)
		rp.MarkLeft(time.Now())

		s := &GameSession{}
		s.SetRematchDeadline(time.Time{})
		now := time.Now()
		s.SetRematchDeadline(now)
		if s.RematchDeadlineAt == nil {
			t.Fatal("rematch deadline should be set")
		}
		if RematchUnanimous(nil, map[string]bool{}) {
			t.Fatal("empty eligible should be false")
		}
		if !RematchUnanimous([]string{"u1"}, map[string]bool{"u1": true}) {
			t.Fatal("single agree should be true")
		}
		if RematchUnanimous([]string{"u1", "u2"}, map[string]bool{"u1": true}) {
			t.Fatal("missing agreement should be false")
		}

		player := []StoredCard{{Rank: "10"}, {Rank: "A"}}
		dealer := []StoredCard{{Rank: "9"}, {Rank: "8"}}
		if out, _ := ResolveRoundOutcome(evalStub{bust: true}, player, dealer); out != OutcomeLose {
			t.Fatal("player bust should lose")
		}
		if out, _ := ResolveRoundOutcome(evalDealerBustByLen{}, player, dealer[:1]); out != OutcomeWin {
			t.Fatal("dealer bust should win")
		}
		if out, _ := ResolveRoundOutcome(evalStub{bj: true}, player, dealer); out != OutcomePush {
			t.Fatal("both blackjack should push")
		}
		if out, _ := ResolveRoundOutcome(evalStub{value: 20}, player, dealer); out != OutcomePush {
			t.Fatal("equal should push")
		}
		if out, _ := ResolveRoundOutcome(evalOutcomeMatrix{pBJ: true, dBJ: false}, player, dealer[:1]); out != OutcomeWin {
			t.Fatal("player blackjack should win")
		}
		if out, _ := ResolveRoundOutcome(evalOutcomeMatrix{pBJ: false, dBJ: true}, player, dealer[:1]); out != OutcomeLose {
			t.Fatal("dealer blackjack should lose")
		}
		if out, _ := ResolveRoundOutcome(evalOutcomeMatrix{pVal: 20, dVal: 18}, player, dealer[:1]); out != OutcomeWin {
			t.Fatal("higher player total should win")
		}
		if out, _ := ResolveRoundOutcome(evalOutcomeMatrix{pVal: 17, dVal: 19}, player, dealer[:1]); out != OutcomeLose {
			t.Fatal("lower player total should lose")
		}
		if !CanJoinAsHumanPlayer(RoomStatusReady) || CanJoinAsHumanPlayer(RoomStatusPlaying) {
			t.Fatal("join rule failed")
		}
		if err := AssertHostCanStart(nil, "u", false); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("expected invalid status: %v", err)
		}
		room := &Room{HostUserID: "h", Status: RoomStatusReady}
		if err := AssertHostCanStart(room, "x", false); !errors.Is(err, ErrForbiddenStart) {
			t.Fatalf("expected forbidden: %v", err)
		}
		room.Status = RoomStatusWaiting
		if err := AssertHostCanStart(room, "h", false); !errors.Is(err, ErrForbiddenStart) {
			t.Fatalf("expected forbidden for waiting room: %v", err)
		}
		room.Status = RoomStatusReady
		if err := AssertHostCanStart(room, "h", true); !errors.Is(err, ErrForbiddenStart) {
			t.Fatalf("expected forbidden for ongoing session: %v", err)
		}
		if err := AssertHostCanStart(room, "h", false); err != nil {
			t.Fatalf("expected host can start: %v", err)
		}

		if dealerUpValue(StoredCard{Rank: "A"}) != 11 || dealerUpValue(StoredCard{Rank: "K"}) != 10 ||
			dealerUpValue(StoredCard{Rank: "Q"}) != 10 || dealerUpValue(StoredCard{Rank: "J"}) != 10 ||
			dealerUpValue(StoredCard{Rank: "10"}) != 10 || dealerUpValue(StoredCard{Rank: "9"}) != 9 ||
			dealerUpValue(StoredCard{Rank: "8"}) != 8 || dealerUpValue(StoredCard{Rank: "7"}) != 7 ||
			dealerUpValue(StoredCard{Rank: "6"}) != 6 || dealerUpValue(StoredCard{Rank: "5"}) != 5 ||
			dealerUpValue(StoredCard{Rank: "4"}) != 4 || dealerUpValue(StoredCard{Rank: "3"}) != 3 ||
			dealerUpValue(StoredCard{Rank: "2"}) != 2 || dealerUpValue(StoredCard{Rank: "?"}) != 10 {
			t.Fatal("dealerUpValue failed")
		}
		if !softHitOrStand(evalStub{value: 17}, player, 5) || softHitOrStand(evalStub{value: 19}, player, 10) {
			t.Fatal("soft rule failed")
		}
		if hardHitOrStand(evalStub{value: 17}, player, 10) ||
			hardHitOrStand(evalStub{value: 13}, player, 2) ||
			!hardHitOrStand(evalStub{value: 12}, player, 2) ||
			!hardHitOrStand(evalStub{value: 11}, player, 10) ||
			!hardHitOrStand(evalStub{value: 10}, player, 10) ||
			!hardHitOrStand(evalStub{value: 9}, player, 6) {
			t.Fatal("hard rule failed")
		}
		if RecommendHitOrStand(evalStub{bust: true}, nil, StoredCard{Rank: "10"}) {
			t.Fatal("empty/bust should stand")
		}
		if !RecommendHitOrStand(evalStub{soft: true, value: 18}, player, StoredCard{Rank: "9"}) {
			t.Fatal("soft recommendation expected hit")
		}
		if !RecommendHitOrStand(evalStub{soft: false, value: 11}, player, StoredCard{Rank: "6"}) {
			t.Fatal("hard recommendation expected hit")
		}
	})
}
