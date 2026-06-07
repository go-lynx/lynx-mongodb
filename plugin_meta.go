package mongodb

import (
	"github.com/go-lynx/lynx/plugins"
)

const (
	pluginName        = "mongodb.client"
	pluginVersion     = "v1.6.3"
	pluginDescription = "mongodb plugin for lynx framework"
	confPrefix        = "lynx.mongodb"
)

// NewMongoDBClient creates a new MongoDB plugin instance.
func NewMongoDBClient() *PlugMongoDB {
	return &PlugMongoDB{
		BasePlugin: plugins.NewBasePlugin(
			plugins.GeneratePluginID("", pluginName, pluginVersion),
			pluginName,
			pluginDescription,
			pluginVersion,
			confPrefix,
			100,
		),
	}
}
