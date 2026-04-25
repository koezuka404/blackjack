package realtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNewRoomSyncBroker_DefaultServerID(t *testing.T) {
	b := NewRoomSyncBroker(nil, "")
	if b == nil {
		t.Fatal("expected non-nil broker")
	}
	if b.serverID != "unknown" {
		t.Fatalf("unexpected default server id: %s", b.serverID)
	}
}

func TestPublish_NoOpCases(t *testing.T) {
	if err := (*RoomSyncBroker)(nil).Publish(context.Background(), "room1", "ROOM_STATE_SYNC"); err != nil {
		t.Fatalf("nil broker publish should be no-op: %v", err)
	}
	b := NewRoomSyncBroker(nil, "s1")
	if err := b.Publish(context.Background(), "", "ROOM_STATE_SYNC"); err != nil {
		t.Fatalf("empty room publish should be no-op: %v", err)
	}
	if err := b.Publish(context.Background(), "room1", "ROOM_STATE_SYNC"); err != nil {
		t.Fatalf("nil redis publish should be no-op: %v", err)
	}
}

func TestRunSubscriber_NoRedis(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := NewRoomSyncBroker(nil, "s1")
	err := b.RunSubscriber(ctx, func(context.Context, string, string) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got: %v", err)
	}
}

func TestRunSubscriber_NoRedisWaitsForCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := NewRoomSyncBroker(nil, "s1")

	done := make(chan error, 1)
	go func() {
		done <- b.RunSubscriber(ctx, func(context.Context, string, string) {})
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	err := <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got: %v", err)
	}
}

