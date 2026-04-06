package model

import "testing"

func TestRoom_RecalculateStatus(t *testing.T) {
	t.Parallel()

	r, err := NewRoom("room-1", "user-1")
	if err != nil {
		t.Fatalf("NewRoom failed: %v", err)
	}

	if err := r.RecalculateStatus(0, false); err != nil {
		t.Fatalf("RecalculateStatus failed: %v", err)
	}
	if r.Status != RoomStatusWaiting {
		t.Fatalf("status mismatch: got=%s want=%s", r.Status, RoomStatusWaiting)
	}

	if err := r.RecalculateStatus(1, false); err != nil {
		t.Fatalf("RecalculateStatus failed: %v", err)
	}
	if r.Status != RoomStatusReady {
		t.Fatalf("status mismatch: got=%s want=%s", r.Status, RoomStatusReady)
	}

	if err := r.RecalculateStatus(1, true); err != nil {
		t.Fatalf("RecalculateStatus failed: %v", err)
	}
	if r.Status != RoomStatusPlaying {
		t.Fatalf("status mismatch: got=%s want=%s", r.Status, RoomStatusPlaying)
	}
}

func TestRoom_RecalculateStatus_RoomFull(t *testing.T) {
	t.Parallel()

	r, _ := NewRoom("room-1", "user-1")
	if err := r.RecalculateStatus(2, false); err == nil {
		t.Fatal("expected room_full error")
	}
}

func TestGameSession_TransitionAndVersion(t *testing.T) {
	t.Parallel()

	s, err := NewGameSession("sess-1", "room-1", 1)
	if err != nil {
		t.Fatalf("NewGameSession failed: %v", err)
	}

	if s.Version != 1 {
		t.Fatalf("version mismatch: got=%d want=1", s.Version)
	}

	if err := s.CheckVersion(1); err != nil {
		t.Fatalf("CheckVersion failed: %v", err)
	}
	if err := s.CheckVersion(2); err == nil {
		t.Fatal("expected version_conflict")
	}

	if err := s.TransitionTo(SessionStatusPlayerTurn); err != nil {
		t.Fatalf("TransitionTo player turn failed: %v", err)
	}
	if err := s.TransitionTo(SessionStatusDealerTurn); err != nil {
		t.Fatalf("TransitionTo dealer turn failed: %v", err)
	}
	if err := s.TransitionTo(SessionStatusResult); err != nil {
		t.Fatalf("TransitionTo result failed: %v", err)
	}
	if err := s.TransitionTo(SessionStatusResetting); err != nil {
		t.Fatalf("TransitionTo resetting failed: %v", err)
	}
	if err := s.TransitionTo(SessionStatusDealing); err != nil {
		t.Fatalf("TransitionTo dealing failed: %v", err)
	}

	s.IncrementVersion()
	if s.Version != 2 {
		t.Fatalf("version mismatch after increment: got=%d want=2", s.Version)
	}
}

func TestPlayerState_SeatAndActability(t *testing.T) {
	t.Parallel()

	if _, err := NewPlayerState("sess-1", "user-1", 2); err == nil {
		t.Fatal("expected invalid seat error")
	}

	p, err := NewPlayerState("sess-1", "user-1", 1)
	if err != nil {
		t.Fatalf("NewPlayerState failed: %v", err)
	}
	if !p.CanAct() {
		t.Fatal("new player should be actable")
	}

	if err := p.SetStatus(PlayerStatusStand); err != nil {
		t.Fatalf("SetStatus failed: %v", err)
	}
	if p.CanAct() {
		t.Fatal("stand player should not be actable")
	}
}
