# Java Client

A Java 21 gRPC client library for queue-ti, hosted at [Joessst-Dev/queue-ti-java-client](https://github.com/Joessst-Dev/queue-ti-java-client).

## Requirements

- Java 21 LTS or later
- Gradle 8+ or Maven 3.9+

## Installation

Releases are published to **GitHub Packages**. GitHub Packages requires authentication even for public repositories, so you need a Personal Access Token (PAT) with the `read:packages` scope.

### 1. Create a Personal Access Token

Go to **GitHub → Settings → Developer settings → Personal access tokens → Tokens (classic)** and generate a token with the `read:packages` scope.

### 2. Store credentials

**Gradle** — add to `~/.gradle/gradle.properties` (never commit this file):

```properties
gpr.user=your-github-username
gpr.key=ghp_xxxxxxxxxxxxxxxxxxxx
```

**Maven** — add to `~/.m2/settings.xml`:

```xml
<settings>
  <servers>
    <server>
      <id>github-queue-ti</id>
      <username>your-github-username</username>
      <password>ghp_xxxxxxxxxxxxxxxxxxxx</password>
    </server>
  </servers>
</settings>
```

### 3. Declare the repository and dependency

Replace `VERSION` with a release tag (e.g. `2026.05.0`). See [releases](https://github.com/Joessst-Dev/queue-ti-java-client/releases) for available versions.

**Gradle (Kotlin DSL)**

```kotlin
repositories {
    maven {
        url = uri("https://maven.pkg.github.com/Joessst-Dev/queue-ti-java-client")
        credentials {
            username = providers.gradleProperty("gpr.user").orNull
            password = providers.gradleProperty("gpr.key").orNull
        }
    }
}

dependencies {
    implementation("de.joesst.dev:queue-ti-java-client:VERSION")
}
```

**Gradle (Groovy)**

```groovy
repositories {
    maven {
        url 'https://maven.pkg.github.com/Joessst-Dev/queue-ti-java-client'
        credentials {
            username = findProperty('gpr.user')
            password = findProperty('gpr.key')
        }
    }
}

dependencies {
    implementation 'de.joesst.dev:queue-ti-java-client:VERSION'
}
```

**Maven**

```xml
<repositories>
    <repository>
        <id>github-queue-ti</id>
        <url>https://maven.pkg.github.com/Joessst-Dev/queue-ti-java-client</url>
    </repository>
</repositories>

<dependency>
    <groupId>de.joesst.dev</groupId>
    <artifactId>queue-ti-java-client</artifactId>
    <version>VERSION</version>
</dependency>
```

### Local build (no token required)

```bash
git clone https://github.com/Joessst-Dev/queue-ti-java-client.git
cd queue-ti-java-client
./gradlew publishToMavenLocal
```

Then use `mavenLocal()` as the repository and `1.0-SNAPSHOT` as the version.

## Quick Start

```java
import de.joesst.dev.queueti.*;
```

### Connect

```java
try (var client = QueueTiClient.connect("localhost:50051",
        ConnectOptions.builder().insecure(true).build())) {
    // use client...
}
```

`QueueTiClient` implements `Closeable`. Use try-with-resources to stop the background token-refresher thread and drain in-flight RPCs cleanly. Omit `.insecure(true)` in production — TLS is negotiated automatically.

### Publish a message

```java
var producer = client.newProducer();
String messageId = producer.publish("orders", "Hello".getBytes()).get();
```

With metadata and a deduplication key:

```java
var options = PublishOptions.builder()
    .metadata(Map.of("source", "checkout", "version", "1.0"))
    .key("order-123")
    .build();
producer.publish("orders", payload, options).thenAccept(id ->
    System.out.println("Published: " + id));
```

### Consume messages (streaming)

```java
var consumer = client.newConsumer("orders",
    ConsumerOptions.builder().concurrency(5).consumerGroup("invoicing").build());

consumer.consume(message -> {
    process(message.payload());
    return null; // null = ack; throw any exception to nack
});
```

`consume()` blocks until the calling thread is interrupted. Messages are dispatched on virtual threads. Automatic exponential-backoff reconnection (500ms–30s) handles stream failures.

### Consume messages (batch polling)

```java
consumer.consumeBatch(10, messages -> {
    for (var msg : messages) {
        if (isPoisonPill(msg)) {
            msg.nack("unprocessable").join();
        } else {
            process(msg.payload());
        }
    }
    return null; // unsettled messages are auto-acked on normal return
                 // throw to nack all unsettled messages instead
});
```

## Configuration

### ConnectOptions

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `insecure` | `boolean` | `false` | Use plaintext channel (no TLS) |
| `tls` | `TlsOptions` | `null` | Custom TLS configuration (custom CA, mTLS, server name override). Ignored when `insecure` is `true`. |
| `token` | `String` | `null` | Initial JWT sent on every request |
| `tokenRefresher` | `TokenRefresher` | `null` | Strategy to obtain fresh tokens |

### ConsumerOptions

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `concurrency` | `int` | `1` | Max messages dispatched concurrently |
| `consumerGroup` | `String` | `""` | Consumer group name |
| `visibilityTimeoutSeconds` | `Integer` | `null` | Visibility timeout; `null` uses server default |

### PublishOptions

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `metadata` | `Map<String, String>` | empty | Arbitrary key-value pairs attached to the message |
| `key` | `String` | `null` | Optional deduplication/routing key |

### BatchOptions

Override consumer group or visibility timeout for a single `consumeBatch` call:

```java
var batchOptions = BatchOptions.builder()
    .consumerGroup("batch-group")
    .visibilityTimeoutSeconds(30)
    .build();
consumer.consumeBatch(10, handler, batchOptions);
```

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `consumerGroup` | `String` | `""` | Consumer group for this batch poll |
| `visibilityTimeoutSeconds` | `Integer` | `null` | Visibility timeout for this batch poll |

## Token Refresh

```java
TokenRefresher refresher = () -> myAuthClient.fetchToken(); // returns CompletableFuture<String>

var options = ConnectOptions.builder()
    .token(initialJwt)       // required — refresher won't fire without a parseable token
    .tokenRefresher(refresher)
    .build();

try (var client = QueueTiClient.connect("localhost:50051", options)) {
    // ...
}
```

The client wakes a background virtual thread 60 seconds before token expiry. On failure it retries with exponential backoff (5s–60s). You can also update the token imperatively:

```java
client.setToken(newToken);
```

## TLS Configuration

### Default TLS (system CAs)

```java
try (var client = QueueTiClient.connect("myserver:50051",
        ConnectOptions.builder().build())) {
    // ...
}
```

### Custom CA certificate (self-signed server)

```java
import java.nio.file.Files;
import java.nio.file.Path;

byte[] caPem = Files.readAllBytes(Path.of("/path/to/ca.pem"));

try (var client = QueueTiClient.connect("myserver:50051",
        ConnectOptions.builder()
            .tls(TlsOptions.builder()
                .rootCertificates(caPem)
                .build())
            .build())) {
    // ...
}
```

### Mutual TLS (mTLS)

```java
try (var client = QueueTiClient.connect("myserver:50051",
        ConnectOptions.builder()
            .tls(TlsOptions.builder()
                .rootCertificates(Files.readAllBytes(Path.of("/path/to/ca.pem")))
                .privateKey(Files.readAllBytes(Path.of("/path/to/client-key.pem")))
                .certificateChain(Files.readAllBytes(Path.of("/path/to/client-cert.pem")))
                .build())
            .build())) {
    // ...
}
```

### Self-signed cert with hostname override

```java
try (var client = QueueTiClient.connect("localhost:50051",
        ConnectOptions.builder()
            .tls(TlsOptions.builder()
                .rootCertificates(Files.readAllBytes(Path.of("/path/to/ca.pem")))
                .serverNameOverride("myserver.internal")
                .build())
            .build())) {
    // ...
}
```

### TlsOptions

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `rootCertificates` | `byte[]` | `null` | PEM-encoded CA certificate(s); uses system CAs when `null`. |
| `privateKey` | `byte[]` | `null` | PEM-encoded client private key for mTLS (requires `certificateChain`). |
| `certificateChain` | `byte[]` | `null` | PEM-encoded client certificate chain for mTLS (requires `privateKey`). |
| `serverNameOverride` | `String` | `null` | Override the hostname used for TLS SNI/verification (useful with self-signed certs). |

## Message fields

| Field | Type | Description |
|-------|------|-------------|
| `id()` | `String` | Server-assigned message ID |
| `topic()` | `String` | Topic the message was published to |
| `payload()` | `byte[]` | Raw message bytes (defensive copy) |
| `metadata()` | `Map<String, String>` | Immutable metadata map |
| `createdAt()` | `Instant` | Enqueue timestamp; `Instant.EPOCH` if unavailable |
| `retryCount()` | `int` | Number of prior delivery attempts (0 on first delivery) |
| `maxRetries()` | `OptionalInt` | Server-configured max retries (batch only; empty for streaming) |

```java
message.ack();                    // CompletableFuture<Void>
message.nack("processing error"); // CompletableFuture<Void>
```

## Admin API

`AdminClient` provides programmatic management of topic configuration, schemas, consumer groups, and stats via the HTTP admin API on port 8080. It is separate from `QueueTiClient` so queue-only consumers carry no extra dependency surface.

### Setup

```java
// No auth (local / dev)
var admin = AdminClient.connect("http://localhost:8080", AdminOptions.defaults());

// With bearer token
var admin = AdminClient.connect("http://localhost:8080",
        AdminOptions.builder().token("eyJ...").build());
```

### AdminOptions

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `token` | `String` | `null` | Bearer token sent in every `Authorization` header |
| `requestTimeout` | `Duration` | 30s | Per-request HTTP timeout (must be positive) |

### Topic configuration

```java
List<TopicConfig> configs = admin.listTopicConfigs();

admin.upsertTopicConfig("orders", new TopicConfig(
        "orders",
        true,   // replayable
        3,      // maxRetries
        null, null, null, null));

admin.deleteTopicConfig("orders");
```

### TopicConfig fields

| Field | Type | Description |
|-------|------|-------------|
| `topic` | `String` | Topic name |
| `replayable` | `boolean` | Whether the topic supports message replay |
| `maxRetries` | `Integer` | Max delivery attempts; `null` = server default |
| `messageTtlSeconds` | `Integer` | Message TTL in seconds; `null` = server default |
| `maxDepth` | `Integer` | Max queue depth; `null` = server default |
| `replayWindowSeconds` | `Integer` | Replay window in seconds; `null` = server default |
| `throughputLimit` | `Integer` | Max messages per second; `null` = server default |

### Schema management

```java
List<TopicSchema> schemas = admin.listTopicSchemas();
TopicSchema schema = admin.getTopicSchema("orders"); // throws NotFoundException on 404

admin.upsertTopicSchema("orders", "{\"type\":\"string\"}");
admin.deleteTopicSchema("orders");
```

### TopicSchema fields

| Field | Type | Description |
|-------|------|-------------|
| `topic` | `String` | Topic name |
| `schemaJson` | `String` | The JSON Schema document |
| `version` | `int` | Schema version number |
| `updatedAt` | `String` | ISO-8601 timestamp of last update |

### Consumer groups

```java
List<String> groups = admin.listConsumerGroups("orders");

admin.registerConsumerGroup("orders", "billing");   // throws ConflictException if exists
admin.unregisterConsumerGroup("orders", "billing"); // throws NotFoundException if not found
```

### Stats

```java
List<TopicStat> stats = admin.stats();
// each entry: stat.topic(), stat.status(), stat.count()
```

### Exceptions

| Exception | HTTP status | Meaning |
|-----------|-------------|---------|
| `NotFoundException` | 404 | Resource does not exist |
| `ConflictException` | 409 | Resource already exists |
| `UncheckedIOException` | other / network | Unexpected error |

## Building from source

```bash
./gradlew build               # compile + all tests
./gradlew test                # tests only
./gradlew generateProto       # regenerate gRPC stubs from proto
./gradlew publishToMavenLocal # install to ~/.m2
```

Tests use JUnit 5 with an in-process gRPC server — no external server required.
