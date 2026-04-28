package realtime

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)


const roomSyncChannel = "blackjack:room:state_sync"


var roomSyncMarshalJSON = json.Marshal

func defaultPubSubChannel(sub *redis.PubSub) <-chan *redis.Message {
	return sub.Channel()
}


var roomSyncPubSubChannelFn = defaultPubSubChannel


type RoomSyncMessage struct {
	RoomID    string `json:"room_id"`
	EventType string `json:"event_type"`
	Origin    string `json:"origin"`
}


type RoomSyncBroker struct {
	rdb      *redis.Client
	serverID string
}


func NewRoomSyncBroker(rdb *redis.Client, serverID string) *RoomSyncBroker {
	if serverID == "" {
		serverID = "unknown"
	}
	return &RoomSyncBroker{rdb: rdb, serverID: serverID}
}


func (b *RoomSyncBroker) Publish(ctx context.Context, roomID, eventType string) error {
	if b == nil || b.rdb == nil || roomID == "" {
		return nil
	}
	m := RoomSyncMessage{RoomID: roomID, EventType: eventType, Origin: b.serverID}
	data, err := roomSyncMarshalJSON(m)
	if err != nil {
		return err
	}
	return b.rdb.Publish(ctx, roomSyncChannel, data).Err()
}


func (b *RoomSyncBroker) RunSubscriber(ctx context.Context, onRemoteSync func(ctx context.Context, roomID, eventType string)) error {
	if b == nil || b.rdb == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	sub := b.rdb.Subscribe(ctx, roomSyncChannel)
	defer func() { _ = sub.Close() }()
	ch := roomSyncPubSubChannelFn(sub)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			if msg == nil {
				continue
			}
			var m RoomSyncMessage
			if err := json.Unmarshal([]byte(msg.Payload), &m); err != nil {
				continue
			}
			if m.Origin == b.serverID || m.RoomID == "" {
				continue
			}
			ev := m.EventType
			if ev == "" {
				ev = "ROOM_STATE_SYNC"
			}
			onRemoteSync(ctx, m.RoomID, ev)
		}
	}
}
