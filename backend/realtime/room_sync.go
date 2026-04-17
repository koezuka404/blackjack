package realtime

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

// roomSyncChannel は全インスタンスで共有する Pub/Sub チャンネル名（Phase 3 / §13.3 補足）。
const roomSyncChannel = "blackjack:room:state_sync"

// RoomSyncMessage はルーム状態同期の Pub/Sub ペイロード。
type RoomSyncMessage struct {
	RoomID    string `json:"room_id"`
	EventType string `json:"event_type"`
	Origin    string `json:"origin"`
}

// RoomSyncBroker は複数インスタンス間で ROOM_STATE_SYNC 相当の通知を Redis Pub/Sub で伝える。
type RoomSyncBroker struct {
	rdb      *redis.Client
	serverID string
}

// NewRoomSyncBroker は broker を生成する。rdb が nil のとき Publish / Subscribe は no-op に近い動作。
func NewRoomSyncBroker(rdb *redis.Client, serverID string) *RoomSyncBroker {
	if serverID == "" {
		serverID = "unknown"
	}
	return &RoomSyncBroker{rdb: rdb, serverID: serverID}
}

// Publish は自インスタンスで DB コミット済みのあと、他インスタンス向けに同期イベントを発行する。
func (b *RoomSyncBroker) Publish(ctx context.Context, roomID, eventType string) error {
	if b == nil || b.rdb == nil || roomID == "" {
		return nil
	}
	m := RoomSyncMessage{RoomID: roomID, EventType: eventType, Origin: b.serverID}
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return b.rdb.Publish(ctx, roomSyncChannel, data).Err()
}

// RunSubscriber は Redis を購読し、他インスタンス由来のメッセージのみ onRemoteSync を呼ぶ（同一 origin は無視）。
func (b *RoomSyncBroker) RunSubscriber(ctx context.Context, onRemoteSync func(ctx context.Context, roomID, eventType string)) error {
	if b == nil || b.rdb == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	sub := b.rdb.Subscribe(ctx, roomSyncChannel)
	defer func() { _ = sub.Close() }()
	ch := sub.Channel()
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
