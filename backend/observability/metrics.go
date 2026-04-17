// Package observability は Prometheus メトリクスを提供する（仕様 20 章）。
// 20.2 相当の deploy_success_count / deploy_failure_count は CI・デプロイ基盤側で計測する想定のためアプリには含めない。
// 20.3 のアラートは Alertmanager 等のルール定義で本メトリクスを参照する。
package observability

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "blackjack_http_requests_total",
		Help: "Total HTTP requests by method/path/status.",
	}, []string{"method", "path", "status"})

	httpRequestDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "blackjack_http_request_duration_seconds",
		Help:    "HTTP request latency by method/path.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "path"})

	wsSendLatencySeconds = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "blackjack_ws_send_latency_seconds",
		Help:    "WebSocket send latency.",
		Buckets: prometheus.DefBuckets,
	})

	wsMessageDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "blackjack_ws_message_duration_seconds",
		Help:    "WebSocket message processing latency by event/result.",
		Buckets: prometheus.DefBuckets,
	}, []string{"event", "result"})

	activeWSConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "blackjack_active_ws_connections",
		Help: "Current active websocket connections.",
	})

	reconnectTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "blackjack_reconnect_total",
		Help: "Count of websocket reconnects (old connection replaced).",
	})

	versionConflictTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "blackjack_version_conflict_total",
		Help: "Count of version_conflict responses.",
	})

	duplicateActionTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "blackjack_duplicate_action_total",
		Help: "Count of duplicate_action responses.",
	})

	autoStandTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "blackjack_auto_stand_total",
		Help: "Count of auto-stand executions.",
	})

	dealerDrawTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "blackjack_dealer_draw_total",
		Help: "Count of dealer draw executions.",
	})

	roomCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "blackjack_room_count",
		Help: "Total rooms count in database.",
	})

	sessionCount = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "blackjack_session_count",
		Help: "Total sessions count in database.",
	})
)

func ObserveHTTPRequest(method, path string, statusCode int, durationSeconds float64) {
	status := strconv.Itoa(statusCode)
	httpRequestsTotal.WithLabelValues(method, path, status).Inc()
	httpRequestDurationSeconds.WithLabelValues(method, path).Observe(durationSeconds)
}

func ObserveWSSendLatency(durationSeconds float64) {
	wsSendLatencySeconds.Observe(durationSeconds)
}

func ObserveWSMessage(eventType, result string, durationSeconds float64) {
	wsMessageDurationSeconds.WithLabelValues(eventType, result).Observe(durationSeconds)
}

func IncActiveWSConnections() {
	activeWSConnections.Inc()
}

func DecActiveWSConnections() {
	activeWSConnections.Dec()
}

func IncReconnect() {
	reconnectTotal.Inc()
}

func IncVersionConflict() {
	versionConflictTotal.Inc()
}

func IncDuplicateAction() {
	duplicateActionTotal.Inc()
}

func IncAutoStand() {
	autoStandTotal.Inc()
}

func IncDealerDraw() {
	dealerDrawTotal.Inc()
}

func SetRoomCount(v float64) {
	roomCount.Set(v)
}

func SetSessionCount(v float64) {
	sessionCount.Set(v)
}
