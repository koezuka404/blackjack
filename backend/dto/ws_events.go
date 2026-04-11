package dto

const (
	WSEventHit         = "HIT"
	WSEventStand       = "STAND"
	WSEventRematchVote = "REMATCH_VOTE"
	WSEventRoomSyncReq = "ROOM_SYNC_REQUEST"
	WSEventPing        = "PING"
	WSEventPong        = "PONG"
	WSEventRoomSync    = "ROOM_STATE_SYNC"
	WSEventError       = "ERROR"
)

const (
	WSErrorInvalidInput    = "invalid_input"
	WSErrorUnauthorized    = "unauthorized"
	WSErrorForbidden       = "forbidden"
	WSErrorInvalidGame     = "invalid_game_state"
	WSErrorRoomFull        = "room_full"
	WSErrorVersionConflict = "version_conflict"
	WSErrorDuplicateAction = "duplicate_action"
	WSErrorNotFound        = "not_found"
	WSErrorRateLimited     = "rate_limited"
	WSErrorInternal        = "internal_error"
)

type WSActionRequest struct {
	Type            string `json:"type"`
	RequestID       string `json:"request_id,omitempty"`
	ActionID        string `json:"action_id,omitempty"`
	ExpectedVersion int64  `json:"expected_version,omitempty"`
	Agree           *bool  `json:"agree,omitempty"`
}

type WSErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type WSErrorEvent struct {
	Type  string      `json:"type"`
	Error WSErrorBody `json:"error"`
}

type WSRoomStateSyncEvent struct {
	Type string               `json:"type"`
	Data RoomStateSyncPayload `json:"data"`
}
