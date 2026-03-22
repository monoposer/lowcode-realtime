## MQ-based Realtime Service (Broadcast & Presence)

This project implements a realtime messaging service decoupled via message queues (MQ), with:

- **Broadcast**: channel-wide messages; clients may publish over WebSocket or external services over HTTP.
- **Presence**: per-channel online state with join, update, leave events and optional full list sync.

The stack uses **Redis** for persistence primitives and a **pluggable MQ** for broadcast:

- **Broadcast MQ (pluggable)**: default Redis Pub/Sub; Kafka, RabbitMQ, etc. can plug in via `MessageBroker`.
- **Presence store (Redis)**:
  - Redis Hash + TTL holds online users per channel.

Implemented in Go; WebSocket uses `gorilla/websocket`; HTTP API uses `gin`.

---

### 1. Architecture

- **WebSocket Gateway (`internal/gateway`)**
  - Manages connections, Ping/Pong heartbeats, and channel subscriptions.
  - Handles client `broadcast` and `presence` messages.
  - Publishes to MQ (`internal/broker`) so all instances receive events; each instance delivers to locally subscribed clients.

- **Presence store (`internal/presence`)**
  - Redis Hash per channel for online users:
    - Key: `presence:<channel>`
    - Field: `user_id`
    - Value: JSON metadata (`meta`, `updated_at`).
  - TTL evicts stale users without heartbeats.

- **HTTP Event API (`POST /api/event`)**
  - Lets backend services publish a channel event (broadcast payload) over HTTP.
  - Auth/authorization is left as an extension; validation then publish to MQ.

- **MQ abstraction (`internal/broker`)**
  - `MessageBroker` hides Redis / Kafka / RabbitMQ differences.
  - Current implementation: `RedisPubSubBroker` (Redis Pub/Sub).

---

### 2. Layout

- `cmd/server`
  - Entrypoint: HTTP + WebSocket server.
- `internal/broker`
  - `broker.go`: `MessageBroker` interface.
  - `factory.go`: composes Presence + broadcast plugins.
  - `redis_pubsub.go`: Redis Pub/Sub implementation; default broadcast plugin (`BROADCAST_MQ_TYPE=redis`).
- `internal/presence`
  - `store.go`: Redis Hash + TTL presence store.
- `internal/gateway`
  - `client.go`: per-connection read/write loops and heartbeat.
  - `jwt.go`: optional HS256 JWT validation for WebSocket; user id from claims.
  - `cors.go`: CORS middleware for cross-origin calls to `/api/event`.
  - `hub.go`: local clients and subscriptions.
  - `message.go`: client and internal bus message types.
  - `server.go`: routes `/ws`, `POST /api/event`, and core logic.

---

### 3. Data shapes

#### 3.1 Client messages (WebSocket)

Clients send JSON over WebSocket:

- **Subscribe**

```json
{
  "type": "subscribe",
  "channel": "room1",
  "listen": ["broadcast", "presence"]
}
```

- **Broadcast**

```json
{
  "type": "broadcast",
  "channel": "room1",
  "event": "user-message",
  "payload": { "text": "hello" }
}
```

- **Presence join / update / leave**

```json
{
  "type": "presence_join",
  "channel": "room1",
  "payload": { "name": "Alice" }
}
```

```json
{
  "type": "presence_update",
  "channel": "room1",
  "payload": { "name": "Alice", "status": "typing" }
}
```

```json
{
  "type": "presence_leave",
  "channel": "room1"
}
```

#### 3.2 Internal `BusMessage` (MQ)

Defined in `internal/gateway/message.go`:

- `BusMessage`:
  - `Type`: `broadcast` / `presence_join` / `presence_update` / `presence_leave`
  - `ChannelID`: channel id
  - `UserID`: user who triggered the event
  - `Event`: broadcast event name
  - `Payload`: broadcast body
  - `Meta`: presence payload (`PresenceMeta`)
  - `Timestamp`: Unix ms

#### 3.3 Presence Redis layout

- Key: `presence:<channel>`
- Field: `<user_id>`
- Example value:

```json
{
  "user_id": "user123",
  "meta": { "name": "Alice" },
  "updated_at": 1620000000
}
```

TTL is controlled by `PRESENCE_TTL_SECONDS` (default 60).

---

### 4. Flows

#### 4.1 Broadcast (client-originated)

1. Client A on gateway G1 sends `type=broadcast`.
2. G1 wraps a `BusMessage` and calls `MessageBroker.PublishBroadcast("room1", data)`.
3. Redis Pub/Sub delivers to all gateways subscribed to `broadcast.*`.
4. Each gateway looks up local subscribers for that channel with `listen` containing `broadcast`.
5. Push JSON to those clients.

#### 4.2 Presence join / update

1. Client B on G2 sends `presence_join` or `presence_update`.
2. G2 writes Redis Hash (`HSET presence:room1 user123 <json>`) and refreshes TTL.
3. G2 publishes `BusMessage` via `PublishPresenceEvent("room1", data)`.
4. All gateways forward presence events to clients subscribed to presence on that channel.
5. Clients update local presence UI.

