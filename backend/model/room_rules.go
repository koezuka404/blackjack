package model

func CanJoinAsHumanPlayer(roomStatus RoomStatus) bool {
	return roomStatus == RoomStatusWaiting || roomStatus == RoomStatusReady
}

func AssertHostCanStart(room *Room, actorUserID string, hasOngoingSession bool) error {
	if room == nil {
		return ErrInvalidStatus
	}
	if room.HostUserID != actorUserID {
		return ErrForbiddenStart
	}
	if room.Status != RoomStatusReady {
		return ErrForbiddenStart
	}
	if hasOngoingSession {
		return ErrForbiddenStart
	}
	return nil
}
