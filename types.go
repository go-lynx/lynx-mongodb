package mongodb

import (
	"sync"

	"github.com/go-lynx/lynx-mongodb/conf"
	"github.com/go-lynx/lynx/plugins"
	"go.mongodb.org/mongo-driver/mongo"
)

// PlugMongoDB represents a MongoDB plugin instance
type PlugMongoDB struct {
	// Inherits from base plugin
	*plugins.BasePlugin
	// MongoDB configuration
	conf *conf.MongoDB
	// MongoDB client instance
	client *mongo.Client
	// MongoDB database instance
	database *mongo.Database
	// Prometheus metrics (nil if EnableMetrics is false)
	prometheusMetrics *PrometheusMetrics
	// Pool monitor: number of checked-out connections (for Prometheus)
	poolActiveConns int64
	// Metrics collection
	statsQuit     chan struct{}
	statsWG       sync.WaitGroup
	statsClosed   bool
	statsMu       sync.Mutex
	metricsCancel func()
	healthCancel  func()
}
