# VFMP Kubernetes Scaling Plan

## Problem Statement

VFMP currently runs with 3 replicas in Kubernetes, but each pod maintains **in-memory queues**. This creates a critical issue:

- Message published to **Pod A** → stored in Pod A's local queue
- Consumer connected to **Pod B** → cannot see messages from Pod A ❌

The LoadBalancer service distributes traffic randomly, so:
- Publishers may send to different pods
- Consumers may connect to different pods
- Messages and consumers are not coordinated

## Current Architecture

```
Publishers → LoadBalancer (random) → Pod 0 (local queues: topicA, topicB)
                                   → Pod 1 (local queues: topicC, topicD)
Consumers  → LoadBalancer (random) → Pod 2 (local queues: topicE, topicF)
```

**Result**: Fragmented queues, messages don't reach consumers.

---

## Solution Overview

All solutions aim to ensure that **messages and consumers for the same topic reach the same pod**.

### Core Challenge: Multi-Topic Consumption

If a client wants to consume from multiple topics (e.g., topicA + topicB), and those topics hash to different pods, we need to handle:
- Multiple backend connections
- Message multiplexing
- Or eliminate the need for routing entirely

---

## Solution 1: Session Affinity by Topic

### Concept
Use consistent hashing to route all traffic for a topic to the same pod.

### Implementation Options

#### A. Kubernetes Session Affinity
```yaml
apiVersion: v1
kind: Service
metadata:
  name: vfmp
spec:
  sessionAffinity: ClientIP
  sessionAffinityConfig:
    clientIP:
      timeoutSeconds: 3600
```

**Problem**: Routes by source IP, not by topic ❌

#### B. Ingress Controller with Hash-Based Routing
Use NGINX/Traefik with custom routing logic based on topic path or header.

**Problem**: Doesn't work for TCP connections ❌

### Verdict
❌ **Not viable** - Cannot route by topic at the service level without custom proxy.

---

## Solution 2: TCP/HTTP Router with Consistent Hashing

### Architecture
```
Publishers (HTTP) → HTTP Router (hash topic from path) → Pod 0
Consumers (TCP)   → TCP Router (hash topic from RDY)  → Pod 1
                                                       → Pod 2
```

### Implementation

#### HTTP Router
- Extract topic from `POST /messages/{topic}`
- Hash topic → pod index: `podIndex = hash(topic) % replicas`
- Forward request to `vfmp-{podIndex}.vfmp-headless:8080`

#### TCP Router (Single Topic Per Connection)
```go
// Reads first RDY message, routes connection to correct pod
func (r *TCPRouter) handleConnection(clientConn net.Conn) {
    // Read: "RDY topicA\n"
    firstMsg := readLine(clientConn)
    topic := parseRDY(firstMsg)

    // Connect to correct pod
    backendAddr := r.routeTopic(topic) // "vfmp-0.vfmp-headless:9090"
    backendConn := dial(backendAddr)

    // Forward RDY message
    backendConn.Write(firstMsg)

    // Bidirectional proxy
    go io.Copy(backendConn, clientConn) // Client → Backend
    io.Copy(clientConn, backendConn)     // Backend → Client
}
```

#### TCP Router (Multi-Topic, Smart Multiplexing)
```go
func (r *SmartTCPRouter) handleConnection(clientConn net.Conn) {
    backendConns := make(map[string]net.Conn) // topic → backend
    msgChan := make(chan []byte, 100)

    // Read from client
    go func() {
        for line := range readLines(clientConn) {
            if isRDY(line) {
                topic := parseTopic(line)

                // Get or create backend connection
                if !exists(backendConns[topic]) {
                    backendConns[topic] = dial(r.routeTopic(topic))
                    go readFromBackend(backendConns[topic], msgChan)
                }

                // Forward RDY to correct backend
                backendConns[topic].Write(line)
            } else if isACK(line) || isNCK(line) || isDLQ(line) {
                // Forward to correct backend based on topic
                topic := parseTopic(line)
                backendConns[topic].Write(line)
            }
        }
    }()

    // Forward messages from all backends to client
    for msg := range msgChan {
        clientConn.Write(msg)
    }
}
```

