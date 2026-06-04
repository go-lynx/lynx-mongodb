package mongodb

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-lynx/lynx-mongodb/conf"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ---------------------------------------------------------------------------
// createTimeoutContext
// ---------------------------------------------------------------------------

func TestCreateTimeoutContext_NoDeadline(t *testing.T) {
	p := NewMongoDBClient()
	ctx, cancel := p.createTimeoutContext(context.Background(), 100*time.Millisecond)
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected a deadline to be set")
	}
	if time.Until(dl) > 200*time.Millisecond {
		t.Errorf("deadline too far in future: %v", time.Until(dl))
	}
}

func TestCreateTimeoutContext_ShorterDeadline(t *testing.T) {
	p := NewMongoDBClient()
	// Parent context expires in 50ms, requested timeout 200ms.
	// The parent deadline is shorter, so it should be used as-is.
	parentCtx, parentCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer parentCancel()

	ctx, cancel := p.createTimeoutContext(parentCtx, 200*time.Millisecond)
	defer cancel()

	parentDl, _ := parentCtx.Deadline()
	childDl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline")
	}
	if !childDl.Equal(parentDl) {
		t.Errorf("expected child to inherit parent deadline; parent=%v child=%v", parentDl, childDl)
	}
}

// ---------------------------------------------------------------------------
// Options: remaining uncovered ones
// ---------------------------------------------------------------------------

func TestWithCredentials(t *testing.T) {
	p := NewMongoDBClient()
	WithCredentials("user", "pass", "admin")(p)
	if p.conf.Username != "user" {
		t.Errorf("expected Username=user, got %q", p.conf.Username)
	}
	if p.conf.Password != "pass" {
		t.Errorf("expected Password=pass, got %q", p.conf.Password)
	}
	if p.conf.AuthSource != "admin" {
		t.Errorf("expected AuthSource=admin, got %q", p.conf.AuthSource)
	}
}

func TestWithTimeouts(t *testing.T) {
	p := NewMongoDBClient()
	WithTimeouts(5*time.Second, 10*time.Second, 15*time.Second)(p)
	if p.conf.ConnectTimeout.AsDuration() != 5*time.Second {
		t.Errorf("ConnectTimeout mismatch")
	}
	if p.conf.ServerSelectionTimeout.AsDuration() != 10*time.Second {
		t.Errorf("ServerSelectionTimeout mismatch")
	}
	if p.conf.SocketTimeout.AsDuration() != 15*time.Second {
		t.Errorf("SocketTimeout mismatch")
	}
}

func TestWithHeartbeatInterval(t *testing.T) {
	p := NewMongoDBClient()
	WithHeartbeatInterval(20 * time.Second)(p)
	if p.conf.HeartbeatInterval.AsDuration() != 20*time.Second {
		t.Errorf("HeartbeatInterval mismatch")
	}
}

func TestWithCompression(t *testing.T) {
	p := NewMongoDBClient()
	WithCompression(true, 6)(p)
	if !p.conf.EnableCompression {
		t.Error("expected EnableCompression true")
	}
	if p.conf.CompressionLevel != 6 {
		t.Errorf("CompressionLevel: got %d, want 6", p.conf.CompressionLevel)
	}
}

func TestWithRetryWrites(t *testing.T) {
	p := NewMongoDBClient()
	WithRetryWrites(true)(p)
	if !p.conf.EnableRetryWrites {
		t.Error("expected EnableRetryWrites true")
	}
}

func TestWithReadConcern(t *testing.T) {
	p := NewMongoDBClient()
	WithReadConcern(true, "majority")(p)
	if !p.conf.EnableReadConcern {
		t.Error("expected EnableReadConcern true")
	}
	if p.conf.ReadConcernLevel != "majority" {
		t.Errorf("ReadConcernLevel: got %q, want majority", p.conf.ReadConcernLevel)
	}
}

func TestWithWriteConcern(t *testing.T) {
	p := NewMongoDBClient()
	WithWriteConcern(true, 2, 3*time.Second)(p)
	if !p.conf.EnableWriteConcern {
		t.Error("expected EnableWriteConcern true")
	}
	if p.conf.WriteConcernW != 2 {
		t.Errorf("WriteConcernW: got %d, want 2", p.conf.WriteConcernW)
	}
	if p.conf.WriteConcernTimeout.AsDuration() != 3*time.Second {
		t.Errorf("WriteConcernTimeout mismatch")
	}
}

