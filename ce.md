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
- SQLite database with encrypted storage (SQLCipher AES-256-GCM)
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
- Agent installed as systemd service via `conduit join`

### Shell & Terminal
- Shell sessions in browser (xterm.js, full feature parity)
- PTY shell execution on agent (Linux)
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
- Webhooks on audit events (HMAC-SHA256 signed payloads)
- Webhook subscription management (create, update, delete, test)
- Webhook delivery history with retry tracking
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

- `POST /api/v1/auth/webauthn/register/begin` — start registration
- `POST /api/v1/auth/webauthn/register/finish` — complete registration
- `POST /api/v1/auth/webauthn/login/begin` — start assertion
- `POST /api/v1/auth/webauthn/login/finish` — complete assertion, returns JWT
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
| **NIST SP 800-53 Rev. 5** | AC-2, AC-3, AC-6, AC-7, AC-11, AC-12, AC-17, AU-2, AU-3, AU-6, AU-9, AU-10, AU-12, CM-2, CM-3, CM-6, CM-7, IA-2, IA-4, IA-5, IA-8, IA-12, SC-8, SC-12, SC-13, SC-23, SC-28, SI-2, SI-4, SI-7, SI-10 |
| **NIST SP 800-63B** | AAL3 passkey authentication |
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
-- Users (single tenant, multiple users with RBAC)
CREATE TABLE users (
    id TEXT PRIMARY KEY,              -- UUID v4
    email TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'org_member', -- platform_owner, org_owner, org_admin, org_member
    password_hash TEXT,               -- Argon2id, NULL after passkey setup
    suspended INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Groups
CREATE TABLE groups (
    id TEXT PRIMARY KEY,              -- UUID v4
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    created_at TEXT NOT NULL
);

-- Group membership
CREATE TABLE group_members (
    group_id TEXT NOT NULL REFERENCES groups(id),
    user_id TEXT NOT NULL REFERENCES users(id),
    created_at TEXT NOT NULL,
    PRIMARY KEY (group_id, user_id)
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

-- SSO providers (SAML/OIDC)
CREATE TABLE sso_providers (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,               -- 'saml' or 'oidc'
    name TEXT NOT NULL,
    config TEXT NOT NULL,             -- JSON: provider-specific config
    enabled INTEGER NOT NULL DEFAULT 1,
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
    transport TEXT,                   -- 'quic' or 'websocket'
    version TEXT,                     -- Agent binary version
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
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL
);

-- Active sessions (for session visibility/revocation)
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    type TEXT NOT NULL,               -- 'web', 'cli', 'ci'
    source_ip TEXT,
    user_agent TEXT,
    refresh_token_hash TEXT,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- Audit log (append-only, immutable)
CREATE TABLE audit_log (
    id TEXT PRIMARY KEY,              -- UUID v4
    event_type TEXT NOT NULL,         -- 'auth.login', 'shell.start', 'file.download', etc.
    user_id TEXT,                     -- NULL for agent-only events
    agent_id TEXT,                    -- NULL for user-only events
    source_ip TEXT,
    detail TEXT,                      -- JSON: event-specific payload
    outcome TEXT NOT NULL,            -- 'success' or 'failure'
    created_at TEXT NOT NULL
);

-- Webhook subscriptions
CREATE TABLE webhooks (
    id TEXT PRIMARY KEY,
    url TEXT NOT NULL,
    secret_hash TEXT NOT NULL,        -- HMAC-SHA256 signing key hash
    events TEXT NOT NULL,             -- JSON: ["agent.connect", "auth.login", ...]
    enabled INTEGER NOT NULL DEFAULT 1,
    created_by TEXT REFERENCES users(id),
    created_at TEXT NOT NULL
);

-- Webhook delivery history
CREATE TABLE webhook_deliveries (
    id TEXT PRIMARY KEY,
    webhook_id TEXT NOT NULL REFERENCES webhooks(id),
    event_type TEXT NOT NULL,
    status_code INTEGER,
    response_body TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    delivered_at TEXT,
    created_at TEXT NOT NULL
);
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
- **Platform-level JWT** (multi-product `products` claim)
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
