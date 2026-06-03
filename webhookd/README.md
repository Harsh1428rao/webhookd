# webhookd

A scalable **Webhook Delivery System** built in **Go** — the backend engine that guarantees reliable event delivery to registered endpoints, with **exponential backoff retries**, **HMAC signature verification**, and a full delivery audit trail.

> Think of it as the system powering Stripe/GitHub webhooks — built from scratch.

## Tech Stack

- **Go (Golang)** — Concurrent delivery worker using goroutines
- **PostgreSQL** — Delivery state, attempt logs, endpoint registry
- **JWT** — Stateless authentication
- **HMAC-SHA256** — Webhook payload signing & verification
- **GitHub Actions** — CI/CD pipeline
- **Docker** — Containerized deployment
- **Render** — Cloud hosting

## How It Works

```
User registers endpoint URL
        ↓
User sends webhook (payload + event type)
        ↓
System queues delivery in PostgreSQL (status: pending)
        ↓
Background worker picks it up every 5 seconds
        ↓
HTTP POST → target URL with HMAC signature header
        ↓
Success (2xx)?  → mark delivered ✓
Failure?        → exponential backoff retry (1s → 2s → 4s → 8s → 16s)
Max attempts?   → mark failed, manual retry available
```

## API Endpoints

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/api/auth/register` | Register | No |
| POST | `/api/auth/login` | Login → JWT | No |
| POST | `/api/endpoints` | Register a target URL | Yes |
| GET | `/api/endpoints` | List your endpoints | Yes |
| DELETE | `/api/endpoints/{id}` | Remove endpoint | Yes |
| POST | `/api/webhooks/send` | Queue a webhook delivery | Yes |
| GET | `/api/webhooks` | List deliveries (filter by status) | Yes |
| GET | `/api/webhooks/{id}` | Get delivery details | Yes |
| GET | `/api/webhooks/{id}/attempts` | Full attempt log | Yes |
| POST | `/api/webhooks/{id}/retry` | Manually retry failed delivery | Yes |
| GET | `/health` | Health check | No |

## Exponential Backoff Schedule

| Attempt | Delay |
|---------|-------|
| 1 | 1 second |
| 2 | 2 seconds |
| 3 | 4 seconds |
| 4 | 8 seconds |
| 5 | 16 seconds |

After max attempts, delivery is marked **failed** and can be manually retried.

## Getting Started

```bash
# 1. Clone
git clone https://github.com/Harsh1428rao/webhookd.git
cd webhookd

# 2. Configure
cp .env.example .env
# Edit .env with your DB credentials and JWT secret

# 3. Install dependencies
go mod tidy

# 4. Run
go run cmd/server/main.go
```

Migrations run automatically on startup.

## Example Usage

### 1. Register & Login
```bash
curl -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"dev@example.com","password":"secret123"}'

# Response: { "token": "eyJ...", "user": {...} }
```

### 2. Register a Target Endpoint
```bash
curl -X POST http://localhost:8080/api/endpoints \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "My Payment Service",
    "url": "https://myapp.com/webhooks/payments",
    "secret": "my-signing-secret"
  }'
```

### 3. Send a Webhook
```bash
curl -X POST http://localhost:8080/api/webhooks/send \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "endpoint_id": 1,
    "event_type": "payment.completed",
    "payload": "{\"order_id\":\"ORD-001\",\"amount\":2999}",
    "max_attempts": 5
  }'

# Response: { "id": 1, "status": "pending", ... }
```

### 4. Check Delivery Status
```bash
curl http://localhost:8080/api/webhooks/1 \
  -H "Authorization: Bearer <token>"

# Response: { "status": "delivered", "attempt_count": 1, ... }
```

### 5. View All Retry Attempts
```bash
curl http://localhost:8080/api/webhooks/1/attempts \
  -H "Authorization: Bearer <token>"

# Response: [{ "status_code": 503, "duration_ms": 142, ... }, ...]
```

### HMAC Verification (on your receiving server)
```go
// Verify incoming webhook is from webhookd
mac := hmac.New(sha256.New, []byte("my-signing-secret"))
mac.Write([]byte(requestBody))
expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
received := r.Header.Get("X-Webhook-Signature")
// expected == received → authentic
```

## Project Structure

```
webhookd/
├── cmd/server/main.go              # Entry point, router, worker launch
├── internal/
│   ├── delivery/worker.go          # Background worker, exponential backoff
│   ├── handlers/handlers.go        # All HTTP handlers + JWT middleware
│   ├── db/db.go                    # PostgreSQL connection + migrations
│   └── models/models.go            # Data structs
├── Dockerfile
├── .github/workflows/ci.yml
├── .env.example
└── README.md
```

## Deployment (Render)

1. Push to GitHub
2. New **Web Service** on Render → connect repo
3. Build: `go build -o webhookd cmd/server/main.go`
4. Start: `./webhookd`
5. Add PostgreSQL on Render → copy `DATABASE_URL` to env vars
