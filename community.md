# Conduit Community Edition

## What This Is

This is the **Conduit Community Edition** — a fully functional, self-hosted remote infrastructure management platform. It is the foundation that the full multi-tenant SaaS will be built on top of. Everything here ships as production-quality code, not throwaway prototyping.

**Single monorepo. Two binaries. Full stack Go + Next.js static export.**

No multi-tenancy. No billing. No sub-tenants. No SaaS mode. Just a rock-solid single-operator platform for managing machines remotely.

---

## What We Are Building

1. **Real-time remote shell** — browser-based terminal (xterm.js + shadcn/ui) connected to agent PTY over QUIC/WebSocket
2. **File system access** — browse directories, download files, upload files on any connected agent
3. **Agent with persistent connection** — outbound-only QUIC (primary) + WebSocket (fallback), auto-reconnect with exponential backoff, installed as a systemd service
4. **Agent join security** — enrollment via single-use and persistent join tokens with label scoping
5. **Passkey authentication** — WebAuthn in production mode, dev-mode password fallback
6. **Let's Encrypt TLS** — ACME auto-provisioning in production, self-signed in dev mode
7. **TUI** — bubbletea interactive terminal for dropping a shell (no-args default)
8. **Community Edition website** — promotional landing page embedded in the same binary
9. **CWP wire protocol** — binary framing, transport-agnostic, multiplexed

---

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    conduit-server (Go)                        │
│                                                              │
│  ┌────────────┐  ┌────────────┐  ┌──────────────────────┐   │
│  │  QUIC      │  │  HTTP/3 +  │  │  Static Next.js      │   │
│  │  Listener  │  │  HTTP/2    │  │  (embed.FS)          │   │
│  │  (agents)  │  │  + WS      │  │  shadcn/ui dashboard │   │
│  └─────┬──────┘  └─────┬──────┘  │  + promo website     │   │
│        │               │         └──────────────────────┘   │
│  ┌─────┴───────────────┴──────────────────────┐              │
│  │              Session Router                 │              │
│  │  (maps browser WS ↔ agent QUIC/WS stream)  │              │
│  └────────────────────┬───────────────────────┘              │
│                       │                                      │
│  ┌────────────────────┴───────────────────────┐              │
│  │  Auth: WebAuthn (prod) / Password (dev)    │              │
│  │  JWT: Ed25519 signed, short-lived          │              │
│  │  Agent Auth: HMAC-SHA256 join tokens       │              │
│  └────────────────────────────────────────────┘              │
│                       │                                      │
│  ┌────────────────────┴───────────────────────┐              │
│  │  SQLite (single-tenant, tenant_id=default) │              │
│  └────────────────────────────────────────────┘              │
│                                                              │
│  Production: UDP 443 (QUIC) + TCP 443 (TLS/HTTP)            │
│  Dev mode:   UDP 8443 + TCP 8443 (no root required)         │
└──────────────────────────────────────────────────────────────┘
       ▲ QUIC/WSS                    ▲ HTTP/3 + WSS
       │ (agent connections)         │ (browser)
       │                             │
  ┌────┴─────┐                 ┌────┴──────────┐
  │  Agent   │                 │   Browser     │
  │ (conduit)│                 │  shadcn/ui    │
  │ systemd  │                 │  xterm.js     │
  │ service  │                 │  file manager │
  └──────────┘                 └───────────────┘

       ┌──────────┐
       │   TUI    │
       │(conduit) │
       │bubbletea │
       │ shell    │
       └──────────┘
