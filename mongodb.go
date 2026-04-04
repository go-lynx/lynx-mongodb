package mongodb

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-lynx/lynx-mongodb/conf"
	"github.com/go-lynx/lynx/log"
	"github.com/go-lynx/lynx/plugins"
	"github.com/prometheus/client_golang/prometheus"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"google.golang.org/protobuf/types/known/durationpb"
)

// Initialize initializes the MongoDB plugin
func (p *PlugMongoDB) Initialize(plugin plugins.Plugin, rt plugins.Runtime) error {
	return p.initializeWithContext(context.Background(), plugin, rt)
}

// Start starts the MongoDB plugin
func (p *PlugMongoDB) Start(plugin plugins.Plugin) error {
	return p.startWithContext(context.Background(), plugin, "Start")
}

// Stop stops the MongoDB plugin
func (p *PlugMongoDB) Stop(plugin plugins.Plugin) error {
	return p.stopWithContext(context.Background(), plugin, "Stop")
}

// CleanupTasks implements the plugin cleanup interface
func (p *PlugMongoDB) CleanupTasks() error {
	return p.CleanupTasksContext(context.Background())
}

// CleanupTasksContext implements context-aware cleanup with proper timeout handling
func (p *PlugMongoDB) CleanupTasksContext(parentCtx context.Context) error {
	log.Info("cleaning up mongodb plugin")

	if err := p.stopBackgroundTasksContext(parentCtx); err != nil {
		return err
	}

	if p.client != nil {
		ctx, cancel := p.createTimeoutContext(parentCtx, 5*time.Second)
		defer cancel()
		if err := p.client.Disconnect(ctx); err != nil {
			log.Errorf("failed to disconnect mongodb client: %v", err)
			return err
		}
		p.client = nil
		p.database = nil
	}
	p.rt = nil

	p.resetLifecycleContext()

	log.Info("mongodb plugin cleaned up successfully")
	return nil
}

// createTimeoutContext creates a context with timeout, respecting parent context deadline
func (p *PlugMongoDB) createTimeoutContext(parentCtx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := parentCtx.Deadline(); ok {
		// Parent context has deadline, check if it's sooner than our timeout
		if time.Until(deadline) < timeout {
			return parentCtx, func() {} // Use parent context, no-op cancel
		}
	}
	return context.WithTimeout(parentCtx, timeout)
}

// parseConfig parses configuration
func (p *PlugMongoDB) parseConfig(cfg config.Config) error {
	// Read mongodb configuration from config
	var mongodbConf conf.MongoDB
	if err := cfg.Scan(&mongodbConf); err != nil {
		return err
	}
	p.conf = &mongodbConf

	// Set default values
	if p.conf.Uri == "" {
		p.conf.Uri = "mongodb://localhost:27017"
	}
	if p.conf.Database == "" {
		p.conf.Database = "test"
	}
	if p.conf.MaxPoolSize == 0 {
		p.conf.MaxPoolSize = 100
	}
	if p.conf.MinPoolSize == 0 {
		p.conf.MinPoolSize = 5
	}
	if p.conf.ConnectTimeout == nil {
		p.conf.ConnectTimeout = durationpb.New(30 * time.Second)
	}
	if p.conf.ServerSelectionTimeout == nil {
		p.conf.ServerSelectionTimeout = durationpb.New(30 * time.Second)
	}
	if p.conf.SocketTimeout == nil {
		p.conf.SocketTimeout = durationpb.New(30 * time.Second)
	}
	if p.conf.HeartbeatInterval == nil {
		p.conf.HeartbeatInterval = durationpb.New(10 * time.Second)
	}
	if p.conf.HealthCheckInterval == nil {
		p.conf.HealthCheckInterval = durationpb.New(30 * time.Second)
	}
	if p.conf.ReadConcernLevel == "" {
		p.conf.ReadConcernLevel = "local"
	}
	if p.conf.WriteConcernW == 0 {
		p.conf.WriteConcernW = 1
	}
	if p.conf.WriteConcernTimeout == nil {
		p.conf.WriteConcernTimeout = durationpb.New(5 * time.Second)
	}

	return nil
}

// createClient creates the MongoDB client
func (p *PlugMongoDB) createClient() error {
	return p.createClientContext(context.Background())
}