#### 4.3 Presence full sync

1. Client subscribes with `listen` containing `"presence"`.
2. Gateway calls `List(channel)` on the presence store.
3. Sends a `presence_sync` payload to that client only.

#### 4.4 External HTTP broadcast

1. Caller:

```http
POST /api/event
Content-Type: application/json

{
  "channel": "room1",
  "event": "user-message",
  "payload": { "text": "hello from http" }
}
```

2. HTTP layer may enforce auth (extension point).
3. Convert to `BusMessage` and `PublishBroadcast`.
4. Same delivery path as client broadcast.

---

### 5. Deploy and run

#### 5.1 Requirements

- Go 1.22+
- Redis 6+ (standalone, Sentinel, or Cluster)

#### 5.2 Environment variables

On startup, `cmd/server` loads a **`.env` file in the current working directory** (via `godotenv`). Variables already set in the shell take precedence. If you run `go run ./cmd/server` from the repo root, place `.env` there; running from another directory will not find it unless you `cd` or set env vars manually.

- `SERVER_ADDR`: HTTP listen address, default `:8080`.
- `REDIS_ADDR`: Redis address, default `127.0.0.1:6379`.
- `REDIS_PASSWORD`: optional.
- `REDIS_DB`: Redis DB index, default `0`.
- `PRESENCE_TTL_SECONDS`: presence TTL seconds, default `60`.
- `BROADCAST_MQ_TYPE`: broadcast backend name, default `redis` (extend with `kafka`, `rabbitmq`, etc.).
- `BROADCAST_MQ_ADDR` / `BROADCAST_MQ_PASSWORD` / `BROADCAST_MQ_DB`: connection for the broadcast plugin (Redis plugin; others may reuse or add keys).
- `CORS_ALLOWED_ORIGINS`: allowed `Origin` values for `POST /api/event`. Default `*` for dev; in production use a comma-separated list, e.g. `https://app.example.com`.
- `JWT_SECRET`: if set, WebSocket connections **must** present a valid HS256 JWT; the user id is read from JWT claims (not from `user_id` query). Send the token via `Authorization: Bearer <token>` or query `token` / `access_token` (browsers often use `?token=` on the WebSocket URL). If unset, the legacy `user_id` query parameter is accepted (dev only).
- `JWT_USER_CLAIM`: claim name for the user id when using JWT (default `sub`). Falls back to `user_id` or `sub` in the payload if needed.

#### 5.3 Run locally

```bash
go run ./cmd/server
```

#### 5.3.1 Docker Compose

The repo includes `Dockerfile` and `docker-compose.yml` starting **Redis** and the **realtime** service:

```bash
docker compose up --build
```

- **HTTP / WebSocket**: `http://localhost:8080` (`POST /api/event`, `/ws`)
- Redis exposed on host `6379` for debugging

`realtime` may set `CORS_ALLOWED_ORIGINS=*` for local use; tighten origins in production.

Inside containers: `REDIS_ADDR=redis:6379`, `BROADCAST_MQ_ADDR=redis:6379`. Override via `docker-compose.yml` `realtime.environment` or `docker compose --env-file .env up`.

#### 5.4 WebSocket example

- URL: `ws://localhost:8080/ws?user_id=user123`
- Use a browser or CLI WebSocket client for testing.

---

### 6. Extending MQ (Kafka / RabbitMQ)

`internal/broker.MessageBroker` abstracts:

- `PublishBroadcast` / `SubscribeBroadcast`
- `PublishPresenceEvent` / `SubscribePresenceEvent`

To add Kafka or RabbitMQ:

1. Add `kafka.go` / `rabbitmq.go` under `internal/broker`.
2. Map `channel` to topics/exchanges/routing keys as needed.
3. Wire the chosen implementation in `cmd/server/main.go` into `gateway.Server`.

---

### 7. Security (extensions)

- **WebSocket**: If `JWT_SECRET` is set, the server validates an HS256 JWT on connect and sets `Client.UserID` from claims (`JWT_USER_CLAIM`, default `sub`). Pass the token as `Authorization: Bearer …` or `?token=` / `?access_token=` on the upgrade request. If `JWT_SECRET` is **not** set, `user_id` may be passed as a query parameter (dev only).
- **`POST /api/event`**: Still unauthenticated in this sample; add API keys, JWT, or mTLS in front as needed.

---

### 8. Ops and HA

- **Metrics**: connections/sec, messages, Redis latency, MQ lag; integrate Prometheus/Grafana.
- **Scale out**: gateway is stateless; put a load balancer in front (Envoy, ALB, etc.).
- **Redis HA**: Sentinel or Cluster.

---

### 9. Summary

This service provides an **MQ-abstracted** realtime layer with:

- Horizontal scaling
- Channel broadcast
- Presence sync
- External HTTP broadcast API

Swap MQ implementations behind `MessageBroker` as your throughput and reliability needs grow.
