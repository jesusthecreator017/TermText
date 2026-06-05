# TermText — End-to-End Roadmap

## Context

You want to build a terminal chat app modeled on [Abi-Liu/TextTunnel](https://github.com/Abi-Liu/TextTunnel) — same shape (Go server, BubbleTea TUI client, Postgres, WebSockets) — but with modernized libraries, production-grade polish, and four extra capability areas: rooms/DMs, history+search, realtime UX signals (presence/typing/read-receipts), and end-to-end encryption for DMs. Today the repo (`/Users/jesusgonzalez07/Desktop/MacToUSB/Go/TermText`) is a Hello-World scaffold with module `github.com/jesusthecreator017/TermText` and Go 1.25. The goal of this plan is to lay out an ordered phase-by-phase build so each phase ships a runnable artifact and teaches one core concept cleanly.

## Stack — chosen (modernized vs. reference repo)

| Layer | TextTunnel | TermText (you) | Why the change |
|---|---|---|---|
| Language | Go 1.22 | Go 1.25 | already set |
| TUI | bubbletea + bubbles + lipgloss | same | best-in-class for Go TUIs |
| WebSocket | `nhooyr.io/websocket` | `github.com/coder/websocket` | nhooyr's lib was donated to coder; same API, actively maintained |
| DB driver | `lib/pq` | `github.com/jackc/pgx/v5` | pq is in maintenance mode; pgx is faster, supports `LISTEN/NOTIFY`, batching |
| Queries | sqlc | sqlc (keep) | type-safe SQL, no ORM tax |
| Migrations | hand-rolled scripts | `pressly/goose` | versioned, embedded, scriptable |
| HTTP router | net/http | `net/http` + `http.ServeMux` (Go 1.22+ pattern matching) | stdlib is enough now |
| Auth hashing | bcrypt | argon2id (`golang.org/x/crypto/argon2`) | OWASP-recommended; better against GPU attacks |
| Sessions | (custom) | PASETO v4 local tokens or DB-backed opaque sessions | PASETO avoids JWT footguns |
| Logging | (custom) | `log/slog` (stdlib) | structured logs, free |
| Config | godotenv | `caarlos0/env` + `.env` | typed config struct |
| Tests | — | stdlib + `testcontainers-go` for Postgres | real DB in CI, no mocks |
| Container | Dockerfile | Dockerfile + `docker compose` for dev | one-command local stack |
| Deploy | — | Fly.io (server) + Fly Postgres | persistent WS friendly, generous free tier, TLS automatic |
| CI | (none visible) | GitHub Actions | lint, test, build on PR |
| E2EE | — | `golang.org/x/crypto/nacl/box` | curve25519 + xsalsa20-poly1305; right primitive for DMs |
| Search | — | Postgres `tsvector` + GIN index | no extra service needed |

## Repository layout (target)

```
TermText/
  cmd/
    server/main.go          # HTTP+WS server entrypoint
    termtext/main.go        # TUI client entrypoint
  internal/
    auth/                   # argon2id, token issue/verify
    config/                 # env-driven typed config
    db/                     # sqlc-generated code + queries.sql
    httpapi/                # REST handlers, middleware
    hub/                    # WS hub: rooms, clients, broadcast
    wsproto/                # message envelope types shared by server+client
    crypto/                 # E2EE helpers (NaCl box wrappers)
    tui/
      app.go                # root BubbleTea model
      views/login.go
      views/rooms.go
      views/chat.go
      keys/                 # local key/profile storage (~/.config/termtext)
  migrations/               # goose .sql files
  scripts/
  docker-compose.yml        # postgres for dev
  Dockerfile                # server image
  .github/workflows/ci.yml
  Makefile
```

---

## Phases

Each phase ends with a runnable demo and a short list of what you'll have learned.

### Phase 0 — Foundations (1–2 evenings)
**Build:** project layout above; `docker compose up` starts Postgres; `make run-server` launches an HTTP server on `:8080` returning `200 OK` from `/healthz`; `make run-client` launches an empty BubbleTea screen that quits on `q`.
**Files touched:** `cmd/server/main.go`, `cmd/termtext/main.go`, `internal/config/config.go`, `docker-compose.yml`, `Makefile`.
**You'll learn:** Go project layout conventions, stdlib `http.ServeMux` (1.22+ method+pattern routing), `log/slog` setup, BubbleTea's `Model/Update/View` loop in its simplest form.

### Phase 1 — Auth + REST surface (1 week)
**Build:** `POST /signup`, `POST /login`, `GET /me`. Passwords hashed with argon2id. Session represented as a PASETO v4 token (or DB-backed opaque token — pick one). Middleware extracts user from `Authorization: Bearer`. Migrations create `users` table via goose. sqlc generates query code from `internal/db/queries.sql`.
**Files touched:** `internal/auth/*`, `internal/httpapi/*`, `migrations/0001_users.sql`, `internal/db/queries.sql`.
**You'll learn:** password hashing tradeoffs (argon2 params: memory/time/parallelism), token vs session tradeoffs, sqlc workflow, goose migrations, table-driven tests against a real Postgres via testcontainers.

### Phase 2 — WebSocket hub (single global room) (1 week)
**Build:** `GET /ws` upgrades to WebSocket (auth via query token or first message). One `Hub` goroutine owns `clients map[userID]*Client`; each client has reader+writer goroutines and a bounded send channel. JSON envelope: `{type, payload}` where type ∈ `message|presence|error`. Broadcast `message` to all connected clients. Ping/pong keepalive every 30s; drop dead clients.
**Critical patterns to follow:** the classic Gorilla "chat" example structure — one goroutine per direction per connection, channels for fan-out. Use `coder/websocket`'s `Reader/Writer` + `context.Context` for cancellation.
**Files touched:** `internal/hub/{hub,client}.go`, `internal/wsproto/envelope.go`, `cmd/server/main.go`.
**You'll learn:** goroutine ownership rules (who closes what), backpressure via bounded channels, framing/keepalive, why you never write to a websocket from two goroutines.

### Phase 3 — TUI MVP (1 week)
**Build:** Login screen → token saved to `~/.config/termtext/session.json` → chat screen with viewport (scrollback) + textarea (input). Client opens WS, decodes envelopes, appends to viewport. `Ctrl+C` disconnects cleanly. Use `bubbles/viewport` and `bubbles/textarea`.
**Files touched:** `internal/tui/*`, `cmd/termtext/main.go`.
**You'll learn:** BubbleTea `tea.Cmd` for async I/O, bridging a network goroutine into the model via `tea.Msg`s on a channel, `lipgloss` layout (borders, joins, width-aware rendering), keymap design.

### Phase 4 — Rooms & DMs (1–2 weeks)
**Build:** schema for `rooms`, `room_members`, `messages` (with `room_id`). REST: `POST /rooms`, `GET /rooms`, `POST /rooms/:id/join`. WS envelope grows a `room_id` field. Hub becomes `map[roomID]map[clientID]*Client`. DMs = auto-created 2-member room with `kind='dm'`. TUI gets a left rooms-list pane and Ctrl+K room switcher (use `bubbles/list` + `sahilm/fuzzy`).
**Files touched:** new migrations, `internal/hub/*`, `internal/httpapi/rooms.go`, `internal/tui/views/rooms.go`.
**You'll learn:** modeling many-to-many membership, fan-out routing inside the hub, designing message envelopes that don't break when fields are added, multi-pane TUI layouts.

### Phase 5 — History + search (3–5 days)
**Build:** `GET /rooms/:id/messages?before=<msg_id>&limit=50` for paginated scrollback. TUI fetches older messages on viewport scroll-top. Add `tsvector` column on `messages.body` with a GIN index; `GET /search?q=...` returns ranked matches scoped to rooms the user is a member of. `/search` slash-command in chat input opens a results overlay.
**Files touched:** migration adding `search_tsv tsvector GENERATED ALWAYS AS (to_tsvector('english', body)) STORED`, search query, TUI overlay.
**You'll learn:** keyset pagination (vs offset), Postgres full-text search (`to_tsvector`, `plainto_tsquery`, `ts_rank`), generated columns + GIN indexes, infinite-scroll UX in a TUI.

### Phase 6 — Presence, typing, read receipts (1 week)
**Build:**
- Presence: hub tracks live members per room; on connect/disconnect, emit `presence` events to that room's other members. TUI shows online dot next to names.
- Typing: client sends `typing` envelope debounced to 1/sec; server fans out; TUI shows "Alice is typing…" with 3s TTL.
- Read receipts: `room_members.last_read_message_id`; client sends `read` envelope when viewport reaches bottom; server updates and fans out. TUI shows "✓ seen by N".
**Files touched:** `internal/hub/*`, new envelope types, migration adding `last_read_message_id`.
**You'll learn:** distinguishing ephemeral (broadcast-only) vs persistent (DB-write-then-broadcast) events, debouncing on the client side, idempotent state updates.

### Phase 7 — End-to-end encryption for DMs (1–2 weeks)
**Build:**
- On signup, client generates a Curve25519 keypair via `nacl/box.GenerateKey`. Public key is sent to server (`users.public_key`). Private key is stored in `~/.config/termtext/keys.enc`, encrypted with a key derived from the user's password via argon2id (separate KDF call from auth hash).
- Sending a DM: client fetches recipient public key, encrypts body with `box.Seal`, sends `{ciphertext, nonce}` as the message body. Server stores ciphertext only.
- Receiving: client decrypts with own private key + sender's public key.
- TUI shows a 🔒 indicator on E2EE DMs; group rooms remain plaintext (or "transport-encrypted only") — keep group E2EE out of scope to stay realistic.
- Add a `GET /users/:id/public_key` endpoint with TOFU (trust-on-first-use): cache fingerprints locally, warn on change.
**Files touched:** `internal/crypto/*`, migration adding `users.public_key`, message body schema change for DMs.
**You'll learn:** public-key vs symmetric crypto, why nonce reuse is catastrophic, KDFs vs hashes (and that you must NOT reuse the auth hash for key derivation), TOFU and the key-change attack, why group E2EE (Signal's Double Ratchet / MLS) is its own multi-month project.

