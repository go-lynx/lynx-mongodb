// Package mongodb provides a MongoDB client plugin for the Lynx framework.
// It manages a mongo.Client connection pool with configurable TLS, authentication,
// read/write concerns, and compression. Background health checks and Prometheus metrics
// (command latency, pool stats, connection health) are collected at a configurable interval.
package mongodb

import (
	"context"

	"github.com/go-lynx/lynx"
	"github.com/go-lynx/lynx/pkg/factory"
	"github.com/go-lynx/lynx/plugins"
	"github.com/prometheus/client_golang/prometheus"
	"go.mongodb.org/mongo-driver/mongo"
)

// init registers the MongoDB plugin with the global factory on import.
func init() {
	factory.GlobalTypedFactory().RegisterPlugin(pluginName, confPrefix, func() plugins.Plugin {
		return NewMongoDBClient()
	})
}

// GetMongoDB returns the underlying mongo.Client, or nil if the plugin is not loaded.
func GetMongoDB() *mongo.Client {
	client, err := GetProvider().Client(context.Background())
	if err != nil {
		return nil
	}
	return client
}

// GetMongoDBPlugin returns the MongoDB plugin instance from the global manager, or nil.
func GetMongoDBPlugin() *PlugMongoDB {
	app := lynx.Lynx()
	if app == nil {
		return nil
	}
	manager := app.GetPluginManager()
	if manager == nil {
		return nil
	}
	plugin := manager.GetPlugin(pluginName)
	if plugin == nil {
		return nil
	}
	client, ok := plugin.(*PlugMongoDB)
	if !ok {
		return nil
	}
	return client
}

// GetMongoDBDatabase returns the configured database, or nil if the plugin is not loaded.
func GetMongoDBDatabase() *mongo.Database {
	database, err := GetProvider().Database(context.Background())
	if err != nil {
		return nil
	}
	return database
}

// GetMongoDBCollection returns a handle to the named collection, or nil if the plugin is not loaded.
func GetMongoDBCollection(collectionName string) *mongo.Collection {
	collection, err := GetProvider().Collection(context.Background(), collectionName)
	if err != nil {
		return nil
	}
	return collection
}

// GetMetricsGatherer returns the Prometheus Gatherer for the mongodb plugin, or nil if not loaded or metrics disabled.
// Use this to merge plugin metrics into your application's /metrics endpoint.
func GetMetricsGatherer() prometheus.Gatherer {
	plugin := GetMongoDBPlugin()
	if plugin == nil {
		return nil
	}
	return plugin.MetricsGatherer()
}
