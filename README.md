# Conduit CE (Community Edition)

Unified server + agent platform for managing remote infrastructure. Single binary, multi-OS agents, real-time web dashboard.

## Standards

- **NIST**: SP 800-53 Rev. 5, SP 800-63B, SP 800-228, SP 800-218, CSF 2.0
- **OWASP**: Top 10 (2021), API Security Top 10 (2023), ASVS v4.0.3 (L2 baseline, L3 for auth/crypto)
- **Crypto**: PQC-first — X25519MLKEM768 (TLS), Ed25519 (JWT), SLH-DSA-SHA2-256s (binary signing). TLS 1.3 only. Classical forbidden unless external party forces it.

Full security spec: [`CLAUDE.md` — Security Standards](CLAUDE.md)

## Architecture

- **Server**: Go binary with embedded Next.js static frontend — single self-contained executable
- **Persistence**: SQLite (single tenant, `tenant_id` on all records for SaaS transferability)
- **Transport**: QUIC primary (UDP 443), WebSocket fallback (TCP 443)
- **Auth**: WebAuthn passkeys (AAL2 minimum), Ed25519 JWTs
- **Agents**: Linux (systemd) amd64+arm64, macOS (launchd) arm64, Windows (service) amd64
- **Frontend**: Next.js static export + shadcn/ui + Tailwind CSS
- **TUI**: bubbletea (shell-only for CE)

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

## Documentation

| Doc | Purpose |
|-----|---------|
| [`CLAUDE.md`](CLAUDE.md) | Everything — security standards, product spec, DB schema, coding rules |
| [`openapi.yaml`](openapi.yaml) | API contract — endpoint shapes, schemas, NIST/OWASP annotations |

---

## Implementation Status

✅ done | 🟡 in-progress | ❌ spec-only

### Server Core

