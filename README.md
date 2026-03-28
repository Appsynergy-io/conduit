# Conduit CE — Community Edition

Secure remote access to every machine. No SSH keys. No VPNs. No inbound ports.

Conduit CE is a self-hosted remote infrastructure management platform. A single Go binary gives you browser-based shell access, file management, and real-time visibility into every machine you manage — authenticated with passkeys, encrypted with PQC hybrid TLS, and connected over QUIC.

This is the open foundation that the full Conduit SaaS platform is built on.

---

## What It Does

- **Remote shell** — Full PTY terminal in the browser (xterm.js) or TUI (bubbletea). Works like SSH, without SSH.
- **File manager** — Browse directories, upload, download, delete, rename, mkdir on any connected machine.
- **Zero inbound ports** — Agents connect outbound only. Works behind NAT, CGNAT, corporate firewalls, dynamic IPs.
- **QUIC + WebSocket** — QUIC primary transport with automatic WebSocket fallback when UDP is blocked.
- **Passkey auth** — WebAuthn passkeys are the sole production authentication. No passwords to rotate.
- **SAML/OIDC SSO** — Integrate with your existing identity provider. Passkeys remain primary.
- **RBAC** — Full role hierarchy: platform_owner, org_owner, org_admin, org_member. Users, groups, permissions.
- **PQC hybrid TLS** — X25519MLKEM768 post-quantum key exchange on every connection Conduit controls.
- **Let's Encrypt TLS** — Automatic certificate provisioning. TLS 1.3 only.
- **Agent enrollment** — Join tokens with label scoping. One command installs and starts the agent as a service.
- **Multi-OS agents** — Linux (systemd), macOS (launchd), Windows (service). amd64 + arm64.
- **Real-time dashboard** — Agent status, connections, and events stream live via WebSocket EventBus. No polling.
- **Audit logging** — Every access event logged: who, what, when, where, outcome. Append-only, immutable.
- **Webhooks** — HMAC-SHA256 signed payloads on audit events. Subscription management with delivery history. HTTPS-only in production; dev mode permits `http://localhost` loopback and self-signed HTTPS.
- **Bulk exec** — Run commands across multiple agents in parallel with streaming output.
- **Terminal recording** — Session recording and playback (asciicast v2).
- **Agent auto-update** — Signed binary push from server to all agents.
- **Service registry** — Extensible service abstraction. CE ships with one service (`remote-access`). Each endpoint is tagged with `x-service`; middleware enforces `jwt.services`. Adding a new service to the SaaS platform is a new table row, not an architecture change.

## Architecture

```
┌────────────────────────────────────────────────────┐
│              conduit-server (single Go binary)      │
│                                                    │
│  QUIC Listener ─── HTTP/3 + HTTP/2 ─── embed.FS   │
│  (agent conns)     (browser + API)     (Next.js    │
│                                        dashboard + │
│                                        promo site) │
│                                                    │
│  Auth: WebAuthn / SAML / OIDC (prod)               │
│        Setup token (dev mode only)                 │
│  JWT: Ed25519  Services: jwt.services middleware   │
│  SQLite        Let's Encrypt                       │
│  TLS: X25519MLKEM768 PQC hybrid                    │
└────────────────────────────────────────────────────┘
       ▲ QUIC/WSS                  ▲ HTTPS + WSS
       │                           │
  ┌────┴─────┐               ┌────┴─────┐
  │  Agent   │               │ Browser  │
  │ (conduit)│               │ Dashboard│
  │ service  │               └──────────┘
  └──────────┘
       ┌──────────┐
       │   TUI    │
       │ (conduit)│
       │bubbletea │
       └──────────┘
```

**Two binaries:**

| Binary | Purpose |
|---|---|
| `conduit-server` | Master server. QUIC + HTTP listeners, API, auth, SQLite, embedded frontend. |
| `conduit` | Agent (daemon), CLI, and interactive TUI. Single binary, multiple modes. |

## Quick Start