func (p *PlugMongoDB) createClientContext(parentCtx context.Context) error {
	// Parse timeout values
	connectTimeout := p.conf.ConnectTimeout.AsDuration()
	serverSelectionTimeout := p.conf.ServerSelectionTimeout.AsDuration()
	socketTimeout := p.conf.SocketTimeout.AsDuration()
	heartbeatInterval := p.conf.HeartbeatInterval.AsDuration()

	// Build client options
	clientOptions := options.Client().ApplyURI(p.conf.Uri)

	// Set CommandMonitor and PoolMonitor for Prometheus metrics
	if p.prometheusMetrics != nil {
		if cmdMon := p.prometheusMetrics.CreateCommandMonitor(p.conf); cmdMon != nil {
			clientOptions.SetMonitor(cmdMon)
		}
		if poolMon := p.prometheusMetrics.CreatePoolMonitor(p.conf, &p.poolActiveConns); poolMon != nil {
			clientOptions.SetPoolMonitor(poolMon)
		}
	}

	// Set connection pool configuration
	clientOptions.SetMaxPoolSize(p.conf.MaxPoolSize)
	clientOptions.SetMinPoolSize(p.conf.MinPoolSize)

	// Set timeout configuration
	clientOptions.SetConnectTimeout(connectTimeout)
	clientOptions.SetServerSelectionTimeout(serverSelectionTimeout)
	clientOptions.SetSocketTimeout(socketTimeout)
	clientOptions.SetHeartbeatInterval(heartbeatInterval)

	// Set authentication information
	if p.conf.Username != "" && p.conf.Password != "" {
		clientOptions.SetAuth(options.Credential{
			Username:   p.conf.Username,
			Password:   p.conf.Password,
			AuthSource: p.conf.AuthSource,
		})
	}

	// Set TLS configuration
	if p.conf.EnableTls {
		tlsOpts := make(map[string]interface{})
		if p.conf.TlsCertFile != "" {
			tlsOpts["certFile"] = p.conf.TlsCertFile
		}
		if p.conf.TlsKeyFile != "" {
			tlsOpts["keyFile"] = p.conf.TlsKeyFile
		}
		if p.conf.TlsCaFile != "" {
			tlsOpts["caFile"] = p.conf.TlsCaFile
		}
		if len(tlsOpts) > 0 {
			tlsConfig, err := options.BuildTLSConfig(tlsOpts)
			if err != nil {
				return fmt.Errorf("failed to build TLS config: %w", err)
			}
			clientOptions.SetTLSConfig(tlsConfig)
		}
	}

	// Set compression configuration
	if p.conf.EnableCompression {
		clientOptions.SetCompressors([]string{"zlib", "snappy"})
	}

	// Set retry writes
	if p.conf.EnableRetryWrites {
		clientOptions.SetRetryWrites(true)
	}

	// Set read concern
	if p.conf.EnableReadConcern {
		var rc *readconcern.ReadConcern
		switch p.conf.ReadConcernLevel {
		case "local":
			rc = readconcern.Local()
		case "majority":
			rc = readconcern.Majority()
		case "linearizable":
			rc = readconcern.Linearizable()
		case "snapshot":
			rc = readconcern.Snapshot()
		default:
			rc = readconcern.Local()
		}
		clientOptions.SetReadConcern(rc)
	}

	// Set write concern
	if p.conf.EnableWriteConcern {
		writeConcernTimeout := p.conf.WriteConcernTimeout.AsDuration()

		wc := writeconcern.New(
			writeconcern.W(int(p.conf.WriteConcernW)),
			writeconcern.WTimeout(writeConcernTimeout),
		)
		clientOptions.SetWriteConcern(wc)
	}

	// Create client with timeout to avoid startup hang
	ctx, cancel := p.createTimeoutContext(parentCtx, connectTimeout)
	defer cancel()
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return err
	}

	p.client = client

	// Get database instance
	p.database = client.Database(p.conf.Database)

	return nil
}

// testConnection tests the connection
func (p *PlugMongoDB) testConnection() error {
	return p.testConnectionContext(context.Background())
}

func (p *PlugMongoDB) testConnectionContext(parentCtx context.Context) error {
	ctx, cancel := p.createTimeoutContext(parentCtx, 10*time.Second)
	defer cancel()

	// Send ping request
	if err := p.client.Ping(ctx, nil); err != nil {
		return err
	}

	return nil
}

// startMetricsCollection starts metrics collection
func (p *PlugMongoDB) startMetricsCollection() {
	// Use health check interval for metrics collection or default to 30 seconds
	var interval time.Duration
	if p.conf.HealthCheckInterval != nil {
		interval = p.conf.HealthCheckInterval.AsDuration()
	} else {
		interval = 30 * time.Second
	}

	// Ensure quit channel exists
	p.ensureStatsQuit()
	baseCtx := p.lifecycleCtx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	p.metricsCancel = cancel

	p.statsWG.Add(1)
	go func() {
		defer p.statsWG.Done()
		// Collect immediately so Grafana database template has data from the start
		p.collectMetricsContext(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				p.collectMetricsContext(ctx)
			case <-ctx.Done():
				return
			case <-p.statsQuit:
				return
			}
		}
	}()
}

// stopMetricsCollection stops metrics collection
func (p *PlugMongoDB) stopMetricsCollection() {
	_ = p.stopBackgroundTasksContext(context.Background())
}

// collectMetrics collects metrics from MongoDB and updates Prometheus
func (p *PlugMongoDB) collectMetrics() {
	p.collectMetricsContext(context.Background())
}

