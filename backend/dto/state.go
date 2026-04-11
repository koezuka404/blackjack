package dto

type CardJSON struct {
	Rank string `json:"rank"`
	Suit string `json:"suit"`
}

type RoomJSON struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type SessionJSON struct {
	ID                string  `json:"id,omitempty"`
	Status            string  `json:"status,omitempty"`
	Version           int64   `json:"version,omitempty"`
	RoundNo           int     `json:"round_no,omitempty"`
	TurnSeat          int     `json:"turn_seat,omitempty"`
	TurnDeadlineAt    *string `json:"turn_deadline_at,omitempty"`
	RematchDeadlineAt *string `json:"rematch_deadline_at,omitempty"`
}

type DealerJSON struct {
	VisibleCards []string `json:"visible_cards"`
	Hidden       bool     `json:"hidden"`
	CardCount    int      `json:"card_count"`
}

type PlayerJSON struct {
	UserID     string   `json:"user_id"`
	SeatNo     int      `json:"seat_no"`
	Status     string   `json:"status"`
	IsMe       bool     `json:"is_me"`
	Hand       []string `json:"hand,omitempty"`
	CardCount  int      `json:"card_count"`
	Outcome    *string  `json:"outcome,omitempty"`
	FinalScore *int     `json:"final_score,omitempty"`
}

type MyActionsJSON struct {
	CanHit         bool `json:"can_hit"`
	CanStand       bool `json:"can_stand"`
	CanRematchVote bool `json:"can_rematch_vote"`
}

type RoomStateSyncJSON struct {
	Room      RoomJSON      `json:"room"`
	Session   SessionJSON   `json:"session"`
	Dealer    DealerJSON    `json:"dealer"`
	Players   []PlayerJSON  `json:"players"`
	MyActions MyActionsJSON `json:"my_actions"`
}

type RoomStateSyncSessionJSON struct {
	ID                *string `json:"id"`
	Status            *string `json:"status"`
	Version           *int64  `json:"version"`
	RoundNo           *int    `json:"round_no"`
	TurnSeat          *int    `json:"turn_seat"`
	TurnDeadlineAt    *string `json:"turn_deadline_at"`
	RematchDeadlineAt *string `json:"rematch_deadline_at"`
}

type RoomStateSyncPayload struct {
	Room      RoomJSON                 `json:"room"`
	Session   RoomStateSyncSessionJSON `json:"session"`
	Dealer    DealerJSON               `json:"dealer"`
	Players   []PlayerJSON             `json:"players"`
	MyActions MyActionsJSON            `json:"my_actions"`
}