### Phase 8 — Production hardening & deploy (1–2 weeks)
**Build:**
- Tests: unit tests on auth/crypto/hub; integration tests for the WS hub using two in-process clients; httptest + testcontainers for the REST API.
- CI: GitHub Actions workflow — `go vet`, `staticcheck`, `go test ./...`, build.
- Observability: structured `slog` everywhere with request IDs; `/metrics` Prometheus endpoint (`prometheus/client_golang`) exposing connection count, messages/sec, hub send-channel saturation.
- Rate limiting: token-bucket per user on `POST /signup`, `POST /login`, and WS messages (e.g., `golang.org/x/time/rate`).
- Deploy: Dockerfile (multi-stage, distroless final image), `fly launch`, Fly Postgres attached, secrets via `fly secrets set`. TLS handled by Fly. `go install github.com/you/TermText/cmd/termtext@latest` works for friends.
**Files touched:** `.github/workflows/ci.yml`, `Dockerfile`, `fly.toml`, `internal/httpapi/middleware/*`.
**You'll learn:** real Go testing patterns (no mocks for DB), CI hygiene, the difference between RED metrics and pure logs, how Fly handles persistent connections, image-size discipline.

---

## What you'll have learned by the end

- **Go in anger:** goroutine ownership, channel patterns, context cancellation, stdlib HTTP, slog.
- **Realtime systems:** WebSocket lifecycle, hub/broadcast patterns, ephemeral-vs-persistent events, presence math.
- **Postgres beyond CRUD:** sqlc workflow, migrations, full-text search, generated columns, GIN indexes, keyset pagination.
- **Security:** argon2id parameters, why bcrypt is dated, PASETO vs JWT, NaCl box, KDFs, TOFU and key-change UX, the cliff between pairwise and group E2EE.
- **TUI engineering:** Elm-architecture-style state, async I/O via `tea.Cmd`, lipgloss layout, fuzzy search UIs.
- **Ops:** Dockerized dev loop, Fly deploy, GitHub Actions, Prometheus metrics for a long-lived-connection server.