### Kubernetes Setup
```yaml
# StatefulSet for predictable pod names
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: vfmp
spec:
  serviceName: vfmp-headless
  replicas: 3
  # Creates: vfmp-0, vfmp-1, vfmp-2

---
# Headless service for direct pod access
apiVersion: v1
kind: Service
metadata:
  name: vfmp-headless
spec:
  clusterIP: None
  selector:
    app: vfmp
  ports:
  - port: 8080
    name: http
  - port: 9090
    name: tcp

---
# Router deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vfmp-router
spec:
  replicas: 2  # Can scale routers independently
  template:
    spec:
      containers:
      - name: router
        image: vfmp-router:latest
        env:
        - name: VFMP_REPLICAS
          value: "3"
        - name: VFMP_SERVICE
          value: "vfmp-headless"

---
# External LoadBalancer points to router
apiVersion: v1
kind: Service
metadata:
  name: vfmp-external
spec:
  selector:
    app: vfmp-router
  type: LoadBalancer
  ports:
  - port: 8080
    name: http
  - port: 9090
    name: tcp
```

### Multi-Topic Consumption Options

#### Option 2A: Multiple Connections Per Client
**Client opens one TCP connection per topic**

```go
// Client side
for _, topic := range topics {
    conn := dial("vfmp:9090")
    go consumeTopic(conn, topic) // Each in own goroutine
}
```

**Pros**:
- ✅ Simple router (unchanged from single-topic version)
- ✅ Natural pattern

**Cons**:
- ❌ More connections (100 topics = 100 connections)
- ❌ Requires client changes

#### Option 2B: Smart Router with Multiplexing
**Router maintains multiple backend connections, multiplexes to client**

See smart router code above.

**Pros**:
- ✅ Transparent to clients (no changes)
- ✅ Single connection from client perspective

**Cons**:
- ❌ Complex router logic
- ❌ Router is stateful
- ❌ Potential bottleneck

### Pros/Cons

**Pros**:
- ✅ Keep in-memory architecture
- ✅ Low latency (one proxy hop)
- ✅ Works with existing VFMP code
- ✅ ~150-300 lines of Go code

**Cons**:
- ❌ Additional component to maintain
- ❌ Router is potential single point of failure
- ❌ Complex if supporting multi-topic consumption

### Effort
- Simple version (single topic): **1-2 days**
- Smart multiplexing version: **3-5 days**

---

## Solution 3: Shared Backend Storage

### Concept
Replace in-memory queues with **Redis** or **PostgreSQL**. All pods access the same queues.

### Architecture
```
Publishers → Any Pod → Redis (shared queues)
Consumers  → Any Pod → Redis (shared queues)
```

No routing needed! Any pod can handle any topic.

### Implementation Options

#### Option 3A: Redis Lists
```go
type RedisQueue struct {
    client *redis.Client
    topic  string
}

func (q *RedisQueue) Append(m model.Message) {
    data := serialize(m)
    q.client.RPush(ctx, q.topic, data)
}

func (q *RedisQueue) Pop() (model.Message, error) {
    result := q.client.BLPop(ctx, 0, q.topic) // Blocking pop
    return deserialize(result[1]), nil
}

func (q *RedisQueue) Len() int {
    return int(q.client.LLen(ctx, q.topic).Val())
}
```

#### Option 3B: Redis Streams
More advanced, supports consumer groups:
```go
func (q *RedisQueue) Append(m model.Message) {
    q.client.XAdd(ctx, &redis.XAddArgs{
        Stream: q.topic,
        Values: map[string]interface{}{
            "body":          m.Body,
            "correlationID": m.CorrelationID,
        },
    })
}

func (q *RedisQueue) Consume(consumerGroup string) <-chan model.Message {
    // XREADGROUP for consumer group semantics
}
```

#### Option 3C: PostgreSQL
```sql
CREATE TABLE messages (
    id SERIAL PRIMARY KEY,
    topic VARCHAR(255) NOT NULL,
    correlation_id UUID NOT NULL,
    body BYTEA NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    locked_until TIMESTAMP,
    locked_by VARCHAR(255)
);

CREATE INDEX idx_topic_status ON messages(topic, status);
CREATE INDEX idx_locked_until ON messages(locked_until) WHERE status = 'in_flight';
```

```go
func (q *PostgresQueue) Pop() (model.Message, error) {
    // Use SELECT FOR UPDATE SKIP LOCKED for efficient dequeue
    var msg model.Message
    err := q.db.QueryRow(`
        UPDATE messages
        SET status = 'in_flight',
            locked_by = $1,
            locked_until = NOW() + INTERVAL '10 seconds'
        WHERE id = (
            SELECT id FROM messages
            WHERE topic = $2 AND status = 'pending'
            ORDER BY created_at
            FOR UPDATE SKIP LOCKED
            LIMIT 1
        )
        RETURNING id, topic, correlation_id, body, created_at
    `, instanceID, topic).Scan(&msg.ID, &msg.Topic, ...)

    return msg, err
}
```

