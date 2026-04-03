# MongoDB Plugin for Lynx Framework

The MongoDB plugin provides complete MongoDB integration support for the Lynx framework, including document storage, queries, aggregation, and other functionalities.

## Features

- ✅ **Complete MongoDB client support**
- ✅ **Connection pool management**
- ✅ **Multiple authentication methods**
- ✅ **TLS/SSL secure connections**
- ✅ **Read/Write concern configuration**
- ✅ **Automatic retry mechanisms**
- ✅ **Health checks**
- ✅ **Metrics monitoring**
- ✅ **Hot configuration updates**

## Quick Start

### 1. Configuration

Add MongoDB configuration in `config.yml`:

```yaml
lynx:
  mongodb:
    uri: "mongodb://localhost:27017"
    database: "test"
    username: "admin"
    password: "password"
    auth_source: "admin"
    max_pool_size: 100
    min_pool_size: 5
    connect_timeout: "30s"
    server_selection_timeout: "30s"
    socket_timeout: "30s"
    heartbeat_interval: "10s"
    enable_metrics: true
    enable_health_check: true
    health_check_interval: "30s"
    enable_tls: false
    enable_compression: true
    enable_retry_writes: true
    enable_read_concern: true
    read_concern_level: "local"
    enable_write_concern: true
    write_concern_w: 1
    write_concern_timeout: "5s"
```

### Configuration Reference

| Field | Proto Type | Default Value | Example | Notes |
|-------|------------|---------------|---------|-------|
| `uri` | `string` | `"mongodb://localhost:27017"` | `"mongodb://mongo-1:27017,mongo-2:27017/?replicaSet=rs0"` | MongoDB connection URI. |
| `database` | `string` | `"test"` | `"myapp"` | Default database name returned by `GetMongoDBDatabase()`. |
| `username` | `string` | `""` | `"admin"` | Username for credential-based authentication. |
| `password` | `string` | `""` | `"password"` | Password for credential-based authentication. |
| `auth_source` | `string` | `""` | `"admin"` | Authentication database used with `username` and `password`. |
| `max_pool_size` | `uint64` | `100` | `100` | Maximum MongoDB driver pool size. |
| `min_pool_size` | `uint64` | `5` | `5` | Minimum MongoDB driver pool size. |
| `connect_timeout` | `google.protobuf.Duration` | `"30s"` | `"30s"` | Initial connection timeout. |
| `server_selection_timeout` | `google.protobuf.Duration` | `"30s"` | `"30s"` | Server selection timeout used by the MongoDB driver. |
| `socket_timeout` | `google.protobuf.Duration` | `"30s"` | `"30s"` | Socket read/write timeout. |
| `heartbeat_interval` | `google.protobuf.Duration` | `"10s"` | `"10s"` | Driver heartbeat interval. |
| `enable_metrics` | `bool` | `false` | `true` | Enables Prometheus metrics collection. |
| `enable_health_check` | `bool` | `false` | `true` | Starts background health checks. |
| `health_check_interval` | `google.protobuf.Duration` | `"30s"` | `"30s"` | Interval used by background health checks. |
| `enable_tls` | `bool` | `false` | `true` | Enables TLS setup for the MongoDB client. |
| `tls_cert_file` | `string` | `""` | `"/etc/ssl/mongodb/client.pem"` | Optional client certificate path. |
| `tls_key_file` | `string` | `""` | `"/etc/ssl/mongodb/client-key.pem"` | Optional client key path. |
| `tls_ca_file` | `string` | `""` | `"/etc/ssl/mongodb/ca.pem"` | Optional CA certificate path. |
| `enable_compression` | `bool` | `false` | `true` | Enables `zlib` and `snappy` compressor negotiation. |
| `compression_level` | `int32` | `0` | `6` | Reserved field in the current implementation; compressor level is stored in config but not applied to driver options yet. |
| `enable_retry_writes` | `bool` | `false` | `true` | Enables retryable writes. |
| `enable_read_concern` | `bool` | `false` | `true` | Applies read concern to the client when enabled. |
| `read_concern_level` | `string` | `"local"` | `"majority"` | Supported values include `local`, `majority`, `linearizable`, and `snapshot`. |
| `enable_write_concern` | `bool` | `false` | `true` | Applies write concern to the client when enabled. |
| `write_concern_w` | `int32` | `1` | `1` | Write concern acknowledgement level. |
| `write_concern_timeout` | `google.protobuf.Duration` | `"5s"` | `"5s"` | Write concern timeout. |

### 2. Usage