// ---------------------------------------------------------------------------
// GetClient / GetDatabase / GetCollection / MetricsGatherer / GetConnectionStats
// ---------------------------------------------------------------------------

func TestGetClientNil(t *testing.T) {
	p := NewMongoDBClient()
	if p.GetClient() != nil {
		t.Error("expected nil client before initialization")
	}
}

func TestGetDatabaseNil(t *testing.T) {
	p := NewMongoDBClient()
	if p.GetDatabase() != nil {
		t.Error("expected nil database before initialization")
	}
}

func TestGetCollectionNilDatabase(t *testing.T) {
	p := NewMongoDBClient()
	if p.GetCollection("col") != nil {
		t.Error("expected nil collection when database is nil")
	}
}

func TestMetricsGathererNilWhenNoMetrics(t *testing.T) {
	p := NewMongoDBClient()
	if p.MetricsGatherer() != nil {
		t.Error("expected nil gatherer when prometheusMetrics is nil")
	}
}

func TestMetricsGathererWithMetrics(t *testing.T) {
	p := NewMongoDBClient()
	p.prometheusMetrics = NewPrometheusMetrics(nil)
	g := p.MetricsGatherer()
	if g == nil {
		t.Error("expected non-nil gatherer when prometheusMetrics is set")
	}
}

func TestGetConnectionStatsNoClient(t *testing.T) {
	p := NewMongoDBClient()
	p.conf = &conf.MongoDB{Database: "test"}
	stats := p.GetConnectionStats()
	if stats["client_initialized"] != false {
		t.Error("expected client_initialized=false")
	}
}

// ---------------------------------------------------------------------------
// stopBackgroundTasksContext – no running goroutines case
// ---------------------------------------------------------------------------

