package mongodb

import (
	"context"

	"github.com/go-lynx/lynx"
	"github.com/go-lynx/lynx/pkg/factory"
	"github.com/go-lynx/lynx/plugins"
	"github.com/prometheus/client_golang/prometheus"
	"go.mongodb.org/mongo-driver/mongo"
)

// init function is a special function in Go that is automatically executed when the package is loaded.
// This function registers the MongoDB client plugin to the global plugin factory.
// The first parameter pluginName is the unique name of the plugin, used to identify the plugin.
// The second parameter confPrefix is the configuration prefix, used to read plugin-related configuration from the config.
// The third parameter is an anonymous function that returns an instance of plugins.Plugin interface type,
// by calling the NewMongoDBClient function to create a new MongoDB client plugin instance.
func init() {
	// Register the MongoDB client plugin to the global plugin factory.
	// The first parameter pluginName is the unique plugin name used for identification.
	// The second parameter confPrefix is the configuration prefix, used to read plugin-related configuration from the config.
	// The third parameter is an anonymous function that returns an instance of plugins.Plugin interface type,
	// by calling the NewMongoDBClient function to create a new MongoDB client plugin instance.
	factory.GlobalTypedFactory().RegisterPlugin(pluginName, confPrefix, func() plugins.Plugin {
		return NewMongoDBClient()
	})
}

// GetMongoDB function is used to get the MongoDB client instance.
// It gets the plugin manager through the global Lynx application instance, then gets the corresponding plugin instance by plugin name,
// finally converts the plugin instance to *PlugMongoDB type and returns its client field, which is the MongoDB client.
func GetMongoDB() *mongo.Client {
	client, err := GetProvider().Client(context.Background())
	if err != nil {
		return nil
	}
	return client
}

// GetMongoDBPlugin gets the MongoDB plugin instance
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

// GetMongoDBDatabase gets the MongoDB database instance
func GetMongoDBDatabase() *mongo.Database {
	database, err := GetProvider().Database(context.Background())
	if err != nil {
		return nil
	}
	return database
}

// GetMongoDBCollection gets the MongoDB collection instance
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