| Feature | Status | Dependencies |
|---------|--------|--------------|
| QUIC + WebSocket dual listeners (UDP 443 + TCP 443) | ✅ done | — |
| HTTP/3 serving for browsers + HTTP/2 fallback | ✅ done | — |
| ACME TLS auto-provisioning (Let's Encrypt) | ✅ done | — |
| X25519MLKEM768 hybrid PQC TLS on all connections | ✅ done | — |
| SQLite database (pure Go, `modernc.org/sqlite`) with AES-256-GCM on sensitive fields | ✅ done | — |
| Embedded static frontend via `embed.FS` | ✅ done | Frontend |
| First-run setup wizard (localhost:8080 → ACME → HTTPS → forced passkey) | ✅ done | Auth, DB |
| WebSocket EventBus for real-time dashboard updates | ✅ done | HTTP server |
| `server.yaml` configuration file | ✅ done | — |
| Input validation middleware — unknown JSON fields rejected | ✅ done | Router |
| Parameterized queries only — no raw SQL interpolation | ✅ done | DB |

### Auth & Identity

| Feature | Status | Dependencies |
|---------|--------|--------------|
| WebAuthn passkey registration + login (sole production auth) | ✅ done | DB, Users |
| Dev-mode password fallback (Argon2id hashed) | ✅ done | DB, Users |
| JWT issuance and validation (Ed25519 signed, short-lived) | ✅ done | — |
| Users + groups + RBAC (platform_owner, org_owner, org_admin, org_member) | ✅ done | DB |
| Account recovery via one-time recovery codes (Argon2id hashed, single-use) | ✅ done | Auth, Users, Passkeys |
| Admin-assisted account recovery (reset user auth state) | ✅ done | Auth, Users, RBAC |
| SAML/OIDC SSO (passkeys remain primary) | ❌ spec-only | Auth, Users |
| CLI browser device flow (passkey → CLI token) | ✅ done | Auth, JWT |
| CLI credential storage (encrypted, per-profile) | ✅ done | CLI |
| All authenticated sessions (web, CLI, CI) visible and revocable | ✅ done | Auth, DB |
| CI token support (`CONDUIT_TOKEN` env var, scoped, revocable) | ✅ done | Auth, DB |

### Agent & Connections

| Feature | Status | Dependencies |
|---------|--------|--------------|
| Agent outbound QUIC connection (primary) | ✅ done | CWP |
| Agent WebSocket transport with reconnection (exponential backoff + jitter) | ✅ done | CWP |
| CWP wire protocol — identical binary framing over QUIC and WebSocket | ✅ done | — |
| Agent registration flow (single-use + persistent join tokens) | ✅ done | Auth, DB |
| Agent WebSocket connection handler + CWP authentication | ✅ done | CWP, EventBus |
| Agent heartbeat and connection status | ✅ done | CWP, EventBus |
| Dashboard shows transport type per agent (QUIC vs WebSocket) | ✅ done | EventBus |
| Linux amd64 + arm64 agent builds | ✅ done | Agent |
| Windows amd64 agent build | 🟡 in-progress | Agent |
| macOS arm64 agent build + launchd service | ✅ done | Agent |
| Agent auto-update with rollback (signed binary push) | ❌ spec-only | Agent, Binary Signing |
| Full host visibility per agent: CPU, memory, disk, network, services, ports | 🟡 in-progress | CWP, Dashboard |
| Agent metrics collection (CPU, RAM, disk, load, uptime — `AGENT_INFO` frames) | ✅ done | CWP |
| Universal resource labelling system for surgical targeting | ✅ done | DB |
| Agent installed as system service (systemd / launchd / Windows) via `conduit join` | ✅ done | Agent |
| Agent binary hosting + install script (one-line curl install) | ✅ done | Server |
| Dashboard join token management (create, revoke, install command generator) | ✅ done | Frontend, Auth |

### Shell & Terminal

| Feature | Status | Dependencies |
|---------|--------|--------------|
| Shell sessions in browser (xterm.js, full feature parity) | ✅ done | CWP, Frontend |
| PTY shell execution on agent (Linux, macOS via creack/pty) | ✅ done | Agent |
| Multiple concurrent shell sessions per agent (multiplexed via CWP) | ✅ done | CWP |
| Terminal session recording + playback (asciicast v2) | ✅ done | Shell, DB |
| Persistent/resumable sessions — survive browser disconnect, resume anywhere | ✅ done | Shell, SessionManager |
| Pin mode — indefinite sessions for monitoring long-running tasks | ✅ done | SessionManager |
| Session detach/attach with ring buffer output replay | ✅ done | SessionManager |
| Pop-out terminal windows (standalone, minimal chrome) | ✅ done | Frontend, Shell |

### File Management

| Feature | Status | Dependencies |
|---------|--------|--------------|
| Browser-based file manager (list, download, upload, delete, rename, mkdir, preview) | ✅ done | CWP, Frontend |
| File transfer over HTTPS/QUIC — no SFTP dependency | ✅ done | CWP |
| Resumable file uploads | ✅ done | CWP |

### CLI & TUI

| Feature | Status | Dependencies |
|---------|--------|--------------|
| CLI binary (`conduit`) — agent daemon, join, uninstall, token, shell subcommands | ✅ done | Auth, CWP |
| CLI runs as full TUI (bubbletea) when no arguments given | ✅ done | CLI |
| CLI-only mode on personal devices (zero daemons, zero listeners) | ❌ spec-only | CLI |
| CLI shell — `conduit shell <agent>` direct shell without TUI | ✅ done | CLI, CWP |
| CLI exec commands — bulk exec from terminal | ❌ spec-only | CLI, Bulk Exec |
| CLI group, user, audit management commands | ✅ done | CLI, Auth |
| CLI lights-out basic (reboot/poweroff via agent) | ✅ done | CLI, CWP |
| Shell completions (bash, zsh, fish, PowerShell) | ✅ done | CLI |

### Bulk Operations

| Feature | Status | Dependencies |
|---------|--------|--------------|
| Bulk command execution — multi-server parallel script runner | ✅ done | CWP, Agent |
| Binary deployment service | ❌ spec-only | Agent, Binary Signing |

### Audit & Compliance

| Feature | Status | Dependencies |
|---------|--------|--------------|
| Audit logging for all access events | ✅ done | DB |
| Audit log viewer in dashboard (search, filter, query) | ✅ done | Frontend, DB |
| Webhooks on audit events (HMAC-SHA256 signed, HTTPS-only in prod) | ✅ done | DB |
| Webhook subscription management (create, update, delete, test) | ✅ done | DB |
| Webhook delivery history with retry tracking | ✅ done | DB |
| Dev mode: webhook delivery to `http://localhost` loopback | ✅ done | Webhooks |
| NIST SP 800-53 Rev. 5 control coverage | ❌ spec-only | All |
| NIST SP 800-131A cryptographic compliance | ❌ spec-only | All |
| Security audit readiness (SOC 2 Type II, penetration test ready) | ❌ spec-only | All |

### Real-Time Dashboard

| Feature | Status | Dependencies |
|---------|--------|--------------|
| WebSocket EventBus for live updates — no polling | ✅ done | HTTP server |
| Agent connect/disconnect events push to all browsers | ✅ done | EventBus |
| Shell session start/stop events | ✅ done | EventBus, Shell |
| File operation events | ✅ done | EventBus, Files |
| Auth events (login, logout, session revocation) | ✅ done | EventBus, Auth |
| Agent metrics streaming | ✅ done | EventBus, CWP |
| Channel-based subscriptions (agents, shell, files, auth, audit, metrics, exec, system) | ✅ done | EventBus |

### Promotional Website

| Feature | Status | Dependencies |
|---------|--------|--------------|
| Public marketing landing page served from same binary | ✅ done | Frontend |
| Fortune 500-quality UI/UX | ✅ done | Frontend |
| SEO optimized for remote access, infrastructure access keywords | ❌ spec-only | Frontend |
| Pre-generated OG images (build-time, static assets in `public/og/`) | ❌ spec-only | Frontend |
| Core Web Vitals targets met (LCP <=2.5s, INP <=200ms, CLS <=0.1) | ❌ spec-only | Frontend |

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

## Out of Scope (SaaS / Later Phases Only)

- Multi-tenancy hierarchy (sub-tenants, three-level hierarchy, visibility modes)
- Billing / Stripe (plans, subscriptions, usage metering, plan builder)
- SaaS mode (`server.mode: saas`, tenant signup flow, subdomain routing)
- Per-tenant object storage bucket
- Vault (secrets & key management, Shamir Secret Sharing, TPM-sealed delivery)
- PKI (root CAs, intermediate CAs, certificate issuance, CRL, OCSP)
- Serial & hardware (serial console connections, OS installation over serial)
- PXE / iPXE provisioning (network boot, OS profile builder)
- Kubernetes / k3s orchestration UI
- Legacy SSH connections (stored credentials, SFTP)
- PIV / smart card authentication
- iOS app
- Cluster mode (rqlite, multi-master federation)
- Anycast / embedded DNS management
- Terraform provider
- AppSynergy QUIC Tunnel Service (separate VPN product)
- Network & routing (BGP, OSPF, topology visualization)
- Log management (collection agent, ingestion pipeline, FTS5 search)
- BMC/IPMI out-of-band management (Redfish, iDRAC, iLO, AMT, DASH)
- Infrastructure TV dashboards (Mission Control, TV/projector display mode)