```

---

## Monorepo Layout

```
conduit/
├── community.md
├── go.mod                          # Go module (both binaries)
├── go.sum
├── Makefile                        # Build both binaries + frontend
│
├── cmd/
│   ├── conduit-server/             # Server binary entrypoint
│   │   └── main.go
│   └── conduit/                    # Agent + CLI + TUI binary entrypoint
│       └── main.go
│
├── internal/
│   ├── protocol/                   # CWP — Conduit Wire Protocol
│   │   ├── frames.go              # Frame type definitions + constants
│   │   ├── codec.go               # Binary encode/decode
│   │   └── mux.go                 # Stream multiplexer (WebSocket path)
│   │
│   ├── server/
│   │   ├── server.go              # Main server lifecycle
│   │   ├── quic.go                # QUIC listener for agent connections
│   │   ├── http.go                # HTTP/3 + HTTP/2 server
│   │   ├── auth.go                # WebAuthn + dev-mode password + JWT
│   │   ├── webauthn.go            # Passkey registration + assertion
│   │   ├── router.go             # Session router (browser ↔ agent)
│   │   ├── agents.go              # Agent registry, join tokens, labels
│   │   ├── filehandler.go         # File operation relay
│   │   ├── eventbus.go            # WebSocket EventBus (real-time updates)
│   │   ├── setup.go               # First-run setup wizard
│   │   └── acme.go                # Let's Encrypt cert provisioning
│   │
│   ├── agent/
│   │   ├── agent.go               # Agent lifecycle + reconnect loop
│   │   ├── shell.go               # PTY management (Linux)
│   │   ├── files.go               # File operations (ls, read, write, stat)
│   │   ├── transport.go           # QUIC primary + WebSocket fallback
│   │   ├── install.go             # systemd service installation
│   │   └── sysinfo.go             # Host info (CPU, mem, disk, OS, arch)
│   │
│   ├── tui/
│   │   ├── tui.go                 # bubbletea app — main model
│   │   ├── serverlist.go          # Agent list view
│   │   └── shell.go               # Shell session view
│   │
│   ├── auth/
│   │   ├── jwt.go                 # JWT issuance + validation (Ed25519)
│   │   ├── passkey.go             # WebAuthn relying party logic
│   │   └── tokens.go              # Agent join tokens (single-use + persistent)
│   │
│   ├── db/
│   │   ├── sqlite.go              # SQLite connection + migrations
│   │   ├── migrations/            # Embedded SQL migrations
│   │   │   └── 001_initial.sql
│   │   ├── users.go               # User CRUD
│   │   ├── agents.go              # Agent registry CRUD
│   │   └── tokens.go              # Join token CRUD
│   │
│   └── shared/
│       ├── tls.go                 # TLS config (self-signed dev / ACME prod)
│       └── config.go              # server.yaml / CLI flag parsing
│
├── web/                           # Next.js 15 App Router
│   ├── package.json
│   ├── pnpm-lock.yaml
│   ├── next.config.js             # output: 'export', unoptimized images
│   ├── components.json            # shadcn/ui config (new-york, rsc:true, tw v4)
│   ├── tsconfig.json
│   ├── app/
│   │   ├── layout.tsx             # Root layout
│   │   ├── page.tsx               # Community Edition promo landing page
│   │   ├── login/
│   │   │   └── page.tsx           # Login (passkey + dev-mode password)
│   │   ├── setup/
│   │   │   └── page.tsx           # First-run setup wizard
│   │   └── dashboard/
│   │       ├── layout.tsx         # Dashboard shell (sidebar, nav)
│   │       ├── page.tsx           # Agent list (default view)
│   │       ├── terminal/
│   │       │   └── [agentId]/
│   │       │       └── page.tsx   # Terminal session
│   │       └── files/
│   │           └── [agentId]/
│   │               └── page.tsx   # File browser
│   ├── components/
│   │   ├── ui/                    # shadcn/ui components (code-owned)
│   │   ├── terminal.tsx           # xterm.js wrapper component
│   │   ├── file-browser.tsx       # File browser component
│   │   ├── agent-list.tsx         # Connected agents table
│   │   └── login-form.tsx         # Passkey + password login
│   ├── lib/
│   │   ├── utils.ts               # cn() utility
│   │   └── api.ts                 # API client (fetch wrapper)
│   ├── hooks/
│   │   ├── use-websocket.ts       # WebSocket connection hook
│   │   └── use-auth.ts            # Auth state hook
│   └── public/
│       ├── fonts/                 # Local font files (no CDN)
│       └── og/                    # Pre-generated OG images
│
└── embed.go                       # //go:embed web/out/* for static frontend
```

---

## Agent Join Security

### Join Tokens

When an operator wants to add machines, they generate a **join token** from the dashboard or CLI. The token encodes what the agent is allowed to be and where it belongs.

**Two token types:**

| Type | Use Case | Lifetime |
|---|---|---|
| **Single-use** | One specific machine. Token is deleted after first successful join. | Expires after configurable TTL (default 1 hour) |
| **Persistent** | Fleet enrollment. Same token used by many machines (e.g., cloud-init, Ansible). | Expires after configurable TTL or manual revocation |

**Token structure:**

```
conduit join <server-url> <token>
```

The token is a signed JWT containing:

```json
{
  "jti": "unique-token-id",
  "type": "single_use | persistent",
  "labels": {"env": "production", "role": "web", "dc": "us-east-1"},
  "exp": 1711584000,
  "iss": "conduit-server"
}
```

### Join Flow

1. Operator generates join token: `conduit token create --labels env=production,role=web --type persistent --ttl 24h`
2. Operator runs on target machine: `conduit join https://conduit.example.com <token>`
3. The `conduit join` command:
   - Validates the token with the server (`POST /api/v1/agents/join`)
   - Server validates signature, checks expiry, checks single-use not already consumed
   - Server generates a unique **agent identity** (UUID + HMAC-SHA256 agent key)
   - Server applies the labels from the token to the new agent record
   - Server returns the agent ID + agent key + server fingerprint
   - Agent stores credentials in `/etc/conduit/agent.yaml`
   - Agent installs itself as a systemd service (`conduit-agent.service`)
   - Agent starts and connects to server using its agent key