func TestStopBackgroundTasks_NoGoroutines(t *testing.T) {
	p := NewMongoDBClient()
	// No background tasks running; should return nil quickly.
	err := p.stopBackgroundTasksContext(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStopBackgroundTasks_WithMetricsCancel(t *testing.T) {
	p := NewMongoDBClient()
	p.ensureStatsQuit()
	var stopped int32
	p.metricsCancel = func() { atomic.StoreInt32(&stopped, 1) }
	p.healthCancel = func() {}

	err := p.stopBackgroundTasksContext(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&stopped) != 1 {
		t.Error("expected metricsCancel to be called")
	}
}

// ---------------------------------------------------------------------------
// collectMetricsContext – nil database path
// ---------------------------------------------------------------------------

func TestCollectMetricsContext_NilDatabase(t *testing.T) {
	p := NewMongoDBClient()
	p.conf = &conf.MongoDB{Database: "test"}
	p.prometheusMetrics = NewPrometheusMetrics(nil)
	// database is nil – should update config metrics and return without error
	p.collectMetricsContext(context.Background())
}

// ---------------------------------------------------------------------------
// CleanupTasksContext – nil client path
// ---------------------------------------------------------------------------

func TestCleanupTasksContext_NilClient(t *testing.T) {
	p := NewMongoDBClient()
	p.conf = &conf.MongoDB{}
	p.ensureLifecycleContext()
	err := p.CleanupTasksContext(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if p.lifecycleCtx != nil {
		t.Error("expected lifecycleCtx to be nil after cleanup")
	}
}

// ---------------------------------------------------------------------------
// Lifecycle context helpers
// ---------------------------------------------------------------------------

func TestEnsureLifecycleContext_AfterCancel(t *testing.T) {
	p := NewMongoDBClient()
	p.ensureLifecycleContext()
	// Cancel the context to simulate a done lifecycle context
	p.lifecycleStop()
	// Now ensureLifecycleContext should re-create the context
	p.ensureLifecycleContext()
	if p.lifecycleCtx == nil {
		t.Error("expected new lifecycleCtx after cancelled one")
	}
	select {
	case <-p.lifecycleCtx.Done():
		t.Error("new lifecycleCtx should not be done")
	default:
	}
}

// ---------------------------------------------------------------------------
// PrometheusMetrics - cloneLabels coverage
// ---------------------------------------------------------------------------

func TestCloneLabels(t *testing.T) {
	orig := map[string]string{"a": "1", "b": "2"}
	cloned := cloneLabels(orig)
	if cloned["a"] != "1" || cloned["b"] != "2" {
		t.Error("clone mismatch")
	}
	// Mutating clone should not affect original
	cloned["a"] = "99"
	if orig["a"] != "1" {
		t.Error("original modified after clone mutation")
	}
}

// ---------------------------------------------------------------------------
// toFloat64
// ---------------------------------------------------------------------------

func TestToFloat64(t *testing.T) {
	tests := []struct {
		in  any
		out float64
		ok  bool
	}{
		{int32(10), 10.0, true},
		{int64(20), 20.0, true},
		{float64(30.5), 30.5, true},
		{int(40), 40.0, true},
		{"notanumber", 0, false},
		{nil, 0, false},
	}
	for _, tt := range tests {
		v, ok := toFloat64(tt.in)
		if ok != tt.ok || v != tt.out {
			t.Errorf("toFloat64(%v) = (%v, %v), want (%v, %v)", tt.in, v, ok, tt.out, tt.ok)
		}
	}
}

// ---------------------------------------------------------------------------
// startMetricsCollection – integration (stop quickly)
// ---------------------------------------------------------------------------

func TestStartMetricsCollection_StopsCleanly(t *testing.T) {
	p := NewMongoDBClient()
	p.conf = &conf.MongoDB{
		Database:            "test",
		HealthCheckInterval: durationpb.New(50 * time.Millisecond),
	}
	p.prometheusMetrics = NewPrometheusMetrics(nil)
	p.ensureLifecycleContext()

	p.startMetricsCollection()

	// Give the goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Stop via metricsCancel
	if p.metricsCancel != nil {
		p.metricsCancel()
	}

	// stopBackgroundTasksContext should drain the WaitGroup
	err := p.stopBackgroundTasksContext(context.Background())
	if err != nil {
		t.Errorf("stopBackgroundTasksContext: %v", err)
	}
}

// ---------------------------------------------------------------------------
// startHealthCheck – integration (stop quickly)
// ---------------------------------------------------------------------------

func TestStartHealthCheck_StopsCleanly(t *testing.T) {
	p := NewMongoDBClient()
	p.conf = &conf.MongoDB{
		Database:            "test",
		HealthCheckInterval: durationpb.New(50 * time.Millisecond),
	}
	p.prometheusMetrics = NewPrometheusMetrics(nil)
	p.ensureLifecycleContext()

	p.startHealthCheck()

	time.Sleep(10 * time.Millisecond)

	if p.healthCancel != nil {
		p.healthCancel()
	}
	p.closeStatsQuitOnce()

	// Drain WaitGroup
	done := make(chan struct{})
	go func() {
		p.statsWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("health check goroutine did not stop")
	}
}

// ---------------------------------------------------------------------------
// extractDocumentsFromReply – remaining paths
// ---------------------------------------------------------------------------

func TestExtractDocumentsFromReply_NRemoved(t *testing.T) {
	reply, _ := bson.Marshal(bson.M{"nRemoved": int64(7), "ok": 1})
	n := extractDocumentsFromReply(reply, "delete")
	if n != 7 {
		t.Errorf("expected nRemoved=7, got %d", n)
	}
}

func TestExtractDocumentsFromReply_NextBatch(t *testing.T) {
	reply, _ := bson.Marshal(bson.M{
		"cursor": bson.M{
			"nextBatch": []any{bson.M{"x": 1}, bson.M{"x": 2}, bson.M{"x": 3}},
			"id":        int64(1),
		},
		"ok": 1,
	})
	n := extractDocumentsFromReply(reply, "getMore")
	if n != 3 {
		t.Errorf("expected nextBatch len=3, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// CommandMonitor – Succeeded / Failed handlers via direct invocation
// ---------------------------------------------------------------------------

func TestCommandMonitor_SucceededAndFailed(t *testing.T) {
	pm := NewPrometheusMetrics(nil)
	cfg := &conf.MongoDB{Database: "db1"}
	mon := pm.CreateCommandMonitor(cfg)
	if mon == nil {
		t.Fatal("nil CommandMonitor")
	}

	ctx := context.Background()

	// Fire the Started callback to exercise that path too
	if mon.Started != nil {
		mon.Started(ctx, &event.CommandStartedEvent{RequestID: 1, CommandName: "find"})
	}

	// Succeed: find 2 docs in a cursor
	replyBson, _ := bson.Marshal(bson.M{
		"cursor": bson.M{
			"firstBatch": []any{bson.M{"a": 1}, bson.M{"a": 2}},
			"id":         int64(0),
		},
		"ok": 1,
	})

	if mon.Succeeded != nil {
		mon.Succeeded(ctx, &event.CommandSucceededEvent{
			CommandFinishedEvent: event.CommandFinishedEvent{
				Duration:    5 * time.Millisecond,
				CommandName: "find",
				RequestID:   1,
			},
			Reply: replyBson,
		})
	}

	if mon.Failed != nil {
		mon.Failed(ctx, &event.CommandFailedEvent{
			CommandFinishedEvent: event.CommandFinishedEvent{
				Duration:    3 * time.Millisecond,
				CommandName: "update",
				RequestID:   2,
			},
		})
	}

	// Verify something was written
	mfs, err := pm.registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "lynx_mongodb_operations_total" {
			found = true
		}
	}
	if !found {
		t.Error("expected lynx_mongodb_operations_total metric")
	}
}

// ---------------------------------------------------------------------------
// PoolMonitor – GetSucceeded / ConnectionReturned
// ---------------------------------------------------------------------------

func TestPoolMonitor_Events(t *testing.T) {
	pm := NewPrometheusMetrics(nil)
	cfg := &conf.MongoDB{Database: "db1"}
	var activeCount int64
	monitor := pm.CreatePoolMonitor(cfg, &activeCount)
	if monitor == nil {
		t.Fatal("nil PoolMonitor")
	}

	monitor.Event(&event.PoolEvent{Type: event.GetSucceeded})
	if atomic.LoadInt64(&activeCount) != 1 {
		t.Errorf("expected activeCount=1, got %d", activeCount)
	}
	monitor.Event(&event.PoolEvent{Type: event.ConnectionReturned})
	if atomic.LoadInt64(&activeCount) != 0 {
		t.Errorf("expected activeCount=0, got %d", activeCount)
	}
}

// ---------------------------------------------------------------------------
// CreatePoolMonitor / CreateCommandMonitor nil safety
// ---------------------------------------------------------------------------

func TestCreatePoolMonitor_NilSafety(t *testing.T) {
	var pm *PrometheusMetrics
	if pm.CreatePoolMonitor(nil, nil) != nil {
		t.Error("expected nil when receiver or args are nil")
	}
	realPm := NewPrometheusMetrics(nil)
	if realPm.CreatePoolMonitor(nil, nil) != nil {
		t.Error("expected nil for nil cfg")
	}
}

func TestCreateCommandMonitor_NilSafety(t *testing.T) {
	var pm *PrometheusMetrics
	if pm.CreateCommandMonitor(nil) != nil {
		t.Error("expected nil when receiver is nil")
	}
}

// ---------------------------------------------------------------------------
// concurrent statsQuit safety
// ---------------------------------------------------------------------------

func TestCloseStatsQuitOnce_Concurrent(t *testing.T) {
	p := NewMongoDBClient()
	p.ensureStatsQuit()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.closeStatsQuitOnce()
		}()
	}
	wg.Wait()
	// The channel should be closed exactly once with no panic.
	select {
	case <-p.statsQuit:
	default:
		t.Error("expected statsQuit to be closed")
	}
}

// ---------------------------------------------------------------------------
// InitializeContext / StartContext / StopContext with cancelled context
// ---------------------------------------------------------------------------

func TestInitializeContext_CancelledContext(t *testing.T) {
	p := NewMongoDBClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.InitializeContext(ctx, p, nil)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestStartContext_CancelledContext(t *testing.T) {
	p := NewMongoDBClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.StartContext(ctx, p)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestStopContext_CancelledContext(t *testing.T) {
	p := NewMongoDBClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := p.StopContext(ctx, p)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// ---------------------------------------------------------------------------
// PluginProtocol + IsContextAware
// ---------------------------------------------------------------------------

func TestPluginProtocol_ContextLifecycle(t *testing.T) {
	p := NewMongoDBClient()
	proto := p.PluginProtocol()
	if !proto.ContextLifecycle {
		t.Error("expected ContextLifecycle=true")
	}
}

func TestIsContextAware(t *testing.T) {
	p := NewMongoDBClient()
	if !p.IsContextAware() {
		t.Error("expected IsContextAware=true")
	}
}

// ---------------------------------------------------------------------------
// Initialize / Start / Stop wrappers (delegate to context versions)
// ---------------------------------------------------------------------------

func TestInitialize_DelegatesToContext(t *testing.T) {
	p := NewMongoDBClient()
	// Passing nil runtime should fail gracefully (not panic)
	_ = p.Initialize(p, nil)
}

// ---------------------------------------------------------------------------
// checkHealthContext – nil client
// ---------------------------------------------------------------------------

func TestCheckHealthContext_NilClient(t *testing.T) {
	p := NewMongoDBClient()
	p.conf = &conf.MongoDB{Database: "test"}
	err := p.checkHealthContext(context.Background())
	if err == nil {
		t.Error("expected error for nil client")
	}
}

// ---------------------------------------------------------------------------
// CheckHealth delegate
// ---------------------------------------------------------------------------

func TestCheckHealth_NilClient(t *testing.T) {
	p := NewMongoDBClient()
	p.conf = &conf.MongoDB{Database: "test"}
	err := p.CheckHealth()
	if err == nil {
		t.Error("expected error from CheckHealth with nil client")
	}
}

// ---------------------------------------------------------------------------
// Provider - Collection / DatabaseName error paths
// ---------------------------------------------------------------------------

func TestProvider_CollectionEmptyName(t *testing.T) {
	prov := GetProvider()
	_, err := prov.Collection(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty collection name")
	}
}

func TestProvider_DatabaseName_Unavailable(t *testing.T) {
	prov := GetProvider()
	name := prov.DatabaseName()
	if name != "" {
		t.Error("expected empty string when lynx not initialized")
	}
}

// ---------------------------------------------------------------------------
// GetMongoDBCollection with empty name
// ---------------------------------------------------------------------------

func TestGetMongoDBCollection_EmptyName(t *testing.T) {
	col := GetMongoDBCollection("")
	if col != nil {
		t.Error("expected nil for empty collection name")
	}
}

// ---------------------------------------------------------------------------
// GetMetricsGatherer via plug.go (nil plugin path)
// ---------------------------------------------------------------------------

func TestGetMetricsGatherer_NilPlugin(t *testing.T) {
	g := GetMetricsGatherer()
	if g != nil {
		t.Error("expected nil when plugin is not loaded")
	}
}

// ---------------------------------------------------------------------------
// UpdateConfigMetrics nil safety
// ---------------------------------------------------------------------------

func TestUpdateConfigMetrics_NilSafety(t *testing.T) {
	var pm *PrometheusMetrics
	pm.UpdateConfigMetrics(nil) // should not panic
	realPm := NewPrometheusMetrics(nil)
	realPm.UpdateConfigMetrics(nil) // should not panic either
}

// ---------------------------------------------------------------------------
// RecordHealthCheck nil safety
// ---------------------------------------------------------------------------

func TestRecordHealthCheck_NilSafety(t *testing.T) {
	var pm *PrometheusMetrics
	pm.RecordHealthCheck(true, nil) // should not panic
}

// ---------------------------------------------------------------------------
// GetGatherer nil safety
// ---------------------------------------------------------------------------

func TestGetGatherer_NilSafety(t *testing.T) {
	var pm *PrometheusMetrics
	if pm.GetGatherer() != nil {
		t.Error("expected nil for nil receiver")
	}
}
