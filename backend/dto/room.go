package dto

type RoomDetailJSON struct {
	ID         string `json:"id"`
	HostUserID string `json:"host_user_id"`
	Status     string `json:"status"`
}

type CreateRoomData struct {
	Room RoomDetailJSON `json:"room"`
}
