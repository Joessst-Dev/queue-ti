# C# Client

[![NuGet](https://img.shields.io/nuget/v/QueueTi.Client)](https://www.nuget.org/packages/QueueTi.Client)

A proto-first gRPC client library for queue-ti. Targets `.NET 8.0`+.

- **Repository**: [github.com/Joessst-Dev/queue-ti-csharp-client](https://github.com/Joessst-Dev/queue-ti-csharp-client)
- **NuGet**: [nuget.org/packages/QueueTi.Client](https://www.nuget.org/packages/QueueTi.Client)

## Installation

```bash
dotnet add package QueueTi.Client
```

Or via the Package Manager Console:

```powershell
Install-Package QueueTi.Client
```

## Quick Start

```csharp
using QueueTi;
using System.Text;

await using var client = QueueTiClient.Create("https://queue.example.com", new QueueTiClientOptions
{
    BearerToken = "your-jwt-token"  // optional
});

// Publish a message
var producer = client.NewProducer();
string id = await producer.PublishAsync(
    "orders", Encoding.UTF8.GetBytes("Hello!"), ct: CancellationToken.None);

// Consume messages — ConsumeAsync blocks until the token is cancelled.
using var cts = new CancellationTokenSource();
var consumer = client.NewConsumer("orders", new ConsumerOptions { ConsumerGroup = "billing" });
await consumer.ConsumeAsync(async (msg, ct) =>
{
    Console.WriteLine($"[{msg.Id}] {Encoding.UTF8.GetString(msg.Payload)}");
    // Automatically acked on return; nacked if an exception is thrown.
}, cts.Token);
// Client is disposed by the await using block above.
```

## Client Creation

### Factory method (recommended)

```csharp
var client = QueueTiClient.Create("https://queue.example.com", new QueueTiClientOptions
{
    BearerToken = "jwt-token",           // optional; enables Bearer auth
    Insecure = false,                    // set true for plaintext http:// endpoints
    ConfigureChannel = opts => {
        opts.MaxReceiveMessageSize = 16 * 1024 * 1024;
    }
});
```

### Manual channel (advanced)

```csharp
var channel = GrpcChannel.ForAddress("https://queue.example.com");
var grpcClient = new QueueService.QueueServiceClient(channel);
var client = new QueueTiClient(grpcClient, new QueueTiClientOptions());
```

When using a manual channel you are responsible for adding any interceptors — `BearerToken` is not automatically applied.

### Dependency Injection (ASP.NET Core)

`AddQueueTiClient` registers `QueueTiClient` as a singleton. Inject it into controllers, services, or minimal API handlers:

```csharp
builder.Services.AddQueueTiClient("https://queue.example.com", opts =>
{
    opts.BearerToken = "initial-token";
    opts.TokenRefresher = async ct => await GetFreshTokenAsync(ct);
});

app.MapPost("/orders", async ([FromServices] QueueTiClient client) =>
{
    var id = await client.NewProducer().PublishAsync("orders", orderPayload);
    return Results.Created($"/orders/{id}", new { id });
});
```

## Publishing Messages

```csharp
var producer = client.NewProducer();

// Minimal
string id = await producer.PublishAsync("orders", payload, ct: ct);

// With routing key and metadata
string id = await producer.PublishAsync("orders", payload, new PublishOptions
{
    Key = "order-123",
    Metadata = new Dictionary<string, string> { ["source"] = "api" }
}, ct);
```

### PublishOptions

| Property | Type | Description |
|----------|------|-------------|
| `Key` | `string?` | Optional routing or ordering key. |
| `Metadata` | `IDictionary<string, string>?` | Arbitrary key-value pairs attached to the message. |

## Consuming Messages

### Streaming consumer (real-time)

```csharp
var consumer = client.NewConsumer("orders", new ConsumerOptions
{
    ConsumerGroup = "billing",
    Concurrency = 4,
    VisibilityTimeoutSeconds = 30
});

await consumer.ConsumeAsync(async (msg, ct) =>
{
    var order = JsonSerializer.Deserialize<Order>(msg.Payload);
    await BillingService.ProcessAsync(order, ct);
    // Acked automatically on success; nacked automatically on exception.
}, ct);
```

The consumer reconnects with exponential backoff (500 ms → 30 s) on gRPC errors.

### Batch consumer (polling)

```csharp
var consumer = client.NewConsumer("orders");

await consumer.ConsumeBatchAsync(
    batchSize: 10,
    handler: async (messages, ct) =>
    {
        foreach (var msg in messages)
        {
            try
            {
                await ProcessAsync(msg, ct);
                await msg.AckAsync(ct);   // must ack manually
            }
            catch (Exception ex)
            {
                await msg.NackAsync(ex.Message, ct);  // must nack manually
            }
        }
    },
    ct: ct
);
```

### ConsumerOptions

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `ConsumerGroup` | `string` | `""` | Consumer group name for independent per-group consumption. |
| `Concurrency` | `int` | `1` | Max concurrent handler invocations (streaming mode only). |
| `VisibilityTimeoutSeconds` | `uint?` | `null` | Override server default visibility timeout. |

## Message Fields

| Property | Type | Description |
|----------|------|-------------|
| `Id` | `string` | Unique message identifier. |
| `Topic` | `string` | Topic name. |
| `Payload` | `byte[]` | Message body. |
| `Metadata` | `IReadOnlyDictionary<string, string>` | Key-value metadata. |
| `CreatedAt` | `DateTimeOffset` | Server timestamp when the message was created. |
| `RetryCount` | `int` | Number of previous delivery attempts. |

## Bearer Token Authentication

### Static token

```csharp
var client = QueueTiClient.Create(address, new QueueTiClientOptions
{
    BearerToken = "eyJhbGc..."
});
```

### Dynamic token refresh

```csharp
var client = QueueTiClient.Create(address, new QueueTiClientOptions
{
    BearerToken = "initial-token",
    TokenRefresher = async ct =>
    {
        var resp = await httpClient.GetAsync("/auth/refresh", ct);
        var json = await resp.Content.ReadAsStringAsync(ct);
        return JsonDocument.Parse(json).RootElement.GetString("access_token")!;
    }
});
```

The refresher is called ~60 seconds before token expiry and retries with exponential backoff on failure.

### Update token at runtime

```csharp
client.SetToken("new-jwt-token");  // thread-safe; immediate effect
```

## Configuration Reference

### QueueTiClientOptions

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `BearerToken` | `string?` | `null` | Initial JWT for Bearer authentication. |
| `TokenRefresher` | `Func<CancellationToken, Task<string>>?` | `null` | Async callback to refresh the bearer token. |
| `Insecure` | `bool` | `false` | Use plaintext `http://` (disables TLS). |
| `ConfigureChannel` | `Action<GrpcChannelOptions>?` | `null` | Callback to configure gRPC channel options. |
| `ConfigureHttpClientBuilder` | `Action<IHttpClientBuilder>?` | `null` | DI only: configure the `IHttpClientBuilder`. |

## Disposal

```csharp
await client.DisposeAsync();  // preferred
client.Dispose();             // synchronous; both are idempotent
```

Disposal cancels the background token refresh task and shuts down the managed gRPC channel.

## Thread Safety

`QueueTiClient`, `Producer`, and `Consumer` are all thread-safe. `SetToken()` is safe to call from any thread and takes effect immediately.

## .NET Aspire Integration

Two additional packages provide first-class [.NET Aspire](https://learn.microsoft.com/dotnet/aspire/get-started/aspire-overview) support:

| Package | Project type | Purpose |
|---------|-------------|---------|
| `QueueTi.Aspire.Hosting` | AppHost | Adds QueueTi as a container resource in the Aspire orchestrator |
| `QueueTi.Client.Aspire` | Service / worker | Registers the client, health checks, and OTel tracing via a single call |

### Installation

**AppHost project:**

```bash
dotnet add package QueueTi.Aspire.Hosting
```

**Service or worker project:**

```bash
dotnet add package QueueTi.Client.Aspire
```

### AppHost Setup

```csharp
// Program.cs — Aspire AppHost project
using QueueTi.Aspire.Hosting;

var builder = DistributedApplication.CreateBuilder(args);

var postgres = builder.AddPostgres("postgres")
    .AddDatabase("queueti-db");

var queue = builder.AddQueueTi("queue")
    .WithNpgsqlDatabase(postgres)
    .WithAuthentication(
        username: "admin",
        password: builder.AddParameter("queue-password", secret: true),
        jwtSecret: builder.AddParameter("queue-jwt-secret", secret: true))
    .WithLogLevel("info");

builder.AddProject<Projects.MyWorker>("worker")
    .WithReference(queue);

builder.Build().Run();
```

To run multiple replicas in development, chain `WithReplicas` and `WithRedis` so all instances share rate-limiting and cache state:

```csharp
var redis = builder.AddRedis("redis");

var queue = builder.AddQueueTi("queue")
    .WithReplicas(3)
    .WithNpgsqlDatabase(postgres)
    .WithRedis(redis)
    .WithAuthentication(...);
```

**Builder methods:**

| Method | Description |
|--------|-------------|
| `AddQueueTi(name, grpcPort?, httpPort?, tag?)` | Adds a QueueTi container resource. Pulls `ghcr.io/joessst-dev/queue-ti`. Exposes endpoints `grpc` (50051) and `http` (8080). |
| `WithNpgsqlDatabase(database)` | Wires an Npgsql database resource. Sets `QUEUETI_DB_*` env vars and adds a `WaitFor` dependency so QueueTi starts only after the database is healthy. |
| `WithAuthentication(username, password, jwtSecret)` | Enables auth. Sets `QUEUETI_AUTH_ENABLED` and related env vars. `username` is a plain string; `password` and `jwtSecret` accept `ParameterResource` values for secrets. |
| `WithReplicas(count)` | Runs `count` instances of the container. When using more than one replica, wire a Redis resource with `WithRedis` to keep rate-limiting and cache state consistent across instances. |
| `WithLogLevel(level)` | Sets `QUEUETI_LOG_LEVEL`. |

### Service Project Setup

`QueueTi.Client.Aspire` provides `AddQueueTiClient` on `IHostApplicationBuilder`. This is distinct from the [non-Aspire `IServiceCollection` extension](#dependency-injection-aspnet-core) documented earlier.

```csharp
// Program.cs — Service or worker project
builder.AddQueueTiClient("queue");

var app = builder.Build();
app.Run();
```

`AddQueueTiClient` reads the connection string from `ConnectionStrings:queue` (injected by Aspire via `WithReference`) and automatically:

- Registers `QueueTiClient` as a singleton in DI
- Registers a health check (`GET /healthz` on port 8080, no auth required) under tags `live` and `queueti`
- Instruments outbound gRPC calls with OpenTelemetry tracing via `OpenTelemetry.Instrumentation.GrpcNetClient`

**With custom settings:**

```csharp
builder.AddQueueTiClient("queue", settings =>
{
    settings.DisableHealthChecks = true;  // if health checks are managed elsewhere
    settings.BearerToken = "your-jwt";    // if auth is enabled on the server
});
```

### QueueTiClientSettings

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `ConnectionString` | `string?` | `null` | Explicit connection string. If unset, read from `ConnectionStrings:{name}` or `QueueTi:{name}` config, where `{name}` is the connection name passed to `AddQueueTiClient`. |
| `DisableHealthChecks` | `bool` | `false` | Skip automatic health check registration. |
| `DisableTracing` | `bool` | `false` | Skip OpenTelemetry instrumentation. |
| `BearerToken` | `string?` | `null` | Optional bearer token for authentication. |
| `TokenRefresher` | `Func<CancellationToken, Task<string>>?` | `null` | Optional callback to refresh the bearer token at runtime. |

## Admin API

`AdminClient` wraps the QueueTi HTTP admin API (port 8080) for programmatic management of topic configurations, schemas, consumer groups, and stats. It ships in the same `QueueTi.Client` NuGet package.

### Setup

```csharp
using QueueTi;

// Factory method (manages its own HttpClient)
var admin = AdminClient.Create(
    baseUrl: "http://queue.example.com",
    options: new QueueTiClientOptions { BearerToken = "your-jwt-token" }
);

// Dependency injection
builder.Services.AddQueueTiAdminClient("http://queue.example.com", opts =>
{
    opts.BearerToken = "admin-token";
});
```

### Example: Topic Configuration

```csharp
// List all topic configs
var configs = await admin.ListTopicConfigsAsync();

// Create or replace a topic config
await admin.UpsertTopicConfigAsync("orders", new TopicConfig(
    Topic: "orders",
    Replayable: true,
    MaxRetries: 3,
    MessageTtlSeconds: 86400,
    MaxDepth: 10000
));

// Delete a topic config
await admin.DeleteTopicConfigAsync("orders");
```

### Error Handling

```csharp
try
{
    await admin.DeleteTopicConfigAsync("nonexistent");
}
catch (QueueTiNotFoundException ex)
{
    // HTTP 404 — resource does not exist
}
catch (QueueTiConflictException ex)
{
    // HTTP 409 — resource already exists
}
```

### Full API

The `AdminClient` covers:
- **Topic configs**: `ListTopicConfigsAsync()`, `UpsertTopicConfigAsync(topic, config)`, `DeleteTopicConfigAsync(topic)`
- **Topic schemas**: `ListTopicSchemasAsync()`, `GetTopicSchemaAsync(topic)`, `UpsertTopicSchemaAsync(topic, schemaJson)`, `DeleteTopicSchemaAsync(topic)`
- **Consumer groups**: `ListConsumerGroupsAsync(topic)`, `RegisterConsumerGroupAsync(topic, group)`, `UnregisterConsumerGroupAsync(topic, group)`
- **Statistics**: `StatsAsync()`

`AdminClient` implements `IDisposable` and `IAsyncDisposable`. When created via `AdminClient.Create`, it owns its `HttpClient` and disposes it on cleanup.

For complete method signatures and DI details, see the [C# client README](https://github.com/Joessst-Dev/queue-ti-csharp-client/blob/main/README.md#admin-client).
