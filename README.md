# Chat Platform

A real-time chat platform built with Go and WebSockets.
This project demonstrate backend engineering practices such as observability, testing, authentication, and deployment.

## Architecture
Browser → Nginx Ingress → Go HTTP Server → Hub (goroutine) → PostgreSQL → Jaeger (traces) → Prometheus (metrics)

1. The core of the system is a Hub 
  - One goroutine that manages all clients and routes messages between them using Go channels. 
2. Each connected client has one goroutine for reading and one for writing.
3. Broadcasts are scoped per room. Messages are saved async to the database so never affects message delivery.

## Features

- Real-time messaging over WebSocket with per-room broadcast scoping
- JWT authentication on both HTTP endpoints and WebSocket upgrades
- Message history loaded from PostgreSQL on room join
- Exponential backoff reconnection on the frontend
- Structured JSON logging with `slog`
- Prometheus metrics: request count, request duration, active connections, messages per room
- OpenTelemetry distributed tracing with Jaeger
- Unit and integration tests with mock repositories
- Multi-stage Docker build
- Kubernetes manifests with rolling deploy strategy
- GitHub Actions CI pipeline: lint, test, build, push

## Tech Stack

- **Backend**: Go 1.23, gorilla/websocket, golang-jwt, bcrypt, pgx
- **Database**: PostgreSQL 16
- **Observability**: Prometheus, Jaeger, OpenTelemetry, slog
- **Infrastructure**: Docker, Kubernetes, GitHub Actions, GitHub Container Registry

## Running Locally

### Prerequisites

- Docker and Docker Compose
- Go 1.23+

### Start all services
```bash
docker-compose up --build -d
```

Open `http://localhost:8080` in your browser.

#### Debug DB
docker exec -it "container name" psql -U "db user" -d "db password"

### Environment Variables

Create `backend/.env` with the following:
- DB_HOST=localhost
- DB_PORT=5432
- DB_USER=your-db-user
- DB_PASSWORD=your-db-password
- DB_NAME=your-db-name
- JWT_SECRET=your-secret-here
- SERVER_ADDRESS=0.0.0.0:8080
- LOG_LEVEL=info
- OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317

### Running Tests
```bash
cd backend
go test -race ./...
```

### Observability

| Service    | URL                     | Purpose                  |
|------------|-------------------------|--------------------------|
| App        | http://localhost:8080   | Chat UI                  |
| Prometheus | http://localhost:9090   | Metrics           |
| Jaeger     | http://localhost:16686  | Traces       |

## Key Technical Decisions

**Hub pattern over mutex-protected map** — a single goroutine owns all client
state. No locks needed because only one goroutine ever reads or writes the
clients map. Channel communication is the synchronization mechanism.

**Async message persistence** — messages are saved to PostgreSQL in a
goroutine after broadcast. A slow or unavailable database never blocks
message delivery to connected clients.

**Interface-driven repositories** — `MessageRepo`, `UserRepo`, and `RoomRepo`
are interfaces, not concrete types. This makes the Hub and handlers testable
without a real database — tests use in-memory mocks.

**JWT on WebSocket upgrade** — browsers cannot set custom headers on WebSocket
connections. The token is passed as a query parameter over TLS, validated
before the upgrade handshake so invalid connections are rejected with a plain
HTTP 401 before the protocol switch occurs.

**`atomic.Int64` for connection counter** — the Prometheus gauge reads the
connected client count from a different goroutine than the Hub. Using
`sync/atomic` avoids a mutex and eliminates the data race without blocking
the Hub's event loop.

## Improvements

- [ ] Redis pub/sub
- [ ] Refresh tokens
- [ ] HorizontalPodAutoscaler
- [ ] Rate limiting
- [ ] TLS
- [ ] Room membership