```go
package main

import (
    "context"
    "log"
    "time"
    
    "github.com/go-lynx/lynx/app/boot"
    "github.com/go-lynx/lynx-mongodb"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
    // Start Lynx application
    boot.LynxApplication(wireApp).Run()
    
    // Get MongoDB client
    client := mongodb.GetMongoDB()
    if client == nil {
        log.Fatal("failed to get mongodb client")
    }
    
    // Get database
    db := mongodb.GetMongoDBDatabase()
    if db == nil {
        log.Fatal("failed to get mongodb database")
    }
    
    // Get collection
    collection := mongodb.GetMongoDBCollection("users")
    if collection == nil {
        log.Fatal("failed to get mongodb collection")
    }
    
    // Insert document
    insertDocument(collection)
    
    // Query documents
    findDocuments(collection)
    
    // Update document
    updateDocument(collection)
    
    // Delete document
    deleteDocument(collection)
}

func insertDocument(collection *mongo.Collection) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // Document data
    doc := bson.M{
        "name":      "Zhang San",
        "email":     "zhangsan@example.com",
        "age":       25,
        "created_at": time.Now(),
    }
    
    result, err := collection.InsertOne(ctx, doc)
    if err != nil {
        log.Printf("failed to insert document: %v", err)
        return
    }
    
    log.Printf("inserted document with ID: %v", result.InsertedID)
}

func findDocuments(collection *mongo.Collection) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // Query conditions
    filter := bson.M{"age": bson.M{"$gte": 20}}
    
    // Query options
    opts := options.Find().SetLimit(10)
    
    cursor, err := collection.Find(ctx, filter, opts)
    if err != nil {
        log.Printf("failed to find documents: %v", err)
        return
    }
    defer cursor.Close(ctx)
    
    var results []bson.M
    if err = cursor.All(ctx, &results); err != nil {
        log.Printf("failed to decode results: %v", err)
        return
    }
    
    log.Printf("found %d documents", len(results))
    for _, result := range results {
        log.Printf("document: %v", result)
    }
}

func updateDocument(collection *mongo.Collection) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // Query conditions
    filter := bson.M{"name": "Zhang San"}
    
    // Update content
    update := bson.M{
        "$set": bson.M{
            "age":        26,
            "updated_at": time.Now(),
        },
    }
    
    result, err := collection.UpdateOne(ctx, filter, update)
    if err != nil {
        log.Printf("failed to update document: %v", err)
        return
    }
    
    log.Printf("updated %d document", result.ModifiedCount)
}

func deleteDocument(collection *mongo.Collection) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    // Query conditions
    filter := bson.M{"name": "Zhang San"}
    
    result, err := collection.DeleteOne(ctx, filter)
    if err != nil {
        log.Printf("failed to delete document: %v", err)
        return
    }
    
    log.Printf("deleted %d document", result.DeletedCount)
}
```

## Configuration Options

See the proto-aligned table in the Quick Start section above for the complete field list, defaults, and examples sourced from `conf/mongodb.proto`.

## API Reference

### Get Client

```go
// Get MongoDB client
client := mongodb.GetMongoDB()

// Get database instance
db := mongodb.GetMongoDBDatabase()

// Get collection instance
collection := mongodb.GetMongoDBCollection("users")

// Get plugin instance
plugin := mongodb.GetMongoDBPlugin()

// Get connection statistics
stats := plugin.GetConnectionStats()

// Get Prometheus metrics gatherer (merge into /metrics endpoint)
if g := mongodb.GetMetricsGatherer(); g != nil {
    // metrics.RegisterGatherer(g) or merge with your Prometheus handler
}
```

### Plugin Options

```go
// Configure plugin using option pattern
plugin := mongodb.NewMongoDBClient(
    mongodb.WithURI("mongodb://localhost:27017"),
    mongodb.WithDatabase("myapp"),
    mongodb.WithCredentials("admin", "password", "admin"),
    mongodb.WithPoolSize(100, 5),
    mongodb.WithTimeouts(30*time.Second, 30*time.Second, 30*time.Second),
    mongodb.WithMetrics(true),
    mongodb.WithHealthCheck(true, 30*time.Second),
    mongodb.WithTLS(true, "cert.pem", "key.pem", "ca.pem"),
    mongodb.WithCompression(true, 6),
    mongodb.WithRetryWrites(true),
    mongodb.WithReadConcern(true, "local"),
    mongodb.WithWriteConcern(true, 1, 5*time.Second),
)
```

## Monitoring and Metrics

When `enable_metrics: true`, the plugin exposes Prometheus metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `lynx_mongodb_connection_pool_active` | Gauge | Active (checked-out) connections |
| `lynx_mongodb_connection_pool_max` | Gauge | Max pool size from config |
| `lynx_mongodb_active_connections` | Gauge | Same as connection_pool_active |
| `lynx_mongodb_operations_total` | Counter | Operations by type (find/insert/update/delete) |
| `lynx_mongodb_query_duration_seconds` | Histogram | Command latency |
| `lynx_mongodb_errors_total` | Counter | Failed operations |
| `lynx_mongodb_documents_processed_total` | Counter | Documents returned/modified |
| `lynx_mongodb_health_check_*` | Counter | Health check success/failure |

Use `GetMetricsGatherer()` or implement `MetricsGatherer()` on the plugin for Lynx lifecycle auto-registration.

## Health Checks

The plugin supports automatic health checks and can monitor:

- Database connection status
- Server availability
- Authentication status
- Query response time

## Error Handling

The plugin provides comprehensive error handling mechanisms:

- Automatic retry on connection failures
- Network timeout handling
- Authentication failure handling
- Cluster failover
- Read/Write concern error handling

## Best Practices

1. **Connection Pool Configuration**: Adjust connection pool size based on load
2. **Timeout Settings**: Reasonably set connection and operation timeout times
3. **Read/Write Concerns**: Configure appropriate read/write concern levels based on business requirements
4. **Index Optimization**: Create indexes for commonly queried fields
5. **Monitoring Alerts**: Enable metrics collection and health checks
6. **Security Configuration**: Use TLS and appropriate authentication methods

## Troubleshooting

### Common Issues

1. **Connection Failures**
   - Check server address and port
   - Verify network connectivity
   - Confirm firewall settings

2. **Authentication Failures**
   - Check username and password
   - Verify authentication database configuration
   - Confirm user permissions

3. **Performance Issues**
   - Adjust connection pool size
   - Optimize query statements
   - Check index configuration

## License

This project is licensed under the Apache License 2.0.
