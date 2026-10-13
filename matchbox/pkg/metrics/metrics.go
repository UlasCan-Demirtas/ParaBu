package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	OrdersProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "matchbox_orders_processed_total",
			Help: "Total number of orders processed",
		},
		[]string{"symbol", "side"},
	)
	TradesExecuted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "matchbox_trades_executed_total",
			Help: "Total number of trades executed",
		},
		[]string{"symbol"},
	)
	WsActiveConnections = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "ws_active_connections",
			Help: "Current number of active WebSocket connections",
		},
	)
	MatchingEngineLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "matching_engine_latency_seconds",
			Help:    "Latency of order matching in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"symbol"},
	)
	HttpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
)

func Init() {
	prometheus.MustRegister(OrdersProcessed)
	prometheus.MustRegister(TradesExecuted)
	prometheus.MustRegister(WsActiveConnections)
	prometheus.MustRegister(MatchingEngineLatency)
	prometheus.MustRegister(HttpRequestsTotal)
}