### Kubernetes Setup
```yaml
# Redis deployment
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis
spec:
  serviceName: redis
  replicas: 1
  template:
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        volumeMounts:
        - name: data
          mountPath: /data
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi

---
# VFMP uses Redis
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vfmp
spec:
  replicas: 3  # Can scale freely!
  template:
    spec:
      containers:
      - name: vfmp
        env:
        - name: QUEUE_BACKEND
          value: redis
        - name: REDIS_URL
          value: redis://redis:6379
```

### Changes to VFMP Code

1. **Create queue interface**:
```go
// internal/queue/queue.go
type Queue interface {
    Append(model.Message)
    Pop() (model.Message, error)
    Peek() (model.Message, error)
    Len() int
    GetMsgChan() <-chan model.Message
}
```

2. **Implement Redis queue**:
```go
// internal/queue/redis.go
type RedisQueue struct {
    client *redis.Client
    topic  string
}

// Implement Queue interface...
```

3. **Update broker**:
```go
// internal/broker/broker.go
func (b *Broker) getOrCreateTopic(ctx context.Context, topic string) Queue {
    b.mu.Lock()
    defer b.mu.Unlock()

    q, ok := b.topics[topic]
    if !ok {
        if b.config.Backend == "redis" {
            q = queue.NewRedis(ctx, topic, b.redisClient)
        } else {
            q = queue.NewInMemory(ctx, topic)
        }
        b.topics[topic] = q
    }
    return q
}
```

### Pros/Cons

**Pros**:
- ✅ True horizontal scaling (any pod handles any topic)
- ✅ Persistence (survive pod restarts)
- ✅ No routing complexity
- ✅ Multi-topic consumption works naturally
- ✅ Industry standard approach
- ✅ Can scale VFMP and storage independently

**Cons**:
- ❌ External dependency (Redis/PostgreSQL)
- ❌ Higher latency (~1-2ms network hop)
- ❌ More operational complexity
- ❌ Costs (storage, memory)

### Effort
- Redis Lists: **3-5 days**
- Redis Streams: **5-7 days**
- PostgreSQL: **5-7 days**

---

## Solution 4: Pod-to-Pod Communication

### Concept
Clients connect to any pod. If that pod doesn't own the topic, it fetches from the correct pod via gRPC/HTTP.

### Architecture
```
Consumer → Pod 1 → (topicA not local) → gRPC call → Pod 0 (has topicA) → Consumer
```

### Implementation
```go
func (b *Broker) NotifyReady(ctx context.Context, topic string) <-chan model.Message {
    // Check if this pod owns this topic
    if !b.ownsTopic(topic) {
        // Fetch from remote pod
        return b.fetchFromRemotePod(ctx, topic)
    }

    // Local queue
    q := b.getOrCreateTopic(ctx, topic)
    return q.GetMsgChan()
}

func (b *Broker) ownsTopic(topic string) bool {
    hash := crc32.ChecksumIEEE([]byte(topic))
    targetPod := int(hash % uint32(b.totalReplicas))
    return targetPod == b.podIndex
}

func (b *Broker) fetchFromRemotePod(ctx context.Context, topic string) <-chan model.Message {
    podAddr := b.getPodAddress(topic)

    // gRPC/HTTP call to remote pod
    conn := grpc.Dial(podAddr)
    client := vfmp.NewBrokerClient(conn)

    stream := client.Subscribe(ctx, &vfmp.SubscribeRequest{Topic: topic})

    msgChan := make(chan model.Message)
    go func() {
        for {
            msg, err := stream.Recv()
            if err != nil {
                close(msgChan)
                return
            }
            msgChan <- convertMessage(msg)
        }
    }()

    return msgChan
}
```

### Pros/Cons

**Pros**:
- ✅ No router needed
- ✅ No external dependencies
- ✅ Transparent to clients

**Cons**:
- ❌ Complex inter-pod communication
- ❌ Extra network hops (pod → pod → client)
- ❌ Requires gRPC/service mesh
- ❌ Difficult to debug
- ❌ Potential cascading failures

### Effort
**7-10 days**

### Verdict
❌ **Not recommended** - Too complex for the benefit.

---

## Solution 5: Single Replica