## Stretch / "phase 9+" ideas (don't plan now)

- File / image attachments (uploads to S3-compatible storage; references in messages)
- Slash commands (`/me`, `/shrug`, `/giphy`)
- Notifications via local OS bell or a desktop-notify package
- Mobile-friendly web client sharing the same WS protocol (htmx + Server-Sent Events fallback)
- Federation between two TermText servers (ActivityPub-lite)

## Critical files to create first (Phase 0)

1. `cmd/server/main.go` — server entrypoint, wires config + slog + http.ServeMux + /healthz.
2. `cmd/termtext/main.go` — client entrypoint, boots BubbleTea program.
3. `internal/config/config.go` — typed struct loaded from env with sane dev defaults.
4. `docker-compose.yml` — Postgres 16 service, volume, port 5432, dev creds.
5. `Makefile` — `run-server`, `run-client`, `migrate-up`, `sqlc-gen`, `test`.
6. `migrations/0001_init.sql` — empty placeholder so goose is wired from day 1.

## Verification

End-to-end smoke at the end of every phase — never let it rot:

1. `docker compose up -d` → Postgres healthy.
2. `make migrate-up` → schema current.
3. `go run ./cmd/server` → listens, `/healthz` returns 200.
4. Two terminals: `go run ./cmd/termtext` in each → sign up two users → both can talk in the room they share.
5. From Phase 5 onward: search returns hits; from Phase 6: typing/read indicators visible; from Phase 7: 🔒 shows on DMs and server-side message rows are ciphertext-only.
6. `go test ./...` green; CI green on PR; `fly deploy` succeeds and a friend can `go install` the client and connect.

Realistic total: **8–14 weekends** of focused work depending on how deep you go on tests and ops in each phase.
