package model

import "time"

const DefaultRematchDeadline = 30 * time.Second

func (s *GameSession) SetRematchDeadline(now time.Time) {
	if now.IsZero() {
		s.RematchDeadlineAt = nil
		return
	}
	t := now.Add(DefaultRematchDeadline)
	s.RematchDeadlineAt = &t
}

func RematchUnanimous(eligibleUserIDs []string, agreeByUserID map[string]bool) bool {
	if len(eligibleUserIDs) == 0 {
		return false
	}
	for _, uid := range eligibleUserIDs {
		if !agreeByUserID[uid] {
			return false
		}
	}
	return true
}