### Concept
Just run one pod. Simple!

```yaml
spec:
  replicas: 1
```

### Pros/Cons

**Pros**:
- ✅ Works immediately
- ✅ No complexity

**Cons**:
- ❌ No high availability
- ❌ Limited throughput
- ❌ Single point of failure

### Effort
**5 minutes**

### Verdict
✅ **Good for development/testing**, ❌ **not for production**

---

## Comparison Matrix

| Solution | Complexity | Latency | Scalability | HA | Multi-Topic | Persistence | Effort |
|----------|------------|---------|-------------|----|-----------|-----------|----|
| **Session Affinity** | Low | Low | ❌ Limited | ✅ Yes | ❌ No | ❌ No | N/A (doesn't work) |
| **TCP/HTTP Router** | Medium | Low | ⚠️ Good | ✅ Yes | ⚠️ Complex | ❌ No | 1-5 days |
| **Shared Backend (Redis)** | Medium | Medium | ✅ Excellent | ✅ Yes | ✅ Natural | ✅ Yes | 3-7 days |
| **Shared Backend (Postgres)** | Medium | Medium | ✅ Excellent | ✅ Yes | ✅ Natural | ✅ Yes | 5-7 days |
| **Pod-to-Pod** | High | High | ⚠️ Good | ✅ Yes | ✅ Natural | ❌ No | 7-10 days |
| **Single Replica** | None | Low | ❌ None | ❌ No | ✅ Natural | ❌ No | 5 min |

---

## Recommendations

### Phase 1 (Immediate): Single Replica
**For development/testing**
- Set `replicas: 1` in deployment
- Gets you running while you plan long-term solution
- **Time**: 5 minutes

### Phase 2 (Short-term): TCP/HTTP Router
**For production with current architecture**
- Implement consistent hashing router
- Keep in-memory queues
- Choose between:
  - **Simple version**: One topic per connection (client changes required)
  - **Smart version**: Multi-topic multiplexing (no client changes)
- **Time**: 1-5 days depending on complexity

### Phase 3 (Long-term): Shared Backend (Redis)
**For true production scale**
- Replace in-memory queues with Redis
- Eliminates routing complexity entirely
- Get persistence as bonus
- Industry standard approach
- **Time**: 3-5 days

### Recommended Path

```
Now:          Use single replica (replicas: 1)
              ↓
1-2 weeks:    Implement simple TCP/HTTP router
              ↓
1-2 months:   Migrate to Redis backend
              ↓
Future:       Consider Redis Streams for advanced features
```

---

## Decision Factors

### Choose **TCP/HTTP Router** if:
- ✅ Want to keep in-memory architecture
- ✅ Need solution quickly (1-5 days)
- ✅ Don't need persistence
- ✅ Clients can handle multiple connections OR you can build multiplexing router

### Choose **Shared Backend (Redis)** if:
- ✅ Want true horizontal scaling
- ✅ Need persistence
- ✅ Multi-topic consumption is common
- ✅ Willing to manage external dependency
- ✅ Building for production scale (This is what Kafka, RabbitMQ, NATS do)

### Choose **Single Replica** if:
- ✅ Development/testing only
- ✅ Low traffic
- ❌ NOT for production

---

## Next Steps

1. **Decide on timeline**: How quickly do you need multi-pod scaling?
2. **Evaluate trade-offs**: Complexity vs features vs effort
3. **Choose approach**: Router (quick) or Redis (robust)
4. **Implement**: Start with phase 1 or 2
5. **Test**: Load testing with multiple pods
6. **Monitor**: Metrics for queue lengths, routing distribution

---

## References

- Current deployment: `/deploy/k8s/base/deployment.yml` (3 replicas)
- Broker code: `/internal/broker/broker.go` (in-memory queues)
- TCP handler: `/internal/tcp/server.go` (RDY/ACK/NCK/DLQ protocol)
- HTTP handler: `/internal/http/handlers.go` (publish endpoint)

## Notes

- **HTTP long polling** was considered as alternative to TCP but rejected due to:
  - Higher latency (new request per message)
  - Connection overhead
  - Less efficient for high-throughput message streams
  - Raw TCP is the right choice for message queue systems

- **Consistent hashing** formula: `podIndex = crc32(topic) % replicas`
- **StatefulSet** provides stable pod names: `vfmp-0`, `vfmp-1`, `vfmp-2`
- **Headless service** enables direct pod access: `vfmp-0.vfmp-headless.default.svc.cluster.local`
