# Conduit CE (Community Edition)

## What This Is

This is the **Conduit Community Edition** — a fully functional, self-hosted remote infrastructure management platform. It is the foundation that the full multi-tenant SaaS will be built on top of. Everything here ships as production-quality code, not throwaway prototyping.

**Single monorepo. Two binaries. Full stack Go + Next.js static export.**

No multi-tenancy hierarchy. No billing. No sub-tenants. No SaaS mode. Single tenant with multiple users and full RBAC. A rock-solid self-hosted platform for managing machines remotely.

---

## What We Are Building

### Server Core
- QUIC + WebSocket dual listeners (UDP 443 + TCP 443)
- HTTP/3 serving for browsers + HTTP/2 fallback
- ACME TLS auto-provisioning (Let's Encrypt)
- X25519MLKEM768 hybrid PQC TLS on all connections
- SQLite database (pure Go, `modernc.org/sqlite`) with app-layer AES-256-GCM on sensitive fields
- Embedded static frontend via `embed.FS`
- First-run setup wizard (localhost:8080 → ACME → HTTPS → forced passkey)
- WebSocket EventBus for real-time dashboard updates
- OpenAPI 3.1 spec generation
- `server.yaml` configuration file
- Input validation middleware — unknown JSON fields rejected on all endpoints
- Parameterized queries only — no raw SQL interpolation

### Auth & Identity
- WebAuthn passkey registration + login (sole production auth)
- Dev-mode password fallback (Argon2id hashed)
- JWT issuance and validation (Ed25519 signed, short-lived)
- Users + groups + RBAC (platform_owner, org_owner, org_admin, org_member)
- SAML/OIDC SSO (passkeys remain primary)
- CLI browser device flow (passkey → CLI token)
- CLI credential storage (encrypted, per-profile)
- All authenticated sessions (web, CLI, CI) visible and revocable from dashboard
- CI token support (`CONDUIT_TOKEN` env var, scoped, revocable tokens)

### Agent & Connections
- Agent outbound QUIC connection (primary)
- Agent automatic WebSocket fallback (if QUIC/UDP blocked)
- CWP wire protocol — identical binary framing over QUIC and WebSocket
- Agent registration flow (single-use + persistent join tokens with label scoping)
- Agent heartbeat and connection status in dashboard
- Dashboard shows transport type per agent (QUIC vs WebSocket)
- Linux amd64 + arm64 agent builds
- Windows amd64 agent build
- macOS arm64 agent build + launchd service
- Agent auto-update (signed binary push from master)
- Full host visibility per agent: CPU, memory, disk, network, services, ports
- Agent metrics dashboard (CPU, RAM, disk — `AGENT_INFO` frames)
- Universal resource labelling system for surgical targeting
- Agent installed as system service (systemd / launchd / Windows service) via `conduit join`

### Shell & Terminal
- Shell sessions in browser (xterm.js, full feature parity)
- PTY shell execution on agent (Linux, macOS, Windows via ConPTY)
- Multiple concurrent shell sessions per agent (multiplexed via CWP)
- Terminal session recording + playback (asciicast v2)

### File Management
- Browser-based file manager (list, download, upload, delete, rename, mkdir, preview)
- File transfer over HTTPS/QUIC — no SFTP protocol dependency
- Resumable file uploads

### CLI & TUI
- CLI binary (`conduit`) — auth, server list, shell, file transfer
- CLI runs as full TUI (bubbletea) when no arguments given
- CLI-only mode on personal devices (zero daemons, zero listeners)
- CLI exec commands — bulk exec from terminal
- CLI group, user, audit management commands
- CLI lights-out basic (reboot/poweroff via agent)
- Shell completions (bash, zsh, fish, PowerShell)

### Bulk Operations
- Bulk command execution — multi-server parallel script runner
- Binary deployment service

### Audit & Compliance
- Audit logging for all access events (who accessed what, when, from where)
- Audit log viewer in dashboard (search, filter, query)
- Webhooks on audit events (HMAC-SHA256 signed payloads, HTTPS-only in production)
- Webhook subscription management (create, update, delete, test)
- Webhook delivery history with retry tracking
- Dev mode: webhook delivery to `http://localhost` loopback and self-signed HTTPS
- NIST SP 800-53 Rev. 5 control coverage
- NIST SP 800-131A cryptographic compliance
- Security audit readiness (SOC 2 Type II, penetration test ready)

### Real-Time Dashboard
- WebSocket EventBus for live updates — no polling, no manual refresh
- Agent connect/disconnect events push to all browsers immediately
- Shell session start/stop events
- File operation events
- Auth events (login, logout, session revocation)
- Agent metrics streaming
- Channel-based subscriptions (agents, shell, files, auth, audit, metrics, exec, system)

### Promotional Website
- Public marketing landing page served from same binary
- Fortune 500-quality UI/UX
- SEO optimized for remote access, infrastructure access keywords
- Pre-generated OG images (build-time, static assets in `public/og/`)
- Core Web Vitals targets met (LCP <=2.5s, INP <=200ms, CLS <=0.1)

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
│  │  SQLite (single-tenant, UUID at setup)      │              │
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
  │  OS svc  │                 │  xterm.js     │
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
├── ce.md
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
│   │   ├── audit.go               # Audit logging middleware
│   │   ├── webhooks.go            # Webhook dispatch + retry
│   │   ├── validation.go          # Input validation middleware (reject unknown fields)
│   │   ├── setup.go               # First-run setup wizard
│   │   └── acme.go                # Let's Encrypt cert provisioning
│   │
│   ├── agent/
│   │   ├── agent.go               # Agent lifecycle + reconnect loop
│   │   ├── shell_linux.go         # PTY management (Linux)
│   │   ├── shell_windows.go       # ConPTY management (Windows)
│   │   ├── shell_darwin.go        # PTY management (macOS)
│   │   ├── files.go               # File operations (ls, read, write, stat)
│   │   ├── transport.go           # QUIC primary + WebSocket fallback
│   │   ├── install_linux.go       # systemd service installation
│   │   ├── install_darwin.go      # launchd service installation
│   │   ├── install_windows.go     # Windows service installation
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
│   │   ├── sso.go                 # SAML/OIDC SSO provider integration
│   │   └── tokens.go              # Agent join tokens (single-use + persistent)
│   │
│   ├── db/
│   │   ├── sqlite.go              # SQLite connection + migrations
│   │   ├── migrations/            # Embedded SQL migrations
│   │   │   └── 001_initial.sql
│   │   ├── users.go               # User CRUD
│   │   ├── groups.go              # Group + membership CRUD
│   │   ├── agents.go              # Agent registry CRUD
│   │   ├── tokens.go              # Join token CRUD
│   │   ├── audit.go               # Audit log writes + queries
│   │   └── webhooks.go            # Webhook subscription + delivery CRUD
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
│   │       ├── files/
│   │       │   └── [agentId]/
│   │       │       └── page.tsx   # File browser
│   │       ├── audit/
│   │       │   └── page.tsx       # Audit log viewer
│   │       ├── users/
│   │       │   └── page.tsx       # User + group management
│   │       ├── webhooks/
│   │       │   └── page.tsx       # Webhook management
│   │       └── settings/
│   │           └── page.tsx       # Server settings
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
   - Validates the token with the server (`POST /api/v1/agents/register`)
   - Server validates signature, checks expiry, checks single-use not already consumed
   - Server generates a unique **agent identity** (UUID + HMAC-SHA256 agent key)
   - Server applies the labels from the token to the new agent record
   - Server returns the agent ID + agent key + server fingerprint + master endpoints (QUIC + WebSocket URLs)
   - Agent stores credentials in a platform-appropriate config path (`/etc/conduit/agent.yaml` on Linux, `/Library/Application Support/Conduit/agent.yaml` on macOS, `C:\ProgramData\Conduit\agent.yaml` on Windows)
   - Agent installs itself as a system service (systemd on Linux, launchd on macOS, Windows service on Windows)
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

### System Service Installation

`conduit join` installs the agent as a system service appropriate to the OS:

**Linux (systemd):**

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

- Binary: `/usr/local/bin/conduit`
- Config: `/etc/conduit/agent.yaml`

**macOS (launchd):**

- Binary: `/usr/local/bin/conduit`
- Config: `/Library/Application Support/Conduit/agent.yaml`
- Plist: `/Library/LaunchDaemons/io.appsynergy.conduit-agent.plist`

**Windows (service):**

- Binary: `C:\Program Files\Conduit\conduit.exe`
- Config: `C:\ProgramData\Conduit\agent.yaml`
- Registered as a Windows service via `sc.exe` or native Go service API

All platforms: service enabled and started immediately, auto-restarts on crash or reboot.

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
- Server tracks each agent: `online`, `offline`, `stale`
- `offline` after heartbeat timeout (45s with no PONG)
- `stale` after extended disconnect (configurable, default 24h)
- Dashboard shows real-time connection status via EventBus
- Reconnecting agent resumes its identity — same agent ID, same labels

---

## Auth Model

### Production Mode (Passkeys)

**WebAuthn is the sole human authentication mechanism.**

- `POST /api/v1/auth/webauthn/register/begin` — start registration
- `POST /api/v1/auth/webauthn/register/finish` — complete registration
- `POST /api/v1/auth/webauthn/login/begin` — start assertion
- `POST /api/v1/auth/webauthn/login/finish` — complete assertion, returns JWT
- JWT: Ed25519 signed, short-lived (15 min access + 24 hour refresh with rotation)
- JWT claims: `sub` (user ID), `tid` (tenant ID), `services` (enabled service slugs, e.g. `["remote-access"]`), `roles`, `permissions`, `iat`, `exp`
- All dashboard/API requests require valid JWT in `Authorization: Bearer` header
- Middleware checks `jwt.services` includes the `x-service` required by each endpoint tag
- Browser WebSocket upgrade includes JWT for auth

### Dev Mode (Setup Token as Password)

- Enabled by `--dev` flag or `server.mode: dev` in `server.yaml`
- Setup token is generated and printed to stdout (same as production)
- After setup, the token is NOT deleted — it persists as the login credential
- `POST /api/v1/auth/password/login` with `{email, password}` (setup token used as password value) — returns JWT
- Server hashes setup token with Argon2id and stores as password_hash
- WebAuthn requires secure context (HTTPS with valid cert or localhost origin) — self-signed dev certs may not satisfy this, so token auth is always available
- If passkey registration succeeds in dev mode, both passkey and token auth remain available
- Dev mode clearly indicated in the UI (banner)
- **Dev mode also uses port 8443 and self-signed certs**
- Webhook URLs: `http://localhost` / `http://127.0.0.1` permitted (loopback only), self-signed HTTPS accepted
- Agent joins: `--dev-insecure` flag to accept self-signed server cert

### Setup Wizard (First Run)

1. Server starts, generates a setup token (CSPRNG, 32 bytes, base64url), prints it to stdout
2. Server opens `localhost:8080` (HTTP only, localhost-only)
3. Wizard collects: setup token, domain name, admin email, org name, first/last name
4. Server validates setup token (constant-time comparison), proves console access (NIST IA-12)
5. Server obtains ACME cert, writes `server.yaml`, creates tenant + admin user + seeds `remote-access` service
6. Restarts on port 443 with TLS
7. Admin submits setup token again + registers passkey via `/setup/passkey`
8. **Production:** setup token DELETE'd from DB (not NULL), passkey-only auth
9. **Dev mode:** setup token persists as password_hash, both passkey and token auth available
10. Setup mode permanently deactivated, `/setup` returns 404 forever after

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
| EXEC_START | 0x40 | Server → Agent | Start bulk exec command |
| EXEC_DATA | 0x41 | Agent → Server | Streaming exec stdout/stderr |
| EXEC_EXIT | 0x42 | Agent → Server | Exec process exited (exit code) |
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
- `/dashboard/audit` — Audit log viewer: search, filter by event type, user, agent, IP, date range, outcome
- `/dashboard/users` — User and group management, RBAC assignments
- `/dashboard/webhooks` — Webhook subscription management, delivery history
- `/dashboard/settings` — Server configuration, session management

### Real-Time Updates
- WebSocket EventBus at `/api/v1/events/stream`
- Agent connect/disconnect events push to all browsers immediately
- No polling. No manual refresh.

---

## Security & Compliance

### NIST Standards

Every security-sensitive endpoint is tagged with `x-nist-controls` and `x-audit-event` extensions.

| Standard | Coverage |
|---|---|
| **NIST SP 800-53 Rev. 5** | AC-2, AC-3, AC-6, AC-7, AC-11, AC-12, AC-17, AU-2, AU-3, AU-6, AU-9, AU-10, AU-12, CM-2, CM-3, CM-6, CM-7, IA-2, IA-4, IA-5, IA-8, IA-12, SC-8, SC-12, SC-13, SC-18, SC-23, SC-28, SI-2, SI-4, SI-7, SI-10, SI-12 |
| **NIST SP 800-63B** | AAL2 baseline, AAL3 with hardware authenticator |
| **NIST SP 800-131A Rev. 2** | Cryptographic algorithm selection and transition |
| **NIST SP 800-57** | Key management lifecycle |
| **FIPS 203 (ML-KEM)** | X25519MLKEM768 hybrid TLS key exchange |
| **FIPS 204 (ML-DSA)** | ML-DSA-65 JWT signing (when Go stdlib ships) |
| **FIPS 205 (SLH-DSA)** | SLH-DSA-SHA2-256s binary signing |

### Cryptographic Policy

**If Conduit controls both ends, PQC is mandatory. If an external party is involved, classical is available with a visible warning and audit log entry. No silent downgrades. Ever.**

| Purpose | Algorithm |
|---|---|
| TLS key exchange | X25519MLKEM768 hybrid (Go 1.24 stdlib) |
| TLS version | 1.3 only (1.0, 1.1, 1.2 disabled) |
| JWT signing | Ed25519 (Phase 1), ML-DSA-65 (future) |
| Binary signing | SLH-DSA-SHA2-256s |
| Agent tokens | HMAC-SHA256 |
| DB field encryption | AES-256-GCM |
| Key derivation | HKDF-SHA256 |
| Password hashing | Argon2id (m=64MB, t=3, p=4) |
| Webhook signatures | HMAC-SHA256 |

**Classical available with warning (external party involved):**

| Context | What is accepted | Why |
|---|---|---|
| WebAuthn | ECDSA P-256, Ed25519, RSA | Hardware authenticator chooses the algorithm |
| ACME / Let's Encrypt | ECDSA P-256 | Let's Encrypt doesn't issue PQC certs yet |
| Self-signed fallback | ECDSA P-256 | Browsers don't support PQC TLS certs yet |
| SAML/OIDC | RSA, ECDSA | External IdP chooses the algorithm |

**Absolutely forbidden (no exceptions):** MD5, SHA-1, DES, 3DES, RC4, TLS 1.0/1.1/1.2, non-CSPRNG sources.

Every use of a classical algorithm where PQC was available is logged with the reason.

### OWASP Standards

Every endpoint is tagged with `x-owasp` extensions in `openapi.yaml` mapping to the applicable OWASP risks. Three standards apply:

**OWASP Top 10 (2021) — Web Application Security Risks:**

| ID | Risk | Conduit CE Controls |
|---|---|---|
| A01 | Broken Access Control | RBAC middleware on every route. Resource ownership verified on every DB query. UUIDs for all IDs. Functional access control tests per role × endpoint. |
| A02 | Cryptographic Failures | TLS 1.3 only. AES-256-GCM at rest. Argon2id for passwords. Ed25519 JWTs. `crypto/rand` exclusively. |
| A03 | Injection | Parameterized SQL only. `oapi-codegen` schema validation. No `os/exec` with user input. |
| A04 | Insecure Design | STRIDE threat modeling. `tenant_id` on every table. Business logic rate limits. |
| A05 | Security Misconfiguration | Security headers on all responses. RFC 9457 errors. CORS allowlist. No debug in prod. |
| A06 | Vulnerable & Outdated Components | `govulncheck` + `npm audit` in CI. SBOM per release. Approved dependencies only. |
| A07 | Identification & Authentication Failures | WebAuthn passkeys (AAL2). Anti-brute-force. Argon2id. No user enumeration. |
| A08 | Software & Data Integrity Failures | All assets embedded. Signed releases. JSON-only deserialization. |
| A09 | Security Logging & Monitoring Failures | `slog` structured JSON. All auth/authz events logged. No secrets in logs. |
| A10 | Server-Side Request Forgery | URL allowlists. Block internal IPs. HTTPS-only outbound. No redirect following. |

**OWASP API Security Top 10 (2023) — API-Specific Risks:**

| ID | Risk | Conduit CE Controls |
|---|---|---|
| API1 | Broken Object Level Authorization | Every DB read/write verifies ownership or role. UUIDs only. Table-driven BOLA tests. |
| API2 | Broken Authentication | Passkeys primary. Stricter rate limits on auth endpoints. JWT validated per request. |
| API3 | Broken Object Property Level Auth | Explicit SQL field selection (no `SELECT *`). Generated response DTOs. No raw body → DB binding. |
| API4 | Unrestricted Resource Consumption | Per-user/IP rate limits. `http.MaxBytesReader`. Max page size 100. `context.WithTimeout` on all queries. |
| API5 | Broken Function Level Authorization | Default deny. RBAC middleware before handler. Admin routes in separate group. |
| API6 | Unrestricted Sensitive Business Flows | Per-operation rate limits. Business logic plausibility checks. |
| API7 | Server-Side Request Forgery | Webhook URL allowlists. Agent identity by token not network. Block metadata endpoints. |
| API8 | Security Misconfiguration | `Content-Type: application/json` enforced. RFC 9457 errors. TLS 1.3. No `Server` header. |
| API9 | Improper Inventory Management | OpenAPI spec is single source of truth. `oapi-codegen` prevents undocumented endpoints. |
| API10 | Unsafe Consumption of APIs | All external data validated as untrusted. TLS required outbound. Timeouts on all external calls. |

**OWASP ASVS v4.0.3 — Verification Standard (Level 2 baseline, Level 3 for auth/crypto/sessions):**

| Chapter | Name | Conduit CE Implementation |
|---|---|---|
| V1 | Architecture | Single auth mechanism (WebAuthn). Single validation layer (`oapi-codegen`). Single logging (`slog`). |
| V2 | Authentication (L3) | 12+ char passwords, Argon2id, breach list check, FIDO2 passkeys, max 100 failed/hour. |
| V3 | Sessions (L3) | New token on auth. 64-bit entropy. `Secure; HttpOnly; SameSite`. 12hr/30min timeouts. Re-auth for sensitive ops. |
| V4 | Access Control | Server-side RBAC. IDOR protection. Anti-CSRF. Segregation of duties. |
| V5 | Validation | Schema validation via `oapi-codegen`. Parameterized SQL. No `eval()`. Mass assignment protection. |
| V6 | Cryptography (L3) | CSPRNG only. AES-256-GCM. Ed25519. No ECB/MD5/SHA-1. Constant-time comparisons. |
| V7 | Errors & Logging | RFC 9457 with request IDs. No secrets in logs. `recover()` middleware. Structured JSON encoding. |
| V8 | Data Protection | `Cache-Control: no-store` on sensitive endpoints. No sensitive data in URLs. Data retention policies. |
| V9 | Communications | TLS 1.3 only. OCSP stapling. Log TLS failures. |
| V10 | Malicious Code | `gosec` + `staticcheck` in CI. No hardcoded creds. Embedded assets. Signed releases. |
| V11 | Business Logic | Sequential processing. Per-user rate limits. DB-level locking for race conditions. |
| V12 | Files & Resources | Size limits. `filepath.Clean` for path traversal. Content-type by content not extension. `Content-Disposition: attachment`. |
| V13 | API Security | No sensitive data in URLs. JSON schema validation. Content-Type enforcement. CSRF via SameSite + Origin. |
| V14 | Configuration | No debug in prod. No version headers. Security headers (CSP, HSTS, etc.). SBOM. `govulncheck` in CI. |

### Transport

| Connection | Primary | Fallback |
|---|---|---|
| Agent to master | QUIC (UDP 443) | WSS (TCP 443) |
| Browser real-time | WebSocket over HTTP/3 | WebSocket over HTTP/2 |
| Browser API | HTTP/3 | HTTP/2 |

### TLS & Certificates

**Production Mode:**
- ACME auto-provisioning via Let's Encrypt
- X25519MLKEM768 hybrid PQC key exchange on all connections
- TLS 1.3 only (1.0/1.1/1.2 disabled)
- Cipher suites: TLS_AES_256_GCM_SHA384, TLS_AES_128_GCM_SHA256, TLS_CHACHA20_POLY1305_SHA256
- Certs stored in `server.yaml`-configured path (default `/var/lib/conduit/certs/`)
- Auto-renewal before expiry
- Ports: UDP 443 + TCP 443

**Dev Mode:**
- Self-signed TLS 1.3 cert generated on startup
- Cert fingerprint printed to console (for agent pinning)
- Ports: UDP 8443 + TCP 8443
- Browser will show cert warning (expected)

### Audit Logging

Every access event is logged: logins, session starts, shell commands, file operations, agent connections, permission changes, configuration changes. Audit logs are append-only and immutable — cannot be modified or deleted by users.

Each log entry includes: who, what, when, where (source IP, agent), and outcome.

---

## SQLite Schema (Community Edition)

```sql
-- Tenant (CE generates one UUID at setup — NIST IA-4)
-- All tables reference this for SaaS transferability
CREATE TABLE tenants (
    id TEXT PRIMARY KEY,              -- UUID v4, generated at setup
    name TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- Service registry (CE ships with one: 'remote-access')
-- SaaS adds more services as rows. JWT 'services' claim lists enabled slugs.
-- Middleware checks jwt.services includes the x-service tag for each endpoint.
CREATE TABLE services (
    id TEXT PRIMARY KEY,              -- UUID v4
    slug TEXT UNIQUE NOT NULL,        -- 'remote-access'
    name TEXT NOT NULL,               -- 'Conduit Remote Access'
    description TEXT,
    created_at TEXT NOT NULL
);

-- Tenant ↔ Service junction (which services a tenant has access to)
CREATE TABLE tenant_services (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    service_id TEXT NOT NULL REFERENCES services(id),
    enabled_at TEXT NOT NULL,
    PRIMARY KEY (tenant_id, service_id)
);

-- Seed at setup time:
-- INSERT INTO services (id, slug, name, description, created_at)
--   VALUES (<uuid>, 'remote-access', 'Conduit Remote Access',
--           'Secure remote shell, file management, and agent orchestration.', <now>);
-- INSERT INTO tenant_services (tenant_id, service_id, enabled_at)
--   VALUES (<tenant_uuid>, <service_uuid>, <now>);

-- Users (single tenant, multiple users with RBAC)
CREATE TABLE users (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    email TEXT UNIQUE NOT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    display_name TEXT,
    role TEXT NOT NULL DEFAULT 'org_member', -- platform_owner, org_owner, org_admin, org_member
    status TEXT NOT NULL DEFAULT 'active', -- active, suspended, invited
    password_hash TEXT,               -- Argon2id, dev mode only (production uses setup token + passkeys)
    created_by TEXT REFERENCES users(id),
    last_login_at TEXT,               -- Nullable, updated on each successful login
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Groups
CREATE TABLE groups (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Group membership (users)
CREATE TABLE group_members (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    group_id TEXT NOT NULL REFERENCES groups(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    PRIMARY KEY (group_id, user_id)
);

-- Group membership (agents) — Agent.groupIds in API
CREATE TABLE agent_group_members (
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    group_id TEXT NOT NULL REFERENCES groups(id),
    agent_id TEXT NOT NULL REFERENCES agents(id),
    created_at TEXT NOT NULL,
    PRIMARY KEY (group_id, agent_id)
);

-- WebAuthn credentials
CREATE TABLE passkeys (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    credential_id BLOB NOT NULL,
    public_key BLOB NOT NULL,
    algorithm TEXT,                   -- e.g., 'ECDSA-P256', 'Ed25519'
    algorithm_warning TEXT,           -- non-null if classical (quantum-vulnerable)
    authenticator_type TEXT,          -- 'platform' or 'cross-platform'
    sign_count INTEGER NOT NULL DEFAULT 0,
    display_name TEXT,                -- User-given name ("MacBook Touch ID")
    created_at TEXT NOT NULL,
    last_used_at TEXT
);

-- SSO providers (SAML/OIDC)
CREATE TABLE sso_providers (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    protocol TEXT NOT NULL,           -- 'saml' or 'oidc'
    name TEXT NOT NULL,
    config TEXT NOT NULL,             -- JSON: provider-specific config
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

-- Registered agents
CREATE TABLE agents (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    hostname TEXT NOT NULL,
    display_name TEXT,
    os TEXT,                          -- 'linux', 'windows', 'darwin'
    arch TEXT,                        -- 'amd64', 'arm64'
    labels TEXT,                      -- JSON: {"env":"production","role":"web"}
    ip TEXT,                          -- Last-known remote IP address
    agent_key_hash TEXT NOT NULL,     -- HMAC key hash for auth
    status TEXT NOT NULL DEFAULT 'offline', -- online, offline, stale
    transport TEXT,                   -- 'quic' or 'websocket'
    version TEXT,                     -- Agent binary version
    last_seen_at TEXT,
    connected_at TEXT,
    created_at TEXT NOT NULL
);

-- Join tokens
CREATE TABLE join_tokens (
    id TEXT PRIMARY KEY,              -- UUID v4 (also the token JTI)
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    type TEXT NOT NULL,               -- 'single_use' or 'persistent'
    name TEXT NOT NULL,
    labels TEXT,                      -- JSON: labels to apply on join
    token_hash TEXT NOT NULL,         -- Hash of the signed JWT
    used_count INTEGER DEFAULT 0,
    max_uses INTEGER,                 -- NULL for unlimited (persistent only)
    expires_at TEXT,                  -- Nullable; NULL = no expiry (persistent tokens)
    revoked INTEGER DEFAULT 0,
    revoked_at TEXT,                  -- Nullable; timestamp when revoked
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL
);

-- Active sessions (for session visibility/revocation)
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    type TEXT NOT NULL,               -- 'web', 'cli', 'ci'
    source_ip TEXT,
    user_agent TEXT,
    refresh_token_hash TEXT,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_active_at TEXT NOT NULL
);

-- Shell sessions (active + closed, for visibility/multiplexing)
CREATE TABLE shell_sessions (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    agent_id TEXT NOT NULL REFERENCES agents(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    status TEXT NOT NULL DEFAULT 'active', -- active, closed
    shell TEXT,                       -- Shell path (e.g. /bin/bash)
    cols INTEGER,
    rows INTEGER,
    recording INTEGER NOT NULL DEFAULT 1, -- Boolean: session recording enabled (NIST AU-2)
    created_at TEXT NOT NULL,
    closed_at TEXT                     -- NULL while active
);

-- Shell recordings (asciicast v2, stored on server)
CREATE TABLE shell_recordings (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    session_id TEXT NOT NULL REFERENCES shell_sessions(id),
    agent_id TEXT NOT NULL REFERENCES agents(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    agent_hostname TEXT,
    user_email TEXT,
    duration INTEGER,                 -- Duration in seconds
    size_bytes INTEGER,               -- File size
    format TEXT NOT NULL DEFAULT 'asciicast-v2',
    created_at TEXT NOT NULL
);

-- RBAC role assignments (user or group → role + scope)
-- Either user_id or group_id must be set (not both, not neither)
CREATE TABLE role_assignments (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    role TEXT NOT NULL,               -- org_owner, org_admin, org_member
    user_id TEXT REFERENCES users(id),  -- NULL if assigned to group
    group_id TEXT REFERENCES groups(id), -- NULL if assigned to user
    scope TEXT,                       -- Resource scope (e.g. specific agent UUID, 'all')
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    CHECK (
        (user_id IS NOT NULL AND group_id IS NULL) OR
        (user_id IS NULL AND group_id IS NOT NULL)
    )
);

-- Audit log (append-only, immutable)
CREATE TABLE audit_log (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    event_type TEXT NOT NULL,         -- 'auth.login', 'shell.start', 'file.download', etc.
    user_id TEXT,                     -- NULL for agent-only events
    user_email TEXT,
    agent_id TEXT,                    -- NULL for user-only events
    agent_hostname TEXT,
    source_ip TEXT,
    user_agent TEXT,
    details TEXT,                     -- JSON: event-specific payload
    outcome TEXT NOT NULL,            -- 'success' or 'failure'
    algorithm_used TEXT,              -- Crypto algorithm used (NIST SP 800-131A)
    algorithm_warning TEXT,           -- Non-null if classical where PQC available
    timestamp TEXT NOT NULL           -- UTC timestamp of the event
);

-- Webhook subscriptions
CREATE TABLE webhook_subscriptions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    url TEXT NOT NULL,
    secret_hash TEXT NOT NULL,        -- HMAC-SHA256 signing key hash
    events TEXT NOT NULL,             -- JSON: ["agent.connected", "auth.login", ...]
    enabled INTEGER NOT NULL DEFAULT 1,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL
);

-- Webhook delivery history
CREATE TABLE webhook_deliveries (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    subscription_id TEXT NOT NULL REFERENCES webhook_subscriptions(id),
    event_type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', -- success, failed, pending
    http_status INTEGER,
    response_time INTEGER,            -- milliseconds
    attempt_number INTEGER NOT NULL DEFAULT 1,
    next_retry_at TEXT,
    delivered_at TEXT,
    attempted_at TEXT NOT NULL
);

-- Bulk exec jobs (parallel command execution across agents)
CREATE TABLE bulk_exec_jobs (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    command TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending', -- pending, running, completed, cancelled, failed
    target_count INTEGER,
    completed_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    results TEXT,                      -- JSON: per-agent results array
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL,
    completed_at TEXT
);

-- Binary deploy jobs (signed binary push to agents)
CREATE TABLE deploy_jobs (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    status TEXT NOT NULL DEFAULT 'uploading', -- uploading, deploying, completed, failed
    target_count INTEGER,
    completed_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    destination_path TEXT,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL
);

CREATE TABLE ci_tokens (
    id TEXT PRIMARY KEY,              -- UUID v4
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    created_by TEXT NOT NULL REFERENCES users(id),
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL,         -- SHA-256 hash of token (plain token shown once at creation)
    scopes TEXT NOT NULL DEFAULT '[]', -- JSON array of scope strings
    expires_at TEXT,                  -- Nullable; NULL = no expiry
    last_used_at TEXT,
    created_at TEXT NOT NULL
);

-- ── Indexes ──────────────────────────────────────────
-- Foreign keys and frequently queried columns

CREATE INDEX idx_users_tenant_id ON users(tenant_id);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_agents_tenant_id ON agents(tenant_id);
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_join_tokens_tenant_id ON join_tokens(tenant_id);
CREATE INDEX idx_join_tokens_revoked ON join_tokens(revoked);
CREATE INDEX idx_sessions_tenant_id ON sessions(tenant_id);
CREATE INDEX idx_sessions_user_id ON sessions(user_id);
CREATE INDEX idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX idx_shell_sessions_tenant_id ON shell_sessions(tenant_id);
CREATE INDEX idx_shell_sessions_agent_id ON shell_sessions(agent_id);
CREATE INDEX idx_shell_sessions_user_id ON shell_sessions(user_id);
CREATE INDEX idx_shell_sessions_status ON shell_sessions(status);
CREATE INDEX idx_shell_recordings_tenant_id ON shell_recordings(tenant_id);
CREATE INDEX idx_shell_recordings_session_id ON shell_recordings(session_id);
CREATE INDEX idx_shell_recordings_agent_id ON shell_recordings(agent_id);
CREATE INDEX idx_shell_recordings_user_id ON shell_recordings(user_id);
CREATE INDEX idx_role_assignments_tenant_id ON role_assignments(tenant_id);
CREATE INDEX idx_role_assignments_user_id ON role_assignments(user_id);
CREATE INDEX idx_role_assignments_group_id ON role_assignments(group_id);
CREATE INDEX idx_audit_log_tenant_id ON audit_log(tenant_id);
CREATE INDEX idx_audit_log_event_type ON audit_log(event_type);
CREATE INDEX idx_audit_log_timestamp ON audit_log(timestamp);
CREATE INDEX idx_audit_log_user_id ON audit_log(user_id);
CREATE INDEX idx_webhook_subscriptions_tenant_id ON webhook_subscriptions(tenant_id);
CREATE INDEX idx_webhook_deliveries_tenant_id ON webhook_deliveries(tenant_id);
CREATE INDEX idx_webhook_deliveries_subscription_id ON webhook_deliveries(subscription_id);
CREATE INDEX idx_webhook_deliveries_status ON webhook_deliveries(status);
CREATE INDEX idx_passkeys_tenant_id ON passkeys(tenant_id);
CREATE INDEX idx_group_members_tenant_id ON group_members(tenant_id);
CREATE INDEX idx_agent_group_members_tenant_id ON agent_group_members(tenant_id);
CREATE INDEX idx_sso_providers_tenant_id ON sso_providers(tenant_id);
CREATE INDEX idx_tenant_services_tenant_id ON tenant_services(tenant_id);
CREATE INDEX idx_bulk_exec_jobs_tenant_id ON bulk_exec_jobs(tenant_id);
CREATE INDEX idx_bulk_exec_jobs_status ON bulk_exec_jobs(status);
CREATE INDEX idx_bulk_exec_jobs_created_by ON bulk_exec_jobs(created_by);
CREATE INDEX idx_deploy_jobs_tenant_id ON deploy_jobs(tenant_id);
CREATE INDEX idx_deploy_jobs_status ON deploy_jobs(status);
CREATE INDEX idx_ci_tokens_tenant_id ON ci_tokens(tenant_id);
CREATE INDEX idx_ci_tokens_created_by ON ci_tokens(created_by);
```

Migrations embedded in binary, applied automatically at startup.

---

## What Is Explicitly Out of Scope (SaaS / Later Phases Only)

- **Multi-tenancy hierarchy** (sub-tenants, three-level hierarchy, visibility modes) — CE has single tenant with multiple users and full RBAC
- **Billing / Stripe** (plans, subscriptions, usage metering, plan builder)
- **SaaS mode** (`server.mode: saas`, tenant signup flow, subdomain routing)
- **Per-tenant object storage bucket**
- **Vault** (secrets & key management, Shamir Secret Sharing, TPM-sealed delivery)
- **PKI** (root CAs, intermediate CAs, certificate issuance, CRL, OCSP)
- **Serial & hardware** (serial console connections, OS installation over serial)
- **PXE / iPXE provisioning** (network boot, OS profile builder)
- **Kubernetes / k3s orchestration UI**
- **Legacy SSH connections** (stored credentials, SFTP)
- **PIV / smart card authentication**
- **iOS app**
- **Cluster mode** (rqlite, multi-master federation)
- **Anycast / embedded DNS management**
- **Terraform provider**
- **AppSynergy QUIC Tunnel Service** (separate VPN product)
- **Network & routing** (BGP, OSPF, topology visualization)
- **Log management** (collection agent, ingestion pipeline, FTS5 search)
- **BMC/IPMI out-of-band management** (Redfish, iDRAC, iLO, AMT, DASH)
- **Infrastructure TV dashboards** (Mission Control, TV/projector display mode)

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
4. On a remote machine (Linux, macOS, or Windows): `conduit join <url> <token>` installs the agent as a system service
5. Agent appears in the dashboard in real-time (no refresh)
6. Operator clicks agent → live terminal works like SSH (full PTY, resize, interactive programs)
7. Operator switches to file browser → navigates directories, downloads a file, uploads a file
8. Operator uses TUI: `conduit` (no args) → sees agent list → enters shell
9. Kill the agent process → dashboard shows disconnected immediately → agent auto-restarts (via the OS service manager) and reconnects
10. Network blip → agent reconnects automatically with no operator intervention
11. Community Edition landing page loads at `/` with self-host pitch
12. Unauthenticated access to dashboard/API returns 401
13. Invalid join token is rejected

---

## Open Questions

None remaining — ready to build. If anything surfaces during implementation, we'll address it inline.
