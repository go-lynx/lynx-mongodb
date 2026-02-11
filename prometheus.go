package mongodb

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/go-lynx/lynx-mongodb/conf"
	"github.com/prometheus/client_golang/prometheus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
)

// PrometheusMetrics holds all Prometheus metrics for MongoDB
type PrometheusMetrics struct {
	registry *prometheus.Registry

	// Connection pool metrics (from PoolMonitor + config)
	connectionPoolActive *prometheus.GaugeVec
	connectionPoolMax    *prometheus.GaugeVec
	activeConnections    *prometheus.GaugeVec // alias for Grafana "lynx_mongodb_active_connections"

	// Operation metrics (from CommandMonitor)
	operationsTotal    *prometheus.CounterVec
	queryDuration      *prometheus.HistogramVec
	errorsTotal        *prometheus.CounterVec
	documentsProcessed *prometheus.CounterVec

	// Health check metrics
	healthCheckTotal   *prometheus.CounterVec
	healthCheckSuccess *prometheus.CounterVec
	healthCheckFailure *prometheus.CounterVec
}

// PrometheusConfig configuration for Prometheus metrics
type PrometheusConfig struct {
	Namespace string
	Subsystem string
	Labels    map[string]string
}

var (
	labelNames = []string{"database"}
)

// NewPrometheusMetrics creates new Prometheus metrics instance
func NewPrometheusMetrics(config *PrometheusConfig) *PrometheusMetrics {
	if config == nil {
		config = &PrometheusConfig{
			Namespace: "lynx",
			Subsystem: "mongodb",
			Labels:    make(map[string]string),
		}
	}
	if config.Subsystem == "" {
		config.Subsystem = "mongodb"
	}

	registry := prometheus.NewRegistry()

	m := &PrometheusMetrics{
		registry: registry,

		connectionPoolActive: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "connection_pool_active",
				Help:      "Number of active (checked out) connections in the pool",
			},
			labelNames,
		),
		connectionPoolMax: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "connection_pool_max",
				Help:      "Maximum number of connections in the pool",
			},
			labelNames,
		),
		activeConnections: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "active_connections",
				Help:      "Number of active connections (same as connection_pool_active)",
			},
			labelNames,
		),
		operationsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "operations_total",
				Help:      "Total number of MongoDB operations by type",
			},
			append(labelNames, "operation"),
		),
		queryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "query_duration_seconds",
				Help:      "MongoDB command duration in seconds",
				Buckets:   []float64{0.001, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.3, 0.5, 0.75, 1, 1.5, 2, 3, 5},
			},
			append(labelNames, "operation"),
		),
		errorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "errors_total",
				Help:      "Total number of MongoDB operation errors",
			},
			labelNames,
		),
		documentsProcessed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "documents_processed_total",
				Help:      "Total number of documents processed (find, update, delete, etc.)",
			},
			labelNames,
		),
		healthCheckTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "health_check_total",
				Help:      "Total number of health checks performed",
			},
			labelNames,
		),
		healthCheckSuccess: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "health_check_success_total",
				Help:      "Total number of successful health checks",
			},
			labelNames,
		),
		healthCheckFailure: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: config.Namespace,
				Subsystem: config.Subsystem,
				Name:      "health_check_failure_total",
				Help:      "Total number of failed health checks",
			},
			labelNames,
		),
	}

	registry.MustRegister(
		m.connectionPoolActive,
		m.connectionPoolMax,
		m.activeConnections,
		m.operationsTotal,
		m.queryDuration,
		m.errorsTotal,
		m.documentsProcessed,
		m.healthCheckTotal,
		m.healthCheckSuccess,
		m.healthCheckFailure,
	)

	return m
}

// CreateCommandMonitor creates a CommandMonitor that records metrics
func (m *PrometheusMetrics) CreateCommandMonitor(cfg *conf.MongoDB) *event.CommandMonitor {
	if m == nil || cfg == nil {
		return nil
	}
	labels := m.buildLabels(cfg)
	startedCmds := &sync.Map{} // requestID -> evt, for cleanup

	return &event.CommandMonitor{
		Started: func(_ context.Context, evt *event.CommandStartedEvent) {
			startedCmds.Store(evt.RequestID, struct{}{})
		},
		Succeeded: func(_ context.Context, evt *event.CommandSucceededEvent) {
			op := mapCommandNameToOperation(evt.CommandName)
			l := cloneLabels(labels)
			l["operation"] = op

			m.operationsTotal.With(l).Inc()
			m.queryDuration.With(l).Observe(evt.Duration.Seconds())

			// Extract documents processed from reply
			if n := extractDocumentsFromReply(evt.Reply, evt.CommandName); n > 0 {
				m.documentsProcessed.With(labels).Add(float64(n))
			}

			startedCmds.Delete(evt.RequestID)
		},
		Failed: func(_ context.Context, evt *event.CommandFailedEvent) {
			op := mapCommandNameToOperation(evt.CommandName)
			l := cloneLabels(labels)
			l["operation"] = op

			m.operationsTotal.With(l).Inc()
			m.queryDuration.With(l).Observe(evt.Duration.Seconds())
			m.errorsTotal.With(labels).Inc()

			startedCmds.Delete(evt.RequestID)
		},
	}
}

