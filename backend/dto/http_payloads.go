package dto

type GetRoomData struct {
	Room    RoomJSON     `json:"room"`
	Session *SessionJSON `json:"session,omitempty"`
}
