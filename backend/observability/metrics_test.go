package observability

import "testing"

func TestMetricsFunctionsSmoke(t *testing.T) {
	ObserveHTTPRequest("GET", "/api/me", 200, 0.01)
	ObserveWSSendLatency(0.02)
	ObserveWSMessage("HIT", "success", 0.03)

	IncActiveWSConnections()
	DecActiveWSConnections()
	IncReconnect()
	IncVersionConflict()
	IncDuplicateAction()
	IncAutoStand()
	IncTimeForfeit()
	IncDealerDraw()
	SetRoomCount(10)
	SetSessionCount(20)
}

