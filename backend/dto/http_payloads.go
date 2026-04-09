package dto

import "time"

type GetRoomData struct {
	Room    RoomDetailJSON `json:"room"`
	Session *SessionJSON   `json:"session,omitempty"`
}

type StartRoomData struct {
	Room    RoomDetailJSON `json:"room"`
	Session SessionJSON    `json:"session"`
}

type TurnActionRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	ActionID        string `json:"action_id"`
}

type TurnActionData struct {
	Room    RoomDetailJSON `json:"room"`
	Session SessionJSON    `json:"session"`
}

type RematchVoteRequest struct {
	Agree           bool   `json:"agree"`
	ExpectedVersion int64  `json:"expected_version"`
	ActionID        string `json:"action_id"`
}

type ListRoomsData struct {
	Rooms []RoomDetailJSON `json:"rooms"`
}

type RoomHistoryItemJSON struct {
	SessionID     string `json:"session_id"`
	RoundNo       int    `json:"round_no"`
	ResultPayload string `json:"result_payload"`
	CreatedAt     string `json:"created_at"`
}

type RoomHistoryData struct {
	RoomID string                `json:"room_id"`
	Items  []RoomHistoryItemJSON `json:"items"`
}

func RoomHistoryItemFromDomain(itemID string, roundNo int, resultPayload string, createdAt time.Time) RoomHistoryItemJSON {
	return RoomHistoryItemJSON{
		SessionID:     itemID,
		RoundNo:       roundNo,
		ResultPayload: resultPayload,
		CreatedAt:     createdAt.UTC().Format(time.RFC3339),
	}
}
