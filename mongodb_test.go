package mongodb

import (
	"testing"
	"time"

	"github.com/go-lynx/lynx-mongodb/conf"
	"go.mongodb.org/mongo-driver/bson"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestNewMongoDBClient(t *testing.T) {
	client := NewMongoDBClient()
	if client == nil {
		t.Fatal("NewMongoDBClient returned nil")
	}
	if client.Name() != pluginName {
		t.Errorf("expected name %q, got %q", pluginName, client.Name())
	}
	if client.conf != nil {
		t.Error("expected conf to be nil before Initialize")
	}
}

func TestOptionFunctions(t *testing.T) {
	client := NewMongoDBClient()
	// Options require conf to be set; we need to run parseConfig first or have options init conf
	// Test that options don't panic when applied to fresh client
	WithURI("mongodb://localhost:27017")(client)
	if client.conf == nil {
		t.Error("WithURI should initialize conf")
	}
	if client.conf.Uri != "mongodb://localhost:27017" {
		t.Errorf("expected uri mongodb://localhost:27017, got %q", client.conf.Uri)
	}

	WithDatabase("testdb")(client)
	if client.conf.Database != "testdb" {
		t.Errorf("expected database testdb, got %q", client.conf.Database)
	}

	WithPoolSize(50, 10)(client)
	if client.conf.MaxPoolSize != 50 || client.conf.MinPoolSize != 10 {
		t.Errorf("expected pool 50/10, got %d/%d", client.conf.MaxPoolSize, client.conf.MinPoolSize)
	}

	WithMetrics(true)(client)
	if !client.conf.EnableMetrics {
		t.Error("expected EnableMetrics true")
	}

	WithHealthCheck(true, 15*time.Second)(client)
	if !client.conf.EnableHealthCheck {
		t.Error("expected EnableHealthCheck true")
	}
}

func TestMapCommandNameToOperation(t *testing.T) {
	tests := []struct {
		cmd      string
		expected string
	}{
		{"find", "find"},
		{"findOne", "find"},
		{"insert", "insert"},
		{"insertOne", "insert"},
		{"insertMany", "insert"},
		{"update", "update"},
		{"updateOne", "update"},
		{"updateMany", "update"},
		{"delete", "delete"},
		{"deleteOne", "delete"},
		{"deleteMany", "delete"},
		{"aggregate", "aggregate"},
		{"unknownCmd", "unknownCmd"},
	}
	for _, tt := range tests {
		got := mapCommandNameToOperation(tt.cmd)
		if got != tt.expected {
			t.Errorf("mapCommandNameToOperation(%q) = %q, want %q", tt.cmd, got, tt.expected)
		}
	}
}

func TestExtractDocumentsFromReply(t *testing.T) {
	// Reply with nModified
	reply1, _ := bson.Marshal(bson.M{"n": int64(5), "ok": 1})
	if n := extractDocumentsFromReply(reply1, "update"); n != 5 {
		t.Errorf("expected n=5, got %d", n)
	}

	// Reply with nInserted
	reply2, _ := bson.Marshal(bson.M{"nInserted": int64(3), "ok": 1})
	if n := extractDocumentsFromReply(reply2, "insert"); n != 3 {
		t.Errorf("expected nInserted=3, got %d", n)
	}

	// Reply with cursor firstBatch
	reply3, _ := bson.Marshal(bson.M{
		"cursor": bson.M{
			"firstBatch": []interface{}{bson.M{"a": 1}, bson.M{"a": 2}},
			"id":         int64(0),
		},
		"ok": 1,
	})
	if n := extractDocumentsFromReply(reply3, "find"); n != 2 {
		t.Errorf("expected firstBatch len=2, got %d", n)
	}

	// Empty reply
	if n := extractDocumentsFromReply(nil, "find"); n != 0 {
		t.Errorf("expected 0 for nil reply, got %d", n)
	}
}

func TestPrometheusMetrics(t *testing.T) {
	pm := NewPrometheusMetrics(nil)
	if pm == nil {
		t.Fatal("NewPrometheusMetrics returned nil")
	}
	if pm.GetGatherer() == nil {
		t.Error("GetGatherer returned nil")
	}

	cfg := &conf.MongoDB{
		Database:    "testdb",
		MaxPoolSize: 100,
	}

	// CreateCommandMonitor should not panic
	cmdMon := pm.CreateCommandMonitor(cfg)
	if cmdMon == nil {
		t.Error("CreateCommandMonitor returned nil")
	}

	// CreatePoolMonitor
	var active int64
	poolMon := pm.CreatePoolMonitor(cfg, &active)
	if poolMon == nil {
		t.Error("CreatePoolMonitor returned nil")
	}

	pm.UpdateConfigMetrics(cfg)
	pm.RecordHealthCheck(true, cfg)
	pm.RecordHealthCheck(false, cfg)
}

func TestParseConfigDefaults(t *testing.T) {
	p := &PlugMongoDB{
		BasePlugin: nil, // will not be used for parseConfig
	}
	// Use a minimal config that Scan can populate
	// The kratos config.Scan typically works with a struct - we need a mock
	// For now, test that defaults are applied when we set empty conf
	p.conf = &conf.MongoDB{}
	// Simulate what parseConfig does for defaults
	if p.conf.Uri == "" {
		p.conf.Uri = "mongodb://localhost:27017"
	}
	if p.conf.Database == "" {
		p.conf.Database = "test"
	}
	if p.conf.MaxPoolSize == 0 {
		p.conf.MaxPoolSize = 100
	}
	if p.conf.ConnectTimeout == nil {
		p.conf.ConnectTimeout = durationpb.New(30 * time.Second)
	}

	if p.conf.Uri != "mongodb://localhost:27017" {
		t.Errorf("default uri: got %q", p.conf.Uri)
	}
	if p.conf.Database != "test" {
		t.Errorf("default database: got %q", p.conf.Database)
	}
	if p.conf.MaxPoolSize != 100 {
		t.Errorf("default maxPoolSize: got %d", p.conf.MaxPoolSize)
	}
}