// CreatePoolMonitor creates a PoolMonitor that updates connection pool metrics
func (m *PrometheusMetrics) CreatePoolMonitor(cfg *conf.MongoDB, activeCount *int64) *event.PoolMonitor {
	if m == nil || cfg == nil || activeCount == nil {
		return nil
	}
	labels := m.buildLabels(cfg)

	return &event.PoolMonitor{
		Event: func(evt *event.PoolEvent) {
			switch evt.Type {
			case event.GetSucceeded:
				atomic.AddInt64(activeCount, 1)
				n := float64(atomic.LoadInt64(activeCount))
				m.connectionPoolActive.With(labels).Set(n)
				m.activeConnections.With(labels).Set(n)
			case event.ConnectionReturned:
				atomic.AddInt64(activeCount, -1)
				n := float64(atomic.LoadInt64(activeCount))
				m.connectionPoolActive.With(labels).Set(n)
				m.activeConnections.With(labels).Set(n)
			}
		},
	}
}

// UpdateConfigMetrics updates connection pool max from config (called periodically)
func (m *PrometheusMetrics) UpdateConfigMetrics(cfg *conf.MongoDB) {
	if m == nil || cfg == nil {
		return
	}
	labels := m.buildLabels(cfg)
	m.connectionPoolMax.With(labels).Set(float64(cfg.MaxPoolSize))
}

// RecordHealthCheck records health check result
func (m *PrometheusMetrics) RecordHealthCheck(success bool, cfg *conf.MongoDB) {
	if m == nil || cfg == nil {
		return
	}
	labels := m.buildLabels(cfg)
	m.healthCheckTotal.With(labels).Inc()
	if success {
		m.healthCheckSuccess.With(labels).Inc()
	} else {
		m.healthCheckFailure.With(labels).Inc()
	}
}

// GetGatherer returns the Prometheus gatherer
func (m *PrometheusMetrics) GetGatherer() prometheus.Gatherer {
	if m == nil || m.registry == nil {
		return nil
	}
	return m.registry
}

func (m *PrometheusMetrics) buildLabels(cfg *conf.MongoDB) prometheus.Labels {
	db := "test"
	if cfg != nil && cfg.Database != "" {
		db = cfg.Database
	}
	return prometheus.Labels{"database": db}
}

func cloneLabels(in prometheus.Labels) prometheus.Labels {
	out := prometheus.Labels{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

// mapCommandNameToOperation maps MongoDB command names to Grafana-friendly operation names
func mapCommandNameToOperation(cmdName string) string {
	switch cmdName {
	case "find", "findOne":
		return "find"
	case "insert", "insertOne", "insertMany":
		return "insert"
	case "update", "updateOne", "updateMany":
		return "update"
	case "delete", "deleteOne", "deleteMany", "remove":
		return "delete"
	case "aggregate":
		return "aggregate"
	default:
		return cmdName
	}
}

// extractDocumentsFromReply extracts document count from command reply
func extractDocumentsFromReply(reply bson.Raw, cmdName string) int64 {
	if len(reply) == 0 {
		return 0
	}
	var doc struct {
		N         int64 `bson:"n"`
		NModified int64 `bson:"nModified"`
		NInserted int64 `bson:"nInserted"`
		NRemoved  int64 `bson:"nRemoved"`
		NDropped  int64 `bson:"nDropped"`
		Cursor    *struct {
			FirstBatch []interface{} `bson:"firstBatch"`
			NextBatch  []interface{} `bson:"nextBatch"`
		} `bson:"cursor"`
	}
	if err := bson.Unmarshal(reply, &doc); err != nil {
		return 0
	}
	if doc.N != 0 {
		return doc.N
	}
	if doc.NModified != 0 {
		return doc.NModified
	}
	if doc.NInserted != 0 {
		return doc.NInserted
	}
	if doc.NRemoved != 0 {
		return doc.NRemoved
	}
	if doc.Cursor != nil {
		if len(doc.Cursor.FirstBatch) > 0 {
			return int64(len(doc.Cursor.FirstBatch))
		}
		if len(doc.Cursor.NextBatch) > 0 {
			return int64(len(doc.Cursor.NextBatch))
		}
	}
	return 0
}
