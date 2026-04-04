package mongodb

import (
	"context"
	"fmt"

	"github.com/go-lynx/lynx"
	"github.com/go-lynx/lynx/log"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	sharedProviderResourceName = pluginName + ".provider"
	privateClientResourceName  = "client"
	privateDatabaseResource    = "database"
	privateConfigResource      = "config"
)

// Provider resolves the current MongoDB handles on demand so long-lived callers do not cache
// concrete client/database pointers across managed restarts.
type Provider interface {
	Client(ctx context.Context) (*mongo.Client, error)
	Database(ctx context.Context) (*mongo.Database, error)
	Collection(ctx context.Context, name string) (*mongo.Collection, error)
	DatabaseName() string
}

type provider struct{}

func getPlugin() (*PlugMongoDB, error) {
	app := lynx.Lynx()
	if app == nil {
		return nil, fmt.Errorf("lynx not initialized")
	}
	manager := app.GetPluginManager()
	if manager == nil {
		return nil, fmt.Errorf("plugin manager not initialized")
	}
	plugin := manager.GetPlugin(pluginName)
	if plugin == nil {
		return nil, fmt.Errorf("plugin %s not found", pluginName)
	}
	client, ok := plugin.(*PlugMongoDB)
	if !ok {
		return nil, fmt.Errorf("plugin %s is not a PlugMongoDB", pluginName)
	}
	return client, nil
}

// GetProvider returns the stable MongoDB provider.
func GetProvider() Provider {
	return provider{}
}

func (provider) Client(ctx context.Context) (*mongo.Client, error) {
	plugin, err := getPlugin()
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	client := plugin.GetClient()
	if client == nil {
		return nil, fmt.Errorf("mongodb client is nil")
	}
	return client, nil
}

func (p provider) Database(ctx context.Context) (*mongo.Database, error) {
	plugin, err := getPlugin()
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	database := plugin.GetDatabase()
	if database == nil {
		return nil, fmt.Errorf("mongodb database is nil")
	}
	return database, nil
}

func (p provider) Collection(ctx context.Context, name string) (*mongo.Collection, error) {
	if name == "" {
		return nil, fmt.Errorf("collection name cannot be empty")
	}
	database, err := p.Database(ctx)
	if err != nil {
		return nil, err
	}
	return database.Collection(name), nil
}

func (provider) DatabaseName() string {
	plugin, err := getPlugin()
	if err != nil || plugin.conf == nil {
		return ""
	}
	return plugin.conf.Database
}

func (p *PlugMongoDB) publishResourceContract() {
	if p == nil || p.rt == nil {
		return
	}

	mongoProvider := GetProvider()
	for _, resourceName := range []string{pluginName, sharedProviderResourceName} {
		if err := p.rt.RegisterSharedResource(resourceName, mongoProvider); err != nil {
			log.Warnf("failed to register mongodb shared resource %s: %v", resourceName, err)
		}
	}
	if err := p.rt.RegisterPrivateResource("provider", mongoProvider); err != nil {
		log.Warnf("failed to register mongodb private provider resource: %v", err)
	}
	if p.client != nil {
		if err := p.rt.RegisterPrivateResource(privateClientResourceName, p.client); err != nil {
			log.Warnf("failed to register mongodb private client resource: %v", err)
		}
	}
	if p.database != nil {
		if err := p.rt.RegisterPrivateResource(privateDatabaseResource, p.database); err != nil {
			log.Warnf("failed to register mongodb private database resource: %v", err)
		}
	}
	if p.conf != nil {
		if err := p.rt.RegisterPrivateResource(privateConfigResource, p.conf); err != nil {
			log.Warnf("failed to register mongodb private config resource: %v", err)
		}
	}
}
