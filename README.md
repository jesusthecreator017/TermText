# TermText

A terminal-native, real-time chat application written in Go — a modernized,
production-grade take on [TextTunnel](https://github.com/Abi-Liu/TextTunnel).
It pairs an HTTP + WebSocket server backed by PostgreSQL with a
[Bubble Tea](https://github.com/charmbracelet/bubbletea) TUI client, and adds
rooms & DMs, message history, full-text search, presence / typing / read
receipts, and **end-to-end encrypted direct messages**.

<!-- ![TermText chat screen](docs/images/chat.png) -->
<img width="1385" height="825" alt="Screenshot 2026-06-05 at 2 48 17 PM" src="https://github.com/user-attachments/assets/0471c7b9-a276-4260-b6c1-d1b2950480a4" />

---

## Features

- 🔐 **Auth** — argon2id password hashing, PASETO v4 session tokens, bearer-auth middleware.
- 💬 **Real-time chat** — a single-goroutine WebSocket hub with per-connection reader/writer goroutines, bounded send buffers, and ping/pong keepalive.
- 🗂️ **Rooms & DMs** — public rooms plus auto-created 2-person DM rooms.
- 📜 **History & search** — keyset-paginated scrollback and PostgreSQL full-text search (`tsvector` + GIN), scoped to your rooms.
- 👀 **Presence, typing & read receipts** — live online dots, "is typing…" indicators, and "seen by N".
- 🔒 **End-to-end encrypted DMs** — Curve25519 + NaCl `box`; the server only ever stores ciphertext. Private keys are encrypted at rest with a password-derived key (separate argon2id KDF) and TOFU fingerprint pinning.
- 🖥️ **Polished TUI** — rooms pane, fuzzy room switcher (Ctrl+K), help bar, spinner, and mouse-scroll.
- 📈 **Production hardening** — structured request logging with request IDs, panic recovery, Prometheus `/metrics`, per-IP rate limiting, a distroless Docker image, and GitHub Actions CI.

<!-- ![Room switcher](docs/images/switcher.png) -->
<img width="1512" height="982" alt="Screenshot 2026-06-05 at 2 50 33 PM" src="https://github.com/user-attachments/assets/6d479d16-5a85-4085-817d-29ee237207ae" />

---

## Architecture

```
┌─────────────────┐        HTTPS / WSS         ┌──────────────────────────┐
│  termtext (TUI) │ ◀────────────────────────▶ │   server (HTTP + WS hub)  │
│  Bubble Tea     │   REST: auth, rooms,       │                          │
│  - viewport     │   history, search, keys    │   ┌──────────────────┐   │
│  - textarea     │                            │   │  Hub (goroutine) │   │
│  - list/help    │   WS: messages, presence,  │   │  userID → conns  │   │
│  E2EE on client │   typing, read receipts    │   └──────────────────┘   │
└─────────────────┘                            │   Router → PostgreSQL    │
                                               └──────────────────────────┘
                                                          │
                                                   ┌──────────────┐
                                                   │  PostgreSQL  │
                                                   │ sqlc + goose │
                                                   └──────────────┘
```


### Project layout

```
cmd/
  server/          # HTTP + WebSocket server entrypoint
  termtext/        # Bubble Tea TUI client entrypoint
internal/
  auth/            # argon2id hashing, PASETO token issue/verify
  client/          # TUI's HTTP+WS client, session & key storage
  crypto/          # E2EE: keypairs, NaCl box, password-encrypted private keys
  db/
    schema/        # goose migrations
    queries/       # sqlc query definitions
    sqlc/          # sqlc-generated, type-safe DB code
  hub/             # WebSocket hub + per-connection client
  metrics/         # Prometheus collectors
  server/          # config, router, middleware, message router, handlers
  tui/             # Bubble Tea models, views, styles, keybindings
  uid/             # pgtype.UUID ↔ string helpers
  wsproto/         # shared WebSocket envelope types
```

---

## Tech stack

| Layer | Choice |
|---|---|
| Language | Go 1.25 |
| TUI | bubbletea · bubbles · lipgloss |
| WebSocket | `github.com/coder/websocket` |
| DB driver | `jackc/pgx/v5` |
| Queries / migrations | `sqlc` · `pressly/goose` |
| Auth | argon2id (`golang.org/x/crypto`) · PASETO v4 (`aidanwoods.dev/go-paseto`) |
| E2EE | Curve25519 + NaCl `box` (`golang.org/x/crypto/nacl`) |
| Config | `caarlos0/env` |
| Metrics | `prometheus/client_golang` |
| Rate limiting | `golang.org/x/time/rate` |
| Deploy | Docker (distroless) · Fly.io |

---

## Getting started

### Prerequisites

- Go 1.25+
- Docker (for the local Postgres via `docker compose`)
- [`sqlc`](https://docs.sqlc.dev) and [`goose`](https://github.com/pressly/goose) for DB codegen/migrations

```bash
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go install github.com/pressly/goose/v3/cmd/goose@latest
```

### 1. Configure environment

Copy the example env and fill it in:

```bash
cp .env.example .env
```

`.env` needs:

```bash
DB_URL=postgres://termtext:termtext_dev@localhost:5432/termtext?sslmode=disable
PORT=:8080
PASETO_KEY=<64 hex chars>   # generate with: openssl rand -hex 32
```

### 2. Start Postgres and migrate

```bash
make db-up        # docker compose up postgres
make migrate-up   # goose migrations
```

### 3. Run the server

```bash
make run          # builds and runs ./bin/main, listening on :8080
```

### 4. Run the client(s)

In one or more separate terminals:

```bash
go run ./cmd/termtext --server http://localhost:8080
# or, after `make build-client`:
./bin/termtext --server http://localhost:8080
```

Sign up two users (press **Ctrl+T** on the login screen to toggle sign-up),
then chat. Open a DM with `/dm <username>`.

<!-- ![Login screen](docs/images/login.png) -->
<img width="1508" height="951" alt="Screenshot 2026-06-05 at 2 51 52 PM" src="https://github.com/user-attachments/assets/ea4ca215-7546-4545-8a23-3de16c46354c" />

---

## Using the TUI

| Key / command | Action |
|---|---|
| `enter` | Send message |
| `ctrl+k` | Open room switcher (type to filter) |
| `pgup` / `pgdn`, mouse wheel | Scroll history (loads older messages at the top) |
| `/room <name>` | Create a room |
| `/dm <username>` | Open an encrypted DM |
| `/search <text>` | Full-text search your rooms |
| `ctrl+l` | Sign out |
| `ctrl+c` | Quit |

<img width="1132" height="617" alt="Screenshot 2026-06-05 at 2 52 45 PM" src="https://github.com/user-attachments/assets/2868578f-0880-418e-a8a1-e1ceb7364082" />

---

## HTTP API

| Method & path | Auth | Description |
|---|---|---|
| `POST /signup` | — | Create account, returns token (rate-limited) |
| `POST /login` | — | Log in, returns token (rate-limited) |
| `GET /me` | ✓ | Current user |
| `GET /rooms` | ✓ | List your rooms (auto-joins `general`) |
| `POST /rooms` | ✓ | Create a room |
| `POST /rooms/{id}/join` | ✓ | Join a room |
| `GET /rooms/{id}/messages` | ✓ | Paginated history (`?before=<id>&limit=N`) |
| `POST /dms` | ✓ | Create / fetch a DM with another user |
| `GET /search` | ✓ | Full-text search (`?q=...`) |
| `PUT /me/public_key` | ✓ | Upload your E2EE public key |
| `GET /users/{id}/public_key` | ✓ | Fetch a user's public key (TOFU) |
| `GET /ws` | token query param | WebSocket upgrade |
| `GET /health` | — | Liveness probe |
| `GET /metrics` | — | Prometheus metrics |

---

## Security notes

- Passwords are hashed with **argon2id** (OWASP interactive parameters).
- Session tokens are **PASETO v4 local** tokens.
- **DM bodies are end-to-end encrypted** with Curve25519 + NaCl `box` — the server stores ciphertext only and cannot read DMs (so DM search is intentionally not possible server-side).
- A user's private key is stored locally, **encrypted with a key derived from their password via a separate argon2id call** (never the auth hash). Logging in unlocks it; silent session-resume keeps DMs locked until you re-enter your password.
- Public keys use **trust-on-first-use** with local fingerprint pinning and a key-change warning.
- Group rooms are transport-encrypted only (TLS), not end-to-end — group E2EE is out of scope.

---

## Testing

```bash
make test     # go test -race ./...
make vet      # go vet ./...
```

---

## Deployment

Build the container locally:

```bash
make docker-build      # distroless image, ~23MB
```

Deploy to [Fly.io](https://fly.io) (requires the `flyctl` CLI and a Fly account):

```bash
fly launch --no-deploy                 # create the app (keep fly.toml)
fly postgres create                    # provision Postgres
fly postgres attach <pg-app>           # sets DATABASE_URL
fly secrets set DB_URL="$DATABASE_URL" PASETO_KEY=$(openssl rand -hex 32)
fly deploy
```

> **Note:** migrations are currently run manually via `goose`; run them against
> the Fly Postgres before first use.

---

## License

MIT