4. If single-use token: server deletes the token immediately after successful join
5. If persistent token: token remains valid for more machines until TTL or revocation

### Agent Identity (Post-Join)

After joining, the agent authenticates on every connection using its unique HMAC-SHA256 agent key. The join token is never used again — it was only for enrollment.

```
Agent connects → QUIC/WS handshake → CWP HELLO frame (hostname, OS, arch, agent-id)
→ CWP AUTH frame (HMAC-SHA256 signature of HELLO payload using agent key)
→ Server validates → AUTH OK/REJECT
```

### Label Scoping

Labels assigned during enrollment are the primary mechanism for organizing and targeting agents:

- **Set at join time** via the token: `--labels env=production,role=web,dc=us-east-1`
- **Mutable after join** — operator can add/remove labels from the dashboard or CLI
- **Used for targeting** — shell access, bulk exec, file operations can target by label selector
  - `conduit exec --label env=production -- uptime`
  - `conduit shell --label role=web` (if multiple matches, shows picker)
- **Displayed in dashboard** — agents grouped/filterable by labels

### systemd Service Installation

`conduit join` installs the agent as a systemd service:

```ini
[Unit]
Description=Conduit Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/conduit agent
Restart=always
RestartSec=5
Environment=CONDUIT_CONFIG=/etc/conduit/agent.yaml
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

- Binary copied to `/usr/local/bin/conduit`
- Config written to `/etc/conduit/agent.yaml`
- Service enabled and started immediately
- `Restart=always` ensures the agent survives crashes and reboots

---

## Connection Persistence Strategy

The agent must stay connected to the server at all times. Disconnection = blind spot.

### QUIC Primary Path
- Agent dials server on UDP 443 (prod) / UDP 8443 (dev)
- QUIC 0-RTT reconnection for fast recovery after brief disconnects
- QUIC handles packet loss and network migration natively
- If QUIC connection fails for ~5 seconds, agent falls back to WebSocket

### WebSocket Fallback Path
- Agent dials `wss://<server>/agent/v1/connect` on TCP 443 (prod) / TCP 8443 (dev)
- Used when UDP is blocked (corporate firewalls, restrictive NAT)
- Same CWP frames, multiplexed via StreamID over single WS connection

