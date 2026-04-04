package mongodb

import (
	"context"
	"fmt"

	"github.com/go-lynx/lynx/log"
	"github.com/go-lynx/lynx/plugins"
)

func (p *PlugMongoDB) PluginProtocol() plugins.PluginProtocol {
	protocol := p.BasePlugin.PluginProtocol()
	protocol.ContextLifecycle = true
	return protocol
}

func (p *PlugMongoDB) IsContextAware() bool {
	return true
}

func (p *PlugMongoDB) InitializeContext(ctx context.Context, plugin plugins.Plugin, rt plugins.Runtime) error {
	return p.initializeWithContext(ctx, plugin, rt)
}

func (p *PlugMongoDB) StartContext(ctx context.Context, plugin plugins.Plugin) error {
	return p.startWithContext(ctx, plugin, "StartContext")
}

func (p *PlugMongoDB) StopContext(ctx context.Context, plugin plugins.Plugin) error {
	return p.stopWithContext(ctx, plugin, "StopContext")
}

func (p *PlugMongoDB) initializeWithContext(ctx context.Context, plugin plugins.Plugin, rt plugins.Runtime) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("initialize canceled before start: %w", err)
	}

	if err := p.BasePlugin.Initialize(plugin, rt); err != nil {
		log.Error(err)
		return err
	}

	cfg := rt.GetConfig()
	if cfg == nil {
		return fmt.Errorf("failed to get config from runtime")
	}

	if err := p.parseConfig(cfg); err != nil {
		return fmt.Errorf("failed to parse mongodb config: %w", err)
	}
	p.rt = rt.WithPluginContext(pluginName)

	if p.conf.EnableMetrics && p.prometheusMetrics == nil {
		p.prometheusMetrics = NewPrometheusMetrics(&PrometheusConfig{
			Namespace: "lynx",
			Subsystem: "mongodb",
		})
	}

	p.ensureLifecycleContext()

	if err := p.createClientContext(ctx); err != nil {
		p.resetLifecycleContext()
		return fmt.Errorf("failed to create mongodb client: %w", err)
	}
	p.publishResourceContract()

	if p.conf.EnableMetrics {
		p.startMetricsCollection()
	}
	if p.conf.EnableHealthCheck {
		p.startHealthCheck()
	}

	log.Info("mongodb plugin initialized successfully")
	return nil
}

func (p *PlugMongoDB) startWithContext(ctx context.Context, plugin plugins.Plugin, source string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start canceled before execution: %w", err)
	}
	if p.Status(plugin) == plugins.StatusActive {
		return plugins.ErrPluginAlreadyActive
	}

	p.SetStatus(plugins.StatusInitializing)
	p.EmitEvent(plugins.PluginEvent{
		Type:     plugins.EventPluginStarting,
		Priority: plugins.PriorityNormal,
		Source:   source,
		Category: "lifecycle",
	})

	if p.client == nil {
		p.SetStatus(plugins.StatusFailed)
		return fmt.Errorf("mongodb client is not initialized")
	}

	if err := p.testConnectionContext(ctx); err != nil {
		p.SetStatus(plugins.StatusFailed)
		return fmt.Errorf("failed to test mongodb connection: %w", err)
	}
	p.publishResourceContract()

	if p.conf != nil && p.conf.EnableMetrics && p.metricsCancel == nil {
		p.startMetricsCollection()
	}
	if p.conf != nil && p.conf.EnableHealthCheck && p.healthCancel == nil {
		p.startHealthCheck()
	}

	p.SetStatus(plugins.StatusActive)
	p.EmitEvent(plugins.PluginEvent{
		Type:     plugins.EventPluginStarted,
		Priority: plugins.PriorityNormal,
		Source:   source,
		Category: "lifecycle",
	})

	log.Info("mongodb plugin started successfully")
	return nil
}

func (p *PlugMongoDB) stopWithContext(ctx context.Context, plugin plugins.Plugin, source string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("stop canceled before execution: %w", err)
	}
	if p.Status(plugin) != plugins.StatusActive {
		return plugins.NewPluginError(p.ID(), "Stop", "Plugin must be active to stop", plugins.ErrPluginNotActive)
	}

	p.SetStatus(plugins.StatusStopping)
	p.EmitEvent(plugins.PluginEvent{
		Type:     plugins.EventPluginStopping,
		Priority: plugins.PriorityNormal,
		Source:   source,
		Category: "lifecycle",
	})

	if err := p.CleanupTasksContext(ctx); err != nil {
		p.SetStatus(plugins.StatusFailed)
		return plugins.NewPluginError(p.ID(), "Stop", "Failed to perform cleanup tasks", err)
	}

	p.SetStatus(plugins.StatusTerminated)
	p.EmitEvent(plugins.PluginEvent{
		Type:     plugins.EventPluginStopped,
		Priority: plugins.PriorityNormal,
		Source:   source,
		Category: "lifecycle",
	})

	log.Info("mongodb plugin stopped successfully")
	return nil
}

func (p *PlugMongoDB) ensureLifecycleContext() {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	if p.lifecycleCtx != nil {
		select {
		case <-p.lifecycleCtx.Done():
			p.lifecycleCtx = nil
			p.lifecycleStop = nil
		default:
			return
		}
	}
	p.lifecycleCtx, p.lifecycleStop = context.WithCancel(context.Background())
}

func (p *PlugMongoDB) resetLifecycleContext() {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	if p.lifecycleStop != nil {
		p.lifecycleStop()
	}
	p.lifecycleCtx = nil
	p.lifecycleStop = nil
}

func (p *PlugMongoDB) ensureStatsQuit() {
	p.statsMu.Lock()
	defer p.statsMu.Unlock()
	if p.statsQuit == nil {
		p.statsQuit = make(chan struct{})
	}
	p.statsClosed = false
}
