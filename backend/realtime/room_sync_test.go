package realtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

func TestPublish_MarshalError(t *testing.T) {
	prev := roomSyncMarshalJSON
	t.Cleanup(func() { roomSyncMarshalJSON = prev })
	roomSyncMarshalJSON = func(any) ([]byte, error) {
		return nil, errors.New("marshal boom")
	}
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	b := NewRoomSyncBroker(rdb, "srv")
	if err := b.Publish(context.Background(), "room-1", "E"); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestPublish_WithRedis(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	b := NewRoomSyncBroker(rdb, "srv-a")
	if err := b.Publish(context.Background(), "room-1", "CUSTOM"); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestRunSubscriber_InvokesCallbackForRemoteOrigin(t *testing.T) {
	mr := miniredis.RunT(t)
	rdbSub := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rdbPub := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdbSub.Close()
	defer rdbPub.Close()

	brokerA := NewRoomSyncBroker(rdbSub, "server-a")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	got := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- brokerA.RunSubscriber(ctx, func(_ context.Context, roomID, ev string) {
			select {
			case got <- roomID + "|" + ev:
			default:
			}
		})
	}()

	time.Sleep(400 * time.Millisecond)
	brokerB := NewRoomSyncBroker(rdbPub, "server-b")
	if err := brokerB.Publish(ctx, "room-99", "MY_EVENT"); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case v := <-got:
		if v != "room-99|MY_EVENT" {
			t.Fatalf("callback payload: %s", v)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for subscriber callback")
	}

	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("subscriber exit: %v", err)
	}
}

func TestRunSubscriber_IgnoresSameOriginAndBadJSON(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	b := NewRoomSyncBroker(rdb, "same-srv")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	errCh := make(chan error, 1)
	go func() {
		errCh <- b.RunSubscriber(ctx, func(context.Context, string, string) {
			calls++
		})
	}()

	time.Sleep(20 * time.Millisecond)
	_ = rdb.Publish(ctx, roomSyncChannel, "{not-json").Err()
	_ = NewRoomSyncBroker(rdb, "same-srv").Publish(ctx, "r1", "E")
	time.Sleep(50 * time.Millisecond)
	if calls != 0 {
		t.Fatalf("expected no remote callbacks, got %d", calls)
	}

	cancel()
	<-errCh
}

func TestRunSubscriber_DefaultEventTypeWhenEmpty(t *testing.T) {
	mr := miniredis.RunT(t)
	rdbSub := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	rdbPub := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdbSub.Close()
	defer rdbPub.Close()

	bA := NewRoomSyncBroker(rdbSub, "a")
	ctx, cancel := context.WithCancel(context.Background())

	got := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- bA.RunSubscriber(ctx, func(_ context.Context, _, ev string) { got <- ev })
	}()

	time.Sleep(400 * time.Millisecond)
	payload := `{"room_id":"rx","event_type":"","origin":"b"}`
	if err := rdbPub.Publish(ctx, roomSyncChannel, payload).Err(); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case ev := <-got:
		if ev != "ROOM_STATE_SYNC" {
			t.Fatalf("want default event, got %q", ev)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout")
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("subscriber: %v", err)
	}
}

func TestRunSubscriber_HookChannelClosedReturnsNil(t *testing.T) {
	prev := roomSyncPubSubChannelFn
	t.Cleanup(func() { roomSyncPubSubChannelFn = prev })
	hookCh := make(chan *redis.Message)
	close(hookCh)
	roomSyncPubSubChannelFn = func(*redis.PubSub) <-chan *redis.Message { return hookCh }

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	b := NewRoomSyncBroker(rdb, "srv")
	err := b.RunSubscriber(context.Background(), func(context.Context, string, string) {
		t.Fatal("no callback")
	})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestRunSubscriber_HookNilThenValidMessage(t *testing.T) {
	prev := roomSyncPubSubChannelFn
	t.Cleanup(func() { roomSyncPubSubChannelFn = prev })
	hookCh := make(chan *redis.Message, 10)
	hookCh <- nil
	hookCh <- &redis.Message{Payload: `{"room_id":"r1","event_type":"Z","origin":"publisher"}`}
	close(hookCh)
	roomSyncPubSubChannelFn = func(*redis.PubSub) <-chan *redis.Message { return hookCh }

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	b := NewRoomSyncBroker(rdb, "subscriber")
	var gotRoom, gotEv string
	err := b.RunSubscriber(context.Background(), func(_ context.Context, roomID, ev string) {
		gotRoom, gotEv = roomID, ev
	})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if gotRoom != "r1" || gotEv != "Z" {
		t.Fatalf("callback got %q %q", gotRoom, gotEv)
	}
}

func TestRunSubscriber_HookEmptyRoomIDSkipped(t *testing.T) {
	prev := roomSyncPubSubChannelFn
	t.Cleanup(func() { roomSyncPubSubChannelFn = prev })
	hookCh := make(chan *redis.Message, 2)
	hookCh <- &redis.Message{Payload: `{"room_id":"","event_type":"Z","origin":"publisher"}`}
	close(hookCh)
	roomSyncPubSubChannelFn = func(*redis.PubSub) <-chan *redis.Message { return hookCh }

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()
	b := NewRoomSyncBroker(rdb, "subscriber")
	err := b.RunSubscriber(context.Background(), func(context.Context, string, string) {
		t.Fatal("no callback")
	})
	if err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}
