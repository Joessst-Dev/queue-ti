# C# Client

[![NuGet](https://img.shields.io/nuget/v/QueueTi.Client)](https://www.nuget.org/packages/QueueTi.Client)

A proto-first gRPC client library for queue-ti. Targets `.NET 10.0`.

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

// Create a client
var client = QueueTiClient.Create("https://queue.example.com", new QueueTiClientOptions
{
    BearerToken = "your-jwt-token"  // optional
});

// Publish a message
var producer = client.NewProducer();
string id = await producer.PublishAsync("orders", Encoding.UTF8.GetBytes("Hello!"), ct: ct);

// Consume messages
var consumer = client.NewConsumer("orders", new ConsumerOptions { ConsumerGroup = "billing" });
await consumer.ConsumeAsync(async (msg, ct) =>
{
    Console.WriteLine($"[{msg.Id}] {Encoding.UTF8.GetString(msg.Payload)}");
    // Automatically acked on return; nacked if an exception is thrown.
}, ct);

await client.DisposeAsync();
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

```csharp
builder.Services.AddQueueTiClient("https://queue.example.com", opts =>
{
    opts.BearerToken = "initial-token";
    opts.TokenRefresher = async ct => await GetFreshTokenAsync(ct);
});

// Inject QueueTiClient into controllers or minimal API handlers
app.MapPost("/orders", async (QueueTiClient client) =>
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
| `Metadata` | `IReadOnlyDictionary<string, string>?` | Arbitrary key-value pairs attached to the message. |

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