### Reconnect Strategy
- **Exponential backoff with jitter:** 1s → 2s → 4s → 8s → 16s → 30s (cap)
- **Jitter:** +/- 30% randomization to prevent thundering herd
- **Transport alternation:** If QUIC fails 3 consecutive times, try WebSocket. If WebSocket fails 3 times, try QUIC again.
- **On success:** Reset backoff timer, log reconnection event
- **Heartbeat:** PING/PONG every 15 seconds. If 3 consecutive PINGs unanswered, consider connection dead and reconnect.
- **Network change detection:** Agent monitors network interfaces; on change, immediately attempt reconnect (don't wait for heartbeat timeout)

### Connection State (Server-Side)
- Server tracks each agent: `connected`, `disconnected`, `stale`
- `disconnected` after heartbeat timeout (45s with no PONG)
- `stale` after extended disconnect (configurable, default 24h)
- Dashboard shows real-time connection status via EventBus
- Reconnecting agent resumes its identity — same agent ID, same labels

---

## Auth Model

### Production Mode (Passkeys)

**WebAuthn is the sole human authentication mechanism.**

- `POST /api/v1/auth/passkey/register/begin` — start registration
- `POST /api/v1/auth/passkey/register/finish` — complete registration
- `POST /api/v1/auth/passkey/login/begin` — start assertion
- `POST /api/v1/auth/passkey/login/finish` — complete assertion, returns JWT
- JWT: Ed25519 signed, short-lived (15 min access + 7 day refresh)
- All dashboard/API requests require valid JWT in `Authorization: Bearer` header
- Browser WebSocket upgrade includes JWT for auth

### Dev Mode (Password Fallback)

- Enabled by `--dev` flag or `server.mode: dev` in `server.yaml`
- `POST /api/v1/auth/login` with `{email, password}` — returns JWT
- Password hashed with Argon2id
- Dev mode clearly indicated in the UI (banner)
- **Dev mode also uses port 8443 and self-signed certs**

### Setup Wizard (First Run)

1. Server starts on `localhost:8080` (HTTP only, localhost-only)
2. Wizard collects: domain name, admin email, org name, temporary password
3. Server obtains ACME cert, writes `server.yaml`, creates admin user
4. Restarts on port 443 with TLS
5. Admin logs in with temp password → forced passkey registration
6. Temp password deleted from DB after passkey registered
7. `/setup` returns 404 forever after

### CLI Authentication

- TUI/CLI opens browser to server's login page
- User authenticates with passkey in browser
- Browser-based device flow returns scoped CLI token
- Token stored encrypted in `~/.config/conduit/credentials.yaml`
- CLI uses token for all API calls and WebSocket connections

---

## Wire Protocol (CWP)

Binary frame format, identical over QUIC streams and WebSocket messages:

```
┌──────────┬──────────┬───────────┬──────────────────┐
│ Type (1B)│ StreamID │ Length    │ Payload          │
│          │  (4B)    │  (4B)    │ (variable)       │
└──────────┴──────────┴───────────┴──────────────────┘
```

**Frame types:**

| Type | ID | Direction | Purpose |
|---|---|---|---|
| HELLO | 0x01 | Agent → Server | Agent announces itself (hostname, OS, arch, agent-id) |
| AUTH | 0x02 | Both | Agent sends HMAC; server sends OK/REJECT |
| SHELL_DATA | 0x10 | Both | PTY stdin/stdout bytes |
| SHELL_RESIZE | 0x11 | Server → Agent | Terminal resize (cols, rows) |
| SHELL_START | 0x12 | Server → Agent | Request new shell session |
| SHELL_EXIT | 0x13 | Agent → Server | Shell process exited (exit code) |
| FILE_LIST | 0x20 | Both | Request/response directory listing |
| FILE_READ | 0x21 | Both | Request/response file content (chunked) |
| FILE_WRITE | 0x22 | Server → Agent | Write file content to agent (chunked) |
| FILE_STAT | 0x23 | Both | Request/response file metadata |
| AGENT_INFO | 0x30 | Agent → Server | System metrics (CPU, mem, disk) |
| PING | 0xF0 | Both | Keepalive |
| PONG | 0xF1 | Both | Keepalive response |

QUIC path: each operation gets its own QUIC stream. StreamID used for correlation.
WebSocket path: multiplexer interleaves frames by StreamID over single connection.

---

## TUI (bubbletea) — Shell Only

When `conduit` is invoked with no arguments (and not in agent mode), it launches the TUI.

**Community Edition TUI scope:**
- Server list view — shows connected agents (hostname, OS, IP, transport, status)
- Press Enter on an agent → drops into a live shell session
- Vim-style navigation (j/k, Enter, Esc, / to search)
- `/shell <name>` — quick jump to a shell
- `/quit` — exit
- Status bar showing authenticated user + connection state

**Direct CLI (with arguments):**
- `conduit shell <agent>` — open shell immediately, no TUI
- `conduit join <server> <token>` — join + install as service
- `conduit token create --labels ... --type ...` — generate join token
- `conduit agent` — run as agent daemon (used by systemd)

---

## Frontend — shadcn/ui + Next.js Static Export

**Next.js 15 App Router → static export → embedded in Go binary via `embed.FS`**

### Setup
- Style: `new-york`
- `rsc: true` (ensures `"use client"` directives present)
- Tailwind v4 (CSS-first config, no `tailwind.config.js`)
- `pnpm dlx skills add shadcn/ui` for AI accuracy
- All fonts local in `public/fonts/` — zero CDN dependencies
- `output: 'export'` + `images: { unoptimized: true }`
- Component variants via CVA — no hardcoded className strings

### Pages

**Public (no auth):**
- `/` — Community Edition promotional landing page
  - Hero: "Secure remote access to every machine. No SSH keys. No VPNs. No inbound ports."
  - Feature highlights (shell, files, QUIC, passkeys, zero-trust)
  - Quick start guide
  - Self-host call to action
- `/login` — Passkey login (+ dev-mode password form)
- `/setup` — First-run wizard (only shown once)

**Dashboard (auth required):**
- `/dashboard` — Agent list: hostname, OS, IP, transport type, CPU/mem, connection status. Real-time via EventBus WebSocket.
- `/dashboard/terminal/[agentId]` — Full-screen xterm.js terminal session
- `/dashboard/files/[agentId]` — File browser: directory tree, file list, download/upload, breadcrumb nav

### Real-Time Updates
- WebSocket EventBus at `/api/v1/events/stream`
- Agent connect/disconnect events push to all browsers immediately
- No polling. No manual refresh.

---

## TLS & Certificates

### Production Mode
- ACME auto-provisioning via Let's Encrypt
- TLS 1.3 only (1.0/1.1/1.2 disabled)
- Certs stored in `server.yaml`-configured path (default `/var/lib/conduit/certs/`)
- Auto-renewal before expiry
- Ports: UDP 443 + TCP 443

### Dev Mode
- Self-signed TLS 1.3 cert generated on startup
- Cert fingerprint printed to console (for agent pinning)
- Ports: UDP 8443 + TCP 8443
- Browser will show cert warning (expected)

---

## SQLite Schema (Community Edition)

```sql
-- Users (single-tenant, no tenant_id scoping yet)
CREATE TABLE users (
    id TEXT PRIMARY KEY,              -- UUID v4
    email TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    password_hash TEXT,               -- Argon2id, NULL after passkey setup
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- WebAuthn credentials
CREATE TABLE passkeys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    credential_id BLOB NOT NULL,
    public_key BLOB NOT NULL,
    sign_count INTEGER NOT NULL DEFAULT 0,
    name TEXT,                        -- User-given name ("MacBook Touch ID")
    created_at TEXT NOT NULL
);

-- Registered agents
CREATE TABLE agents (
    id TEXT PRIMARY KEY,              -- UUID v4
    hostname TEXT NOT NULL,
    os TEXT,
    arch TEXT,
    labels TEXT,                      -- JSON: {"env":"production","role":"web"}
    agent_key_hash TEXT NOT NULL,     -- HMAC key hash for auth
    status TEXT NOT NULL DEFAULT 'disconnected',
    last_seen TEXT,
    created_at TEXT NOT NULL
);

-- Join tokens
CREATE TABLE join_tokens (
    id TEXT PRIMARY KEY,              -- UUID v4 (also the token JTI)
    type TEXT NOT NULL,               -- 'single_use' or 'persistent'
    labels TEXT,                      -- JSON: labels to apply on join
    token_hash TEXT NOT NULL,         -- Hash of the signed JWT
    used_count INTEGER DEFAULT 0,
    expires_at TEXT NOT NULL,
    revoked INTEGER DEFAULT 0,
    created_at TEXT NOT NULL
);

-- Active sessions (for session visibility/revocation)
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    type TEXT NOT NULL,               -- 'web', 'cli'
    refresh_token_hash TEXT,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);
```

Migrations embedded in binary, applied automatically at startup.

---

## What Is Explicitly Out of Scope

- Multi-tenancy (`tenant_id` scoping, sub-tenants)
- Billing / Stripe integration
- SaaS mode (`server.mode: saas`)
- RBAC / role hierarchy (single admin user for CE)
- PQC hybrid key exchange (standard TLS 1.3 for now)
- Agent auto-update / binary signing
- Terminal session recording
- Bulk exec across agents
- SAML/OIDC SSO
- Windows / macOS agents
- Audit log UI (events logged to stdout/file, no dashboard viewer)
- Subdomain routing
- Cluster mode

---

## Build & Run

```bash
# Build frontend
cd web && pnpm install && pnpm build && cd ..

# Build server (embeds frontend)
go build -o conduit-server ./cmd/conduit-server

# Build agent/CLI
CGO_ENABLED=0 go build -o conduit ./cmd/conduit

# --- Dev mode (local testing) ---
./conduit-server --dev
# → Starts on https://localhost:8443 with self-signed cert

./conduit join https://localhost:8443 <token> --dev-insecure
# → Joins, installs service, connects

# --- Production ---
./conduit-server
# → First run: setup wizard on localhost:8080
# → After setup: ACME cert, port 443

./conduit join https://conduit.example.com <token>
```

---

## Success Criteria

The Community Edition is complete when:

1. `conduit-server` starts, runs setup wizard, obtains Let's Encrypt cert
2. Operator registers a passkey and is in the dashboard
3. Operator generates a join token with labels from the dashboard
4. On a remote Linux machine: `conduit join <url> <token>` installs the agent as a systemd service
5. Agent appears in the dashboard in real-time (no refresh)
6. Operator clicks agent → live terminal works like SSH (full PTY, resize, interactive programs)
7. Operator switches to file browser → navigates directories, downloads a file, uploads a file
8. Operator uses TUI: `conduit` (no args) → sees agent list → enters shell
9. Kill the agent process → dashboard shows disconnected immediately → agent auto-restarts (systemd) and reconnects
10. Network blip → agent reconnects automatically with no operator intervention
11. Community Edition landing page loads at `/` with self-host pitch
12. Unauthenticated access to dashboard/API returns 401
13. Invalid join token is rejected

---

## Open Questions

None remaining — ready to build. If anything surfaces during implementation, we'll address it inline.