### 1. Start the Server

```bash
# Production — runs setup wizard, obtains Let's Encrypt cert
./conduit-server

# Dev mode — self-signed cert on port 8443
./conduit-server --dev
```

First run generates a **setup token** and prints it to the server console. Open `localhost:8080`, enter the setup token along with your domain and admin email. The server obtains a TLS cert, restarts on port 443, and requires the setup token again to register your passkey. The setup token is deleted permanently after passkey registration. In dev mode, the setup token persists as the login credential (email + token as password) because WebAuthn requires a secure context.

### 2. Enroll an Agent

Generate a join token from the dashboard or CLI:

```bash
conduit token create --labels env=production,role=web --type persistent --ttl 24h
```

On the target machine:

```bash
conduit join https://conduit.example.com <token>
```

This single command:
- Validates the token with the server
- Receives a unique agent identity (UUID + HMAC-SHA256 key)
- Applies the labels from the token
- Installs the binary and writes config
- Installs and starts a system service (systemd / launchd / Windows service)
- Connects to the server immediately

### 3. Use It

**Browser:** Open `https://conduit.example.com`, authenticate with your passkey, click an agent, drop into a terminal.

**TUI:** Run `conduit` with no arguments for an interactive agent list. Press Enter to shell in.

**CLI:** `conduit shell <agent-name>` for a direct connection.

## Agent Connection Strategy

Agents maintain a persistent connection to the server at all times.

- **QUIC primary** (UDP 443) with 0-RTT reconnection
- **WebSocket fallback** (TCP 443) when UDP is blocked
- **Exponential backoff** with jitter on disconnect (1s → 30s cap)
- **Transport alternation** — if one transport fails 3 times, tries the other
- **Heartbeat** — PING/PONG every 15s, connection declared dead after 3 missed
- **Network change detection** — immediate reconnect on interface change

## Join Token Security

Tokens are signed JWTs containing labels, type (single-use or persistent), and expiry. Single-use tokens are deleted after first successful enrollment. Persistent tokens support fleet enrollment (cloud-init, Ansible) and can be revoked from the dashboard.

After enrollment, the agent authenticates with its unique HMAC-SHA256 key on every connection. The join token is never used again.

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

### Transport

| Connection | Primary | Fallback |
|---|---|---|
| Agent to master | QUIC (UDP 443) | WSS (TCP 443) |
| Browser real-time | WebSocket over HTTP/3 | WebSocket over HTTP/2 |
| Browser API | HTTP/3 | HTTP/2 |

### Audit Logging

Every access event is logged: logins, session starts, shell commands, file operations, agent connections, permission changes, configuration changes. Audit logs are append-only and immutable. Searchable and filterable in the dashboard.

Each log entry includes: who, what, when, where (source IP, agent), and outcome.

## Tech Stack

| Layer | Technology |
|---|---|
| Server | Go 1.24+, `quic-go`, SQLite (SQLCipher AES-256-GCM encrypted) |
| Frontend | Next.js 15 (static export), shadcn/ui, Tailwind v4, xterm.js |
| TUI | bubbletea |
| Auth | WebAuthn (passkeys), SAML/OIDC SSO, Ed25519 JWT |
| Transport | QUIC (primary), WebSocket (fallback), CWP binary wire protocol |
| TLS | TLS 1.3 only, X25519MLKEM768 PQC hybrid, Let's Encrypt ACME |
| Agent | Go, Linux/macOS/Windows, systemd/launchd/Windows service |

## Project Structure

