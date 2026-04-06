package model

import (
	"fmt"
	"time"
)

type RoundLog struct {
	ID            string
	SessionID     string
	RoundNo       int
	ResultPayload string
	CreatedAt     time.Time
}

func (r RoundLog) Validate() error {
	if r.SessionID == "" || r.RoundNo <= 0 || r.ResultPayload == "" {
		return fmt.Errorf("invalid round log")
	}
	return nil
}
