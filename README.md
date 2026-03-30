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
- **Agents**: Linux (systemd), macOS (launchd), Windows (service) — amd64 + arm64
- **Frontend**: Next.js static export + shadcn/ui + Tailwind CSS
- **TUI**: bubbletea (shell-only for CE)

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