```
conduit/
├── ce.md                       # Community Edition specification
├── directive.md                # Project directive (single source of truth)
├── openapi.yaml                # OpenAPI 3.1.1 API specification
├── cmd/
│   ├── conduit-server/         # Server binary
│   └── conduit/                # Agent + CLI + TUI binary
├── internal/
│   ├── protocol/               # CWP wire protocol
│   ├── server/                 # Server core (QUIC, HTTP, auth, routing, audit, webhooks)
│   ├── agent/                  # Agent (shell, files, transport, install per OS)
│   ├── tui/                    # bubbletea TUI
│   ├── auth/                   # JWT, WebAuthn, SSO, join tokens
│   ├── db/                     # SQLite + migrations
│   └── shared/                 # TLS, config
├── web/                        # Next.js 15 frontend
│   ├── app/                    # Pages (landing, login, dashboard, audit, users, webhooks)
│   └── components/             # shadcn/ui + terminal + file browser
└── embed.go                    # //go:embed web/out/*
```

## API Specification

The full REST API is defined in `openapi.yaml` (OpenAPI 3.1.1, ~6,100 lines, 81 paths, 101 operations + 7 webhook callbacks, 40 schemas).

```bash
# Redocly preview
npx @redocly/cli preview-docs openapi.yaml

# Generate Go server stubs
oapi-codegen -generate types,server,spec -package api openapi.yaml > api/api.gen.go

# Generate TypeScript client types
npx openapi-typescript openapi.yaml -o src/lib/api-types.ts

# Lint
npx @redocly/cli lint openapi.yaml
```

### Key Design Decisions

1. **Single file** — one spec file, no split `$ref` directories. Simplifies code generation.
2. **Cursor-based pagination** — all list endpoints use opaque cursors, not offset/limit.
3. **RFC 9457 errors** — all errors use `application/problem+json` with field-level validation.
4. **`additionalProperties: false`** — all input schemas reject unknown fields. No silent ignore.
5. **`x-nist-controls`, `x-audit-event`, and `x-service`** — custom extensions for security controls, audit events, and service scoping on operations.
6. **Nullable via type arrays** — OpenAPI 3.1 `type: ["string", "null"]` syntax.
7. **WebSocket as upgrade endpoints** — WS endpoints documented as GET with 101 response.
8. **Service registry** — endpoint tags carry `x-service` extensions; middleware enforces `jwt.services` claim. CE ships with one service (`remote-access`).

## What CE Does Not Include

These features are part of the full Conduit SaaS platform or later phases. This list exists so we know exactly what was left out — everything not on this list is in CE.

| Feature | Why excluded |
|---|---|
| Multi-tenancy hierarchy (sub-tenants, visibility modes) | SaaS-only. CE generates a tenant UUID at setup time (NIST IA-4) with multiple users + RBAC. All records carry a `tenantId` field for transferability to the multi-tenant SaaS. |
| Billing / Stripe (plans, subscriptions, usage metering) | SaaS-only. |
| SaaS mode (tenant signup, subdomain routing) | SaaS-only. |
| Per-tenant object storage bucket | SaaS-only. Tied to multi-tenancy. |
| Vault (secrets & key management, Shamir Secret Sharing, TPM-sealed delivery) | Later phase. |
| PKI (root CAs, intermediate CAs, certificate issuance, CRL, OCSP) | Later phase. |
| Serial & hardware (serial console connections, OS installation over serial) | Later phase. |
| PXE / iPXE provisioning (network boot, OS profile builder) | Later phase. |
| Kubernetes / k3s orchestration UI | Later phase. |
| Legacy SSH connections (stored credentials, SFTP) | Later phase. |
| PIV / smart card authentication | Later phase. |
| iOS app | Later phase. |
| Cluster mode (rqlite, multi-master federation) | Later phase. |
| Anycast / embedded DNS management | Later phase. |
| Terraform provider | Later phase. |
| AppSynergy QUIC Tunnel Service (separate VPN product) | Separate product. |
| Network & routing (BGP, OSPF, topology visualization) | Later phase. |
| Log management (collection agent, ingestion pipeline, FTS5 search) | Later phase. |
| BMC/IPMI out-of-band management (Redfish, iDRAC, iLO, AMT, DASH) | Later phase. |
| Infrastructure TV dashboards (Mission Control, TV/projector display mode) | Later phase. |

## License

Proprietary. Copyright AppSynergy.