func (p *PlugMongoDB) collectMetricsContext(parentCtx context.Context) {
	ctx, cancel := p.createTimeoutContext(parentCtx, 5*time.Second)
	defer cancel()

	// Update config-based metrics (connection pool max, etc.)
	if p.prometheusMetrics != nil {
		p.prometheusMetrics.UpdateConfigMetrics(p.conf)
	}

	// Get database statistics (validates connection, supports future extended metrics)
	var dbStatsResult bson.M
	if p.database == nil {
		return
	}
	if err := p.database.RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}}).Decode(&dbStatsResult); err != nil {
		log.Errorf("failed to get database stats: %v", err)
		return
	}
	_ = dbStatsResult // reserved for future storage-size etc. metrics

	log.Debug("mongodb metrics collected")
}

// startHealthCheck starts health check
func (p *PlugMongoDB) startHealthCheck() {
	interval := p.conf.HealthCheckInterval.AsDuration()

	// Ensure quit channel exists
	p.ensureStatsQuit()
	baseCtx := p.lifecycleCtx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	ctx, cancel := context.WithCancel(baseCtx)
	p.healthCancel = cancel

	p.statsWG.Add(1)
	go func() {
		defer p.statsWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := p.checkHealthContext(ctx); err != nil {
					log.Errorf("mongodb health check failed: %v", err)
				}
			case <-ctx.Done():
				return
			case <-p.statsQuit:
				return
			}
		}
	}()
}

// stopHealthCheck stops health check
func (p *PlugMongoDB) stopHealthCheck() {
	_ = p.stopBackgroundTasksContext(context.Background())
}

// closeStatsQuitOnce closes statsQuit only once in a thread-safe way
func (p *PlugMongoDB) closeStatsQuitOnce() {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	if !p.statsClosed && p.statsQuit != nil {
		close(p.statsQuit)
		p.statsClosed = true
	}
}

// checkHealth executes health check
func (p *PlugMongoDB) checkHealth() error {
	return p.checkHealthContext(context.Background())
}

func (p *PlugMongoDB) checkHealthContext(parentCtx context.Context) error {
	ctx, cancel := p.createTimeoutContext(parentCtx, 5*time.Second)
	defer cancel()

	if p.client == nil {
		return fmt.Errorf("mongodb client is nil")
	}
	err := p.client.Ping(ctx, nil)
	if p.prometheusMetrics != nil {
		p.prometheusMetrics.RecordHealthCheck(err == nil, p.conf)
	}
	if err != nil {
		return err
	}
	return nil
}

func (p *PlugMongoDB) CheckHealth() error {
	return p.checkHealthContext(context.Background())
}

func (p *PlugMongoDB) stopBackgroundTasksContext(parentCtx context.Context) error {
	if p.metricsCancel != nil {
		p.metricsCancel()
		p.metricsCancel = nil
	}
	if p.healthCancel != nil {
		p.healthCancel()
		p.healthCancel = nil
	}
	if p.statsQuit != nil {
		p.closeStatsQuitOnce()
	}

	done := make(chan struct{})
	go func() {
		p.statsWG.Wait()
		close(done)
	}()

	waitCtx, cancel := p.createTimeoutContext(parentCtx, 10*time.Second)
	defer cancel()

	select {
	case <-done:
		log.Infof("mongodb background tasks stopped successfully")
		p.statsMu.Lock()
		p.statsQuit = nil
		p.statsClosed = false
		p.statsMu.Unlock()
		return nil
	case <-waitCtx.Done():
		return waitCtx.Err()
	}
}

// GetClient gets the MongoDB client
func (p *PlugMongoDB) GetClient() *mongo.Client {
	return p.client
}

// GetDatabase gets the MongoDB database instance
func (p *PlugMongoDB) GetDatabase() *mongo.Database {
	return p.database
}

// GetCollection gets the collection instance
func (p *PlugMongoDB) GetCollection(collectionName string) *mongo.Collection {
	if p.database == nil {
		return nil
	}
	return p.database.Collection(collectionName)
}

// MetricsGatherer returns the Prometheus Gatherer for this plugin (implements metricsGathererProvider interface).
// Used by Lynx lifecycle to auto-register plugin metrics into the global /metrics endpoint.
func (p *PlugMongoDB) MetricsGatherer() prometheus.Gatherer {
	if p.prometheusMetrics == nil {
		return nil
	}
	return p.prometheusMetrics.GetGatherer()
}

// GetConnectionStats gets connection statistics
func (p *PlugMongoDB) GetConnectionStats() map[string]any {
	stats := make(map[string]any)

	if p.client != nil {
		// Get client statistics
		stats["client_initialized"] = true
		stats["database"] = p.conf.Database
		stats["max_pool_size"] = p.conf.MaxPoolSize
		stats["min_pool_size"] = p.conf.MinPoolSize
		stats["compression_enabled"] = p.conf.EnableCompression
		stats["tls_enabled"] = p.conf.EnableTls
	} else {
		stats["client_initialized"] = false
	}

	return stats
}
