# Conduit by appsynergy — Project Directive

This is the single source of truth for building Conduit. All prior spec documents, CLAUDE.md files, and correction documents are superseded by this file. If anything conflicts with this document, this document wins.

---

## 1. Vision

Conduit is a remote infrastructure management platform that eliminates the pain points of traditional server administration. It gives operators secure, instant shell access to any machine — without opening a single inbound port, without managing SSH keys, without VPNs, and without trusting the network.

Every managed machine runs a lightweight agent that connects outbound to the Conduit master server. The operator opens a browser, authenticates with a passkey, and has a full terminal session, file manager, and management dashboard in seconds. No firewall rules to configure. No credentials to rotate. No attack surface to expose.

Conduit is designed for:

- **IT teams** managing fleets of servers behind NAT, CGNAT, corporate firewalls, and dynamic IPs
- **MSPs and hosting providers** who need multi-tenant infrastructure management with per-customer isolation
- **Security-conscious organizations** that want zero-trust access without the complexity of bastion hosts, jump servers, and SSH key sprawl
- **Solo operators** who want a single pane of glass for all their machines

The platform is built as a multi-tenant SaaS from day one — AppSynergy operates the hosted version, but any organization can self-host a private instance from the same binary. Tenants can create sub-tenants for their own clients, making Conduit a platform that MSPs and resellers can build businesses on.

Conduit is also the foundation of the broader AppSynergy product ecosystem. The same platform, identity system, and dashboard shell will host the AppSynergy QUIC Tunnel Service (a managed VPN product replacing WireGuard), network routing management, log management, and other infrastructure services — each as separate product offerings within a unified platform.

### What Conduit Does Not Do

- Act as a full SSH server (agents use the OS's local SSH or exec PTY directly)
- Support password-based login in production mode (dev mode and the first-run setup wizard are the sole exceptions — see §7)
- Support legacy TLS (1.0, 1.1, 1.2)
- Replace a full Kubernetes control plane (Conduit orchestrates via kubectl/k3s CLI on agents, it is not a new k8s API server)
- Integrate with HashiCorp Vault (Conduit has its own built-in Vault feature — see §72, Phase 3+)

### Core Principles

- **Zero inbound ports.** Agents connect outbound only. The master never dials agents. Works behind any firewall.
- **Passkeys only.** No passwords in production. No SSH keys to manage. WebAuthn is the sole authentication mechanism.
- **Post-quantum from day one.** Every connection Conduit controls uses PQC hybrid cryptography. Classical algorithms are accepted only where external parties (hardware authenticators, CAs, legacy SSH servers) require them — always with a warning and audit log entry.
- **QUIC first.** Every connection type uses QUIC as the primary transport with automatic WebSocket/HTTP fallback. One policy, everywhere, no exceptions.
- **Multi-tenant always.** The three-level hierarchy (platform → tenant → sub-tenant) exists from the first line of code. Every table has `tenant_id`. Every query is scoped. Self-hosted mode is just single-tenant SaaS mode.
- **Static frontend.** The UI is a static Next.js export embedded in the Go binary. No Node.js in production. No SSR. The Go backend is the only runtime.
- **Two binaries, two repos.** `conduit-server` (the master) and `conduit` (the agent + CLI). They are never merged.

---

## 2. Architecture

### 2.1 System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                     conduit-server (master)                     │
│                                                                 │
│  ┌───────────┐  ┌──────────┐  ┌──────────┐  ┌───────────────┐  │
│  │  HTTP/3 +  │  │ WebSocket│  │   QUIC   │  │   Embedded    │  │
│  │  HTTP/2    │  │ EventBus │  │  Streams  │  │   Static UI   │  │
│  │  (browser) │  │ (browser)│  │  (agents) │  │  (embed.FS)   │  │
│  └─────┬─────┘  └────┬─────┘  └─────┬────┘  └───────────────┘  │
│        │              │              │                           │
│  ┌─────┴──────────────┴──────────────┴────┐  ┌───────────────┐  │
│  │            Go API + Auth               │  │  ACME / TLS   │  │
│  │       (JWT, RBAC, tenant scoping)      │  │  (Let's Enc.) │  │
│  └─────────────────┬──────────────────────┘  └───────────────┘  │
│                    │                                             │
│  ┌─────────────────┴──────────────────────┐                     │
│  │   SQLite (tenant-scoped query wrapper) │                     │
│  └────────────────────────────────────────┘                     │
│                                                                 │
│  Listens: UDP 443 (QUIC) + TCP 443 (TLS)                       │
└─────────────────────────────────────────────────────────────────┘
        ▲                    ▲                       ▲
        │ QUIC/WSS           │ HTTP/3 + HTTP/2       │ WSS
        │ (agent connections)│ (page loads, API)     │ (EventBus)
        │                    │                       │
   ┌────┴────┐          ┌───┴────┐              ┌───┴────┐
   │  Agent  │          │Browser │              │Browser │
   │(conduit)│          │ (web)  │              │ (dash) │
   └─────────┘          └────────┘              └────────┘
```

### 2.2 The Two Binaries

**`conduit-server`** — the master server. A single Go binary that contains:
- QUIC + WebSocket listeners for agent connections
- HTTP/3 + HTTP/2 web server for browsers
- The entire frontend (static Next.js export embedded via `embed.FS`)
- REST API for all operations
- WebSocket EventBus for real-time dashboard updates
- ACME TLS certificate management (Let's Encrypt)
- SQLite database with tenant-scoped query wrapper
- Setup wizard for first-run configuration
- Stripe billing integration

**`conduit`** — the agent and CLI. A single Go binary that serves two purposes:
- **Agent mode (daemon):** Runs as a background service on managed machines. Connects outbound to the master server via QUIC (primary) or WebSocket (fallback). Provides PTY shell access, file operations, system metrics, and command execution. Auto-updates itself when the master pushes a signed binary.
- **CLI mode:** A command-line management tool for operators. Authenticates via browser-based passkey flow, stores credentials encrypted per-profile. Provides terminal access, file transfer, bulk operations, and full infrastructure management from the command line. CLI-only mode on personal devices activates zero daemon processes, zero listeners, zero network interfaces. Supports both direct subcommands (`conduit shell myserver`) and an interactive TUI mode.

### 2.3 Interactive TUI (bubbletea)

When `conduit` is invoked with no arguments, it launches a full interactive terminal UI built with [bubbletea](https://github.com/charmbracelet/bubbletea). The TUI is a terminal-native equivalent of the web dashboard — not a stripped-down shortcut menu, but a real management interface for operators who live in the terminal.

**What the TUI provides:**

- **Server dashboard** — a live-updating list of all connected agents showing hostname, OS, IP, transport type (QUIC/WS), CPU, memory, disk, and connection status. Equivalent to the web dashboard's server list. Data updates in real-time via the same EventBus the browser uses.
- **Quick shell access** — navigate to any server and press Enter to drop directly into a shell session. Multiple sessions can be opened in tabs/splits.
- **File browser** — navigate the remote filesystem, upload, download, rename, delete, mkdir. Keyboard-driven, vim-style navigation.
- **Real-time event stream** — a live tail of events across the infrastructure: agent connections, shell sessions, file operations, auth events. The terminal version of the web EventBus.
- **Server detail view** — select a server to see full host info: CPU, memory, disk, network interfaces, running services, open ports, labels, connection history.
- **User and group management** — view and manage users, groups, and RBAC assignments.
- **Audit log viewer** — search and filter audit events with keyboard navigation.
- **Bulk exec** — select multiple servers (by label, group, or manual selection) and run commands across all of them with streaming output.

**Interaction model:**

- Keyboard-driven, vim-influenced navigation (j/k for movement, Enter to select, Esc to go back, / to search)
- `/` commands for quick actions within the TUI — same mental model as slash commands in chat applications:
  - `/shell <server>` — open a shell session
  - `/servers` — jump to server list
  - `/exec <label:value> <command>` — run a command across matched servers
  - `/files <server>` — open the file browser for a server
  - `/audit` — jump to audit log
  - `/users` — jump to user management
  - `/quit` — exit
- Tab or split-pane support for multiple simultaneous views (e.g., shell session in one pane, server list in another)
- Status bar showing connection state, authenticated user, active profile, and current tenant/sub-tenant context

**When arguments are provided**, the CLI skips the TUI and executes the command directly — standard CLI behavior. For example:
- `conduit shell myserver` — opens a shell immediately, no TUI
- `conduit files ls myserver:/var/log/` — lists files, prints output, exits
- `conduit exec --label env:prod -- uptime` — runs across matched servers, streams output, exits

The TUI is the "no arguments" default. The direct CLI is for scripting, automation, and quick one-off commands. Both use the same underlying API client and credential store.

### 2.4 Conduit Wire Protocol (CWP)

All agent-to-master communication uses the Conduit Wire Protocol. CWP is a binary framing protocol that is transport-agnostic — identical bytes over QUIC and WebSocket.

- On the QUIC path, each CWP stream maps to a native QUIC stream (no application-level multiplexer needed)
- On the WebSocket path, the CWP multiplexer handles multiple sessions over the single TCP connection using Stream IDs
- Frame types include: `HELLO`, `AUTH`, `SHELL_DATA`, `SHELL_RESIZE`, `FILE_OP`, `AGENT_INFO`, `EXEC`, and others
- The protocol supports multiple concurrent shell sessions, file transfers, and management operations over a single agent connection

### 2.5 Real-Time Dashboard

All dashboard state updates in real-time via the WebSocket EventBus. No polling. No manual refresh. When an agent connects, a shell session starts, a file upload completes, or any state changes — every connected browser sees it immediately.

---

## 3. Security Model

### 3.1 Authentication

**Passkeys are the sole production authentication mechanism.** There are no passwords, no SSH keys, no API keys that a human types. The only exceptions:

- **Setup wizard:** A temporary password is used exactly once during first-run setup to bootstrap the first passkey. It is permanently deleted from the database the moment the passkey is registered.
- **Dev mode:** Password auth is available for local development only.
- **CI tokens:** Machine-to-machine tokens for automation (`CONDUIT_TOKEN` env var), scoped and revocable.
- **Agent tokens:** HMAC-SHA256 tokens presented by agents during connection handshake.

**WebAuthn flow:** The server is a WebAuthn Relying Party. Users register passkeys (platform authenticators like Touch ID / Windows Hello, or roaming authenticators like YubiKeys). Login is a WebAuthn assertion. The server never sees or stores a password.

**CLI authentication:** The CLI opens the user's browser to the Conduit instance, the user authenticates with their passkey, and a scoped CLI token is returned to the CLI via a device flow. The CLI stores this token encrypted on disk, per-profile.

**Session visibility:** All authenticated sessions — web, iOS, CLI, CI — are visible and individually revocable from the dashboard.

### 3.2 Cryptographic Policy

**If Conduit controls both ends, PQC is mandatory. If an external party is involved, classical is available with a visible warning and audit log entry. No silent downgrades. Ever.**

**PQC mandatory (both ends are Conduit code):**

| Purpose | Algorithm |
|---|---|
| TLS key exchange | X25519MLKEM768 hybrid (Go 1.24 stdlib) |
| JWT signing | Ed25519 Phase 1, ML-DSA-65 when Go stdlib ships it |
| Binary signing | SLH-DSA-SHA2-256s |
| Agent token HMAC | HMAC-SHA256 |
| DB field encryption | AES-256-GCM |
| Key derivation | HKDF-SHA256 |

No third-party crypto libraries for JWTs. Do NOT use `circl` or similar. Wait for Go stdlib.

**Classical available with warning (external party involved):**

| Context | What is accepted | Why |
|---|---|---|
| WebAuthn | ECDSA P-256, Ed25519, RSA | Hardware authenticator chooses the algorithm |
| PIV / Smart cards | RSA-2048, ECDSA P-256 | Government-issued cards use what they use |
| Legacy SSH | Whatever the remote SSH server supports | Can't control the other end |
| PKI cert issuance | RSA, ECDSA as explicit opt-in | Must support external clients, but with visible "Legacy — quantum-vulnerable" badge and audit log |
| ACME / Let's Encrypt | ECDSA P-256 | Let's Encrypt doesn't issue PQC certs yet |
| Self-signed fallback | ECDSA P-256 | Browsers don't support PQC TLS certs yet |

**No silent downgrades:** The system never auto-selects a weaker algorithm. If a user or device presents RSA or standalone ECDSA, it is accepted but logged. Classical options are visible in the UI but never the default — PQC or the strongest available option is always pre-selected.

**Absolutely forbidden (broken algorithms, no exceptions):**
- MD5 (anywhere)
- SHA-1 (anywhere)
- DES, 3DES
- RC4
- TLS 1.0, 1.1, 1.2
- Non-CSPRNG sources for secrets

### 3.3 Transport Security

TLS 1.3 is mandatory on all paths. TLS 1.2 is disabled. This is a hard requirement.

**Cipher suites (TLS 1.3, NIST-approved):**
- `TLS_AES_256_GCM_SHA384`
- `TLS_AES_128_GCM_SHA256`
- `TLS_CHACHA20_POLY1305_SHA256`

### 3.4 Audit Logging

Every access event is logged: logins, session starts, shell commands, file operations, agent connections, permission changes, configuration changes. Audit logs are tenant-scoped and immutable. Every use of a classical algorithm where PQC was available is logged with the reason.

### 3.5 NIST Security Standards

Conduit applies NIST standards as the security foundation — not as an afterthought bolted on before an audit, but as the design constraints that shape every decision from the first line of code.

#### NIST SP 800-131A Rev. 2 — Cryptographic Transitions

This standard defines which algorithms are acceptable, restricted, or disallowed. Conduit's entire cryptographic policy (§3.2) is derived from it:

- Every algorithm Conduit selects for internal use (where both ends are Conduit code) meets or exceeds 800-131A requirements. X25519MLKEM768 hybrid key exchange, Ed25519/ML-DSA-65 signing, AES-256-GCM encryption, HMAC-SHA256, and HKDF-SHA256 are all compliant.
- Algorithms that 800-131A marks as "disallowed" (MD5, SHA-1, DES, 3DES, RC4) are forbidden in Conduit unconditionally — there is no override, no flag, no escape hatch.
- Where 800-131A marks algorithms as "acceptable" but quantum-vulnerable (RSA, ECDSA without PQC hybrid), Conduit allows them only for external-party interactions and logs every use with a warning badge. This positions Conduit ahead of the standard's current requirements while remaining interoperable.
- TLS 1.0 and 1.1 are disallowed by 800-131A. Conduit goes further and also disallows TLS 1.2 — only TLS 1.3 is permitted.

#### NIST SP 800-53 Rev. 5 — Security and Privacy Controls

This is the comprehensive catalog of security controls used by federal systems and increasingly adopted by private sector organizations pursuing SOC 2, FedRAMP, and ISO 27001. Conduit implements applicable controls as code, not as policy documents:

**Access Control (AC)**
- AC-2: Account management — user lifecycle managed through dashboard, all accounts tied to passkeys, no shared credentials, session visibility and revocation
- AC-3: Access enforcement — RBAC with tenant-scoped permissions, enforced at the API layer on every request
- AC-6: Least privilege — roles grant minimum necessary access, platform_owner/org_owner/org_admin/org_member hierarchy
- AC-7: Unsuccessful login attempts — WebAuthn failures are logged and rate-limited
- AC-11: Session lock — idle session timeout, configurable per tenant
- AC-12: Session termination — all sessions revocable from dashboard, automatic expiry on JWT tokens
- AC-17: Remote access — the entire product is remote access, secured by passkeys + PQC TLS + audit logging

**Audit and Accountability (AU)**
- AU-2: Auditable events — every access event is logged (logins, shell sessions, file ops, agent connections, config changes, permission changes)
- AU-3: Content of audit records — each log entry includes who, what, when, where (source IP, tenant, agent), and outcome
- AU-6: Audit review — audit logs are queryable in the dashboard with filters and search
- AU-9: Protection of audit information — audit logs are tenant-scoped, append-only, and cannot be modified or deleted by tenant users
- AU-10: Non-repudiation — passkey authentication provides cryptographic proof of identity; agent tokens provide proof of machine identity
- AU-12: Audit generation — audit logging is automatic and cannot be disabled by tenants

**Configuration Management (CM)**
- CM-2: Baseline configuration — server configuration is declarative (`server.yaml`), agent configuration is managed centrally from the master
- CM-3: Configuration change control — all configuration changes are audit-logged
- CM-6: Configuration settings — security-relevant defaults are hardened (TLS 1.3 only, PQC preferred, no password auth)
- CM-7: Least functionality — agents expose only the capabilities needed (PTY, file ops, metrics), no unnecessary services

**Identification and Authentication (IA)**
- IA-2: Identification and authentication — all users authenticate via WebAuthn passkeys, all agents authenticate via HMAC tokens
- IA-5: Authenticator management — passkeys are hardware-bound or platform-bound, no server-side secrets to leak; agent tokens are HMAC-SHA256
- IA-8: Identification and authentication (non-organizational users) — sub-tenant users authenticate through the same WebAuthn flow with tenant isolation
- IA-12: Identity proofing — first user is proofed via physical access to the server (setup wizard on localhost only)

**System and Communications Protection (SC)**
- SC-8: Transmission confidentiality and integrity — all data in transit is encrypted with TLS 1.3, PQC hybrid key exchange
- SC-12: Cryptographic key establishment and management — keys are generated from `crypto/rand` (CSPRNG), key exchange uses X25519MLKEM768 hybrid
- SC-13: Cryptographic protection — all algorithms are NIST-approved, compliant with 800-131A Rev. 2 and FIPS 203/204/205
- SC-23: Session authenticity — JWTs are signed with Ed25519, sessions are bound to authenticated identities
- SC-28: Protection of information at rest — sensitive database fields encrypted with AES-256-GCM, key derivation via HKDF-SHA256

**System and Information Integrity (SI)**
- SI-2: Flaw remediation — agent auto-update delivers signed binaries from the master
- SI-4: System monitoring — real-time agent metrics (CPU, memory, disk, network), connection status, EventBus for live updates
- SI-7: Software, firmware, and information integrity — binary signing with SLH-DSA-SHA2-256s, agents verify signature before applying updates
- SI-10: Information input validation — unknown JSON fields rejected on all API endpoints, parameterized SQL only

#### FIPS 203, 204, 205 — Post-Quantum Cryptography Standards

These are the finalized NIST post-quantum standards (August 2024). Conduit adopts them where Go stdlib support exists:

- **FIPS 203 (ML-KEM):** X25519MLKEM768 hybrid key exchange is used for all TLS connections. Go 1.24 ships this in the standard library. This protects all data in transit against harvest-now-decrypt-later attacks by quantum computers.
- **FIPS 204 (ML-DSA):** ML-DSA-65 will replace Ed25519 for JWT signing when Go stdlib ships it. Until then, Ed25519 is used. No third-party crypto libraries are used as a stopgap — we wait for stdlib.
- **FIPS 205 (SLH-DSA):** SLH-DSA-SHA2-256s is used for binary signing (agent updates, release artifacts). This ensures that even a quantum-capable adversary cannot forge a signed binary.

#### NIST SP 800-63B — Digital Identity Guidelines (Authentication)

Conduit's passkey-only authentication aligns with AAL3 (the highest assurance level):

- Authentication is based on cryptographic proof of possession of a hardware-bound key (passkey)
- No passwords, no knowledge-based factors, no phishable credentials
- WebAuthn assertion provides verifier impersonation resistance (the browser verifies the origin)
- Replay resistance is built into the WebAuthn protocol (challenge-response with server-generated nonce)
- The setup wizard's temporary password is a deliberate, time-limited exception that is destroyed after first use

#### NIST SP 800-57 — Key Management

- Key generation uses CSPRNG (`crypto/rand`) exclusively
- Key usage is purpose-bound (TLS keys for transport, signing keys for JWTs, encryption keys for database fields — never reused across purposes)
- Key rotation is automated for TLS (ACME renewal) and JWTs (configurable rotation period)
- Agent tokens use HMAC-SHA256 with per-agent keys
- Conduit Vault (Phase 3+) will provide full key lifecycle management with Shamir Secret Sharing and TPM-sealed delivery

#### How This Is Enforced

NIST compliance is not a checklist reviewed before release. It is enforced through code:

- **Linting:** `fmt.Sprintf` in SQL is a build failure. Raw queries bypassing the tenant wrapper are a lint failure.
- **Configuration:** TLS 1.2 is disabled in code, not by configuration. There is no flag to re-enable it.
- **Defaults:** PQC algorithms are always pre-selected in the UI. Classical options exist but are never the default.
- **Middleware:** Every API request passes through auth middleware (JWT validation), tenant middleware (scope enforcement), and plan middleware (feature/limit checks).
- **Audit:** Every authentication event, every algorithm selection, every access decision is logged. The audit log itself is append-only and tenant-isolated.
- **Testing:** Security-relevant behavior (algorithm selection, tenant isolation, input validation, auth flows) is covered by automated tests that run on every build.

---

## 4. Transport

### 4.1 Global Policy

**QUIC first, WebSocket/HTTP fallback. Everywhere. Phase 1. Day one.**

The master server listens on both UDP 443 (QUIC) and TCP 443 (TLS) from first boot. Every connection type uses QUIC primary with automatic fallback.

| Connection | QUIC path | Fallback path |
|---|---|---|
| Agent to master | Raw QUIC streams (UDP 443) | WSS over TCP 443 |
| Browser real-time (EventBus) | WebSocket over HTTP/3 (QUIC) | WebSocket over HTTP/2 (TCP) |
| Browser web/API (page loads) | HTTP/3 (QUIC) | HTTP/2 (TCP) |

### 4.2 Agent Connections

- Agent attempts QUIC first using `quic-go`
- If QUIC fails (~5 second timeout, typically UDP blocked by firewall), agent falls back to WebSocket (TCP 443) automatically
- 0-RTT reconnection for agents on flaky networks
- Native QUIC stream multiplexing without head-of-line blocking
- The agent logs which transport it connected with
- The dashboard shows transport type per agent

**Agent connection endpoints:**
```
QUIC:  quic://[master-domain]:443              (preferred)
WSS:   wss://[master-domain]/agent/v1/connect  (fallback)
```

The agent presents its token in the connection handshake. For WebSocket, this is the `Authorization: Bearer <token>` header during upgrade. For QUIC, this is the first CWP `HELLO` frame after handshake.

### 4.3 Browser Connections

HTTP/3 and HTTP/2 are negotiated transparently by the browser via `Alt-Svc` header. No application code needed for fallback. WebSocket connections ride over HTTP/3 when available, fall back to HTTP/2 over TCP automatically.

### 4.4 Terminology

Agent connections are **never** called "tunnels." They are "agent connections" or "persistent streams." The word "tunnel" refers exclusively to the AppSynergy QUIC Tunnel Service — a separate VPN product sold on the platform. These are completely unrelated to how agents communicate with the master server.

---

## 5. Data Model

### 5.1 Multi-Tenancy

Three-level hierarchy from the first line of code. No deeper.

1. **Platform** — AppSynergy (the operator)
2. **Tenant** — a paying customer, owns a Stripe subscription and a resource pool
3. **Sub-tenant** — created by a tenant to manage divisions or their own clients

Every data table has `tenant_id TEXT NOT NULL DEFAULT 'default'`. Every database query goes through the tenant-scoped query wrapper which injects `WHERE tenant_id = ?` automatically. Raw queries bypassing the wrapper are a lint failure.

**Self-hosted mode:** One tenant, `tenant_id = "default"`. Multi-tenancy is invisible.
**SaaS mode:** Many tenants, each isolated. Enabled by config flag: `server.mode: saas`. Launch-ready from day one.

### 5.2 Sub-Tenant Model

Tenants create sub-tenants and distribute their purchased seats, agents, and features across them. Sub-tenants are isolated from each other. Sub-tenants never see parent or sibling data.

**Visibility modes (set per sub-tenant, changeable by parent):**

| Mode | Parent sees | Use case |
|---|---|---|
| `full` | All resources, sessions, servers in sub-tenant | Internal division — parent manages everything |
| `aggregate` | Usage numbers, quota consumption, agent count only | Semi-managed client — parent monitors but doesn't operate |
| `opaque` | Only existence and total quota usage | Independent client, parent is a reseller |

The `tenants` table has `parent_tenant_id` (NULL for top-level) and `visibility` columns.

**Resource allocation:** Parents distribute their plan's resources across sub-tenants via the `sub_tenant_allocations` table. The sum of all sub-tenant allocations cannot exceed the parent's plan limits. Enforced at the API layer.

### 5.3 Plan Builder & Feature Gating

**Plans are NOT hardcoded.** No default "Free/Starter/Pro/Enterprise" plans in the code. The platform admin creates every plan from scratch in the dashboard.

On first run, the system creates a "Personal Unlimited" plan (all limits NULL/unlimited, all features enabled) assigned to the platform owner. Not shown on any pricing page.

**Plan builder UI allows the admin to:**
- Name the plan and set a URL slug
- Toggle each feature on/off (dynamic checkboxes driven by the feature registry)
- Set numeric limits per dimension (seats, agents, groups, concurrent sessions, storage quota) with "unlimited" toggle
- Link to a Stripe price (or leave unlinked for internal/free plans)
- Mark as public (appears on pricing page) or internal

**Feature registry:** The `plan_feature_registry` table defines what features exist. When a new feature ships, a migration inserts a row and it automatically appears as a new toggle in the plan builder UI. No code changes to the plan builder required.

**Enforcement middleware** checks both feature access and numeric limits on every relevant API endpoint. Stripe products and prices are created dynamically when the admin creates a plan. Tenants can distribute their plan's resources across sub-tenants but cannot exceed the parent plan's limits.

---

## 6. Frontend

**Static export embedded in Go. No SSR. No Node.js in production.**

- Next.js 15 App Router compiles to static output (`next build` → `web/out/`)
- `web/out/` is embedded into `conduit-server` via Go `embed.FS`
- Go serves all static assets — there is no Node.js process in production
- Marketing/promotional pages are pre-rendered at build time (good SEO for fixed routes)
- Dashboard is a client-side SPA behind auth — fetches all data from the Go API
- Pricing page fetches Stripe data client-side, not server-side
- OG images are pre-generated at build time as static PNGs in `public/og/`
- White-label branding loaded via client-side fetch from the Go API, cached aggressively
- All frontend assets (JS, CSS, fonts, icons, WASM) bundled at build time — zero runtime CDN dependencies
- Local font files in `public/fonts/` — no `next/font/google` runtime fetches
- Zero browser-native UI primitives (alert, confirm, prompt, tooltip) — all UI with shadcn/ui components
- Fortune 500-quality UI/UX throughout dashboard and promotional site
- Core Web Vitals targets: LCP <=2.5s, INP <=200ms, CLS <=0.1
- Dashboard designed for open-ended expansion — new products and services can be added without architectural teardown
- Promotional site optimized to rank for remote access, IP tunneling, and infrastructure access keywords

**Forbidden in the frontend:**
- React Server Components (RSC)
- Server-side rendering (SSR) at request time
- `@vercel/og` server-side OG image generation
- Dynamic server-side white-label rendering
- Any reference to "SSR for marketing site, RSC for dashboard shell"
- `next/font/google` runtime fetches
- Any Next.js API routes that proxy to the Go backend (the Go backend IS the API)

### 6.1 shadcn/ui — Component System

All UI components are built with shadcn/ui. This is **not** a traditional npm dependency — it is a code distribution system that copies component source code directly into the project. You own every file. There is no `shadcn-ui` in `package.json`.

**How it works — three layers:**
1. **Radix UI Primitives** — unstyled, accessible headless components that handle behavior, ARIA attributes, keyboard navigation, and state management. These *are* real npm dependencies (`@radix-ui/*` packages) installed when you add a component.
2. **Tailwind CSS** — all styling is done via Tailwind utility classes and CSS custom properties for theming.
3. **Your code** — the shadcn/ui source files placed in `components/ui/`, which compose Radix primitives with Tailwind styling. You modify these directly.

**Setup:**
```bash
npx shadcn@latest init        # Initialize in the project (creates components.json)
npx shadcn@latest add button   # Add individual components as needed
npx shadcn@latest add dialog
```

**`components.json` configuration (critical settings):**

| Field | Value | Why |
|---|---|---|
| `style` | `"new-york"` | The default style is deprecated |
| `rsc` | `true` | Ensures `"use client"` directives are added where needed — required even with static export because Next.js App Router treats files as server components by default |
| `tsx` | `true` | TypeScript |
| `tailwind.config` | `""` (empty) | Tailwind v4 uses CSS-first config, no `tailwind.config.js` |
| `tailwind.css` | Path to global CSS | Where Tailwind directives live |
| `tailwind.cssVariables` | `true` | Use CSS custom properties for theming |
| `aliases.ui` | `@/components/ui` | Where component files are placed |
| `aliases.utils` | `@/lib/utils` | Where `cn()` utility lives |
| `aliases.hooks` | `@/hooks` | Where hooks are placed |

**shadcn/skills (AI accuracy):**

shadcn/ui ships a skills file that dramatically improves AI coding accuracy. Install it:
```bash
pnpm dlx skills add shadcn/ui
```
This generates `.shadcn/skills.md` with machine-readable documentation for AI assistants. Without it, AI models have a ~34% API error rate on shadcn components. With it, errors drop to ~3%.

**Component variants use CVA (Class Variance Authority).** All visual variants (size, color, state) must be defined using `cva()` functions — never scattered hardcoded className strings. This keeps theming in a single source of truth.

#### Common AI Mistakes with shadcn/ui — Do NOT Make These

These are the documented, real-world failure patterns that AI coding assistants fall into with shadcn/ui. This project explicitly forbids all of them:

**1. Hallucinating props and APIs.** AI models mix up APIs from MUI, Radix, React Select, and outdated shadcn patterns. Example: generating `<Select onValueChanged={...}>` when the actual prop is `onValueChange`. Always verify props against the actual component source in `components/ui/`, not from training data.

**2. Missing required sub-components.** Components like Dialog, Sheet, and AlertDialog have mandatory composition rules enforced by Radix:
- `DialogContent` MUST contain a `DialogTitle` (inside `DialogHeader`) and a `DialogDescription`
- `SheetContent` MUST contain a `SheetTitle` and `SheetDescription`
- `AlertDialogContent` MUST contain `AlertDialogTitle` and `AlertDialogDescription`
- Omitting these produces accessibility warnings and broken behavior. NEVER skip them. If a visual title is not wanted, use `<DialogTitle className="sr-only">Accessible title</DialogTitle>`.

**3. Ignoring CVA variants.** Do NOT hardcode Tailwind classes directly on components when a variant should be used. Use the component's defined variants via props (e.g., `<Button variant="destructive" size="sm">`) instead of overriding with raw className strings.

**4. Breaking responsive design.** Always use mobile-first Tailwind prefixes (`sm:`, `md:`, `lg:`). Use `w-full` and flex/grid layouts — never fixed pixel widths that break on mobile. The dashboard must be usable on mobile.

**5. Missing ARIA labels.** Every icon-only button MUST have either an `aria-label` prop or a `<span className="sr-only">` child. Every interactive element must be keyboard navigable (Radix handles this if you don't break it).

**6. Wrong Tailwind version syntax.** This project uses Tailwind v4:
- No `tailwind.config.js` — all config is CSS-first
- Use `@import "tw-animate-css"` — `tailwindcss-animate` is deprecated
- Colors use OKLCH, not HSL
- Use `@theme inline` directive for CSS variable definitions

**7. Installing components that aren't needed.** Each component adds Radix dependencies and increases the bundle size, which increases the Go binary size. Add components one at a time as they are needed. Never bulk-install the entire library.

**8. Treating shadcn/ui as a dependency.** Never try to `npm install shadcn-ui`. It is not a package. The CLI copies source code into your project. You own it and edit it directly.

**9. Using `<Image>` without configuring it for static export.** Next.js image optimization requires a server. With `output: 'export'`, you must either use a custom loader or set `unoptimized: true` in `next.config.js`.

**10. Forgetting that portalled components work differently in tests.** Dialog, Popover, Sheet, and DropdownMenu render via Radix portals (injected at `document.body`). Radix also sets `pointer-events: none` on `document.body` during interactions. JSDOM-based tests (Jest, Vitest) struggle with this — use Playwright for testing these components.

#### Static Export Compatibility Notes

Since Conduit uses `output: 'export'` (static build embedded in Go):

- `"use client"` directives are harmless and required — they mark components for client-side hydration, which is how static export works
- Portal-based components (Dialog, Popover, Sheet) work fine — portals are purely client-side behavior
- No `cookies()`, `headers()`, or `searchParams` in server components — this is a Next.js constraint, not shadcn-specific
- No dynamic routes without `generateStaticParams` — all routes must be enumerable at build time
- The Go binary's size is directly affected by the frontend bundle — only add components you use

---

## 7. Setup Wizard (First Run)

1. **Server starts on `localhost:8080` (HTTP only).** No TLS, no external access. Setup wizard is localhost-only.
2. **Wizard collects:** domain name, admin email, organization name, first user (email + display name), temporary password.
3. **Server configures itself:** writes `server.yaml`, obtains ACME TLS cert, creates `platform` and `default` tenants, creates first user with `platform_owner` role, stores temp password hash (Argon2id `m=64MB, t=3, p=4`), restarts on HTTPS port 443.
4. **User redirected to real domain.** Logs in with temporary password.
5. **Forced passkey registration screen.** No other action possible until a passkey is registered.
6. **Passkey registered.** Temporary password is `DELETE`d from database (not set to NULL). Dev-mode password auth disabled. Dashboard fully accessible.
7. **`GET /setup` returns 404 forever.** Setup mode flag stored in database, not config file — cannot be re-triggered by deleting config.

**Security:** The temporary password exists for the minimum time between initial login and passkey registration. If the user closes the browser before registering, the next login re-presents the forced registration screen. After registration, the password hash row is deleted — there is no residual credential.

---

## 8. Naming — Mandatory Terminology

| Term | Means | Does NOT mean |
|---|---|---|
| Agent | The background daemon mode of the `conduit` binary | An AI agent |
| Agent connection / persistent stream | How the agent talks to the master server | A tunnel |
| Tunnel | The AppSynergy QUIC Tunnel Service — a separate paid VPN product (Phase 5) | Agent connections |
| Vault | Conduit's built-in secrets/key management (Phase 3+) | HashiCorp Vault |
| Single binary | Two separate things: `conduit-server` is one, `conduit` is another | One merged binary |

---

## 9. Code Standards

- Go 1.24+ minimum (required for X25519MLKEM768 TLS)
- All database queries through tenant-scoped query wrapper — no raw SQL without `tenant_id` filter
- All IDs: UUID v4 from `crypto/rand`
- No CGO for the client binary
- No external CDN dependencies in frontend — everything bundled via npm at build time
- Parameterized queries only — `fmt.Sprintf` in SQL is a build failure, linting enforced
- Unknown JSON fields rejected on all API endpoints (no silent ignore)
- Schema migrations are additive-only forward migrations, embedded in the binary, applied automatically at startup
- All storage interfaces are abstract — SQLite (monolith) and rqlite (cluster) are drop-in substitutes
- OpenAPI 3.1 spec for all REST endpoints
- Hybrid database architecture: SQLite for relational data, VictoriaMetrics/SQLite ring-buffer for time-series metrics
- Server binary is monolith-first but cluster-ready — single binary runs alone or joins a cluster via a join token

---

## 10. Prioritized Feature Checklist

Work top-to-bottom. Check off each item as it is completed. Related features are grouped so UI, backend, and CLI for each area are built together. The architecture must never block features lower on the list — design for everything, build in priority order.

### Server Core
- [ ] QUIC + WebSocket dual listeners (UDP 443 + TCP 443)
- [ ] HTTP/3 serving for browsers + HTTP/2 fallback
- [ ] ACME TLS auto-provisioning (Let's Encrypt)
- [ ] X25519MLKEM768 hybrid PQC TLS on all connections
- [ ] SQLite database with tenant-scoped query wrapper
- [ ] Embedded static frontend via `embed.FS`
- [ ] First-run setup wizard (localhost:8080 → ACME → HTTPS → forced passkey)
- [ ] WebSocket EventBus for real-time dashboard updates
- [ ] OpenAPI 3.1 spec generation
- [ ] `server.yaml` configuration file
- [ ] Server binary cluster-ready (single binary or join via token)

### Auth & Identity
- [ ] WebAuthn passkey registration + login
- [ ] JWT issuance and validation (Ed25519)
- [ ] Users + groups + RBAC
- [ ] CLI browser device flow (passkey → CLI token)
- [ ] CLI credential storage (encrypted, per-profile)
- [ ] All authenticated sessions (web, CLI, CI) visible and revocable from dashboard
- [ ] SAML/OIDC SSO (passkeys remain primary)
- [ ] PIV / smart card authentication
- [ ] Platform-level JWT (multi-product `products` claim)

### Agent & Connections
- [ ] Agent outbound QUIC connection (primary)
- [ ] Agent automatic WebSocket fallback (if QUIC/UDP blocked)
- [ ] CWP wire protocol — identical framing over QUIC and WebSocket
- [ ] Agent registration flow (single-use + persistent join tokens)
- [ ] Agent heartbeat and connection status in dashboard
- [ ] Dashboard shows transport type per agent (QUIC vs WebSocket)
- [ ] Linux amd64 + arm64 agent builds
- [ ] Windows amd64 agent build
- [ ] macOS arm64 agent build + launchd service
- [ ] Agent auto-update (signed binary push from master)
- [ ] Full host visibility per agent: CPU, memory, disk, network, services, ports
- [ ] Agent metrics dashboard (CPU, RAM, disk — `AGENT_INFO` frames)
- [ ] Universal resource labelling system for surgical targeting

### Multi-Tenancy & Billing
- [ ] Multi-tenant data model (`tenant_id` on every table, three-level hierarchy)
- [ ] Self-hosted mode (single tenant, `tenant_id = "default"`)
- [ ] SaaS mode (config flag `server.mode: saas`)
- [ ] Tenant signup flow
- [ ] Subdomain routing per tenant
- [ ] Sub-tenant creation with visibility modes (full/aggregate/opaque)
- [ ] Sub-tenant resource allocation and enforcement
- [ ] Plan builder: admin creates plans from scratch, all features gatable, all limits configurable
- [ ] Plan feature registry (dynamic toggles, migration-driven)
- [ ] Plan enforcement middleware (feature access + numeric limits)
- [ ] Stripe billing integration (products/prices created dynamically per plan)
- [ ] Per-tenant object storage bucket (files, scripts, binaries, build artifacts)

### Shell & Terminal
- [ ] Shell sessions in browser (xterm.js, full feature parity)
- [ ] PTY shell execution on agent
- [ ] Multiple concurrent shell sessions per agent (multiplexed via CWP)
- [ ] Terminal session recording + playback (asciicast v2)
- [ ] Infrastructure visibility dashboards designed for TV/projector display
- [ ] Mission Control mega TV dashboard

### File Management
- [ ] Browser-based file manager (list, download, upload, delete, rename, mkdir, preview)
- [ ] File transfer over HTTPS/QUIC — no SFTP protocol dependency
- [ ] Resumable file uploads

### CLI
- [ ] CLI binary (`conduit`) — auth, server list, shell, file transfer
- [ ] CLI runs as full TUI (bubbletea) when no arguments given
- [ ] CLI-only mode on personal devices (zero daemons, zero listeners)
- [ ] CLI exec commands — full bulk exec from terminal
- [ ] CLI group, user, audit management commands
- [ ] CLI serial connect command
- [ ] CLI k8s commands
- [ ] CLI lights-out basic (reboot/poweroff via agent)
- [ ] CLI lights-out BMC/IPMI support
- [ ] CLI local DHCP + iPXE server (`conduit pxe serve`)
- [ ] Complete certificate lifecycle in CLI
- [ ] Shell completions (bash, zsh, fish, PowerShell)
- [ ] CI token support (`CONDUIT_TOKEN` env var, scoped tokens)

### Audit & Compliance
- [ ] Audit logging for all access events
- [ ] Webhooks on audit events
- [ ] Security audit readiness (SOC 2 Type II, penetration test ready)
- [ ] NIST SP 800-53 Rev. 5 control coverage
- [ ] NIST SP 800-131A cryptographic compliance

### Promotional Website
- [ ] Public marketing landing page served from same binary
- [ ] Fortune 500-quality UI/UX
- [ ] SEO optimized for remote access, IP tunneling, infrastructure access keywords
- [ ] Pre-generated OG images (build-time, static assets in `public/og/`)
- [ ] Core Web Vitals targets met (LCP <=2.5s, INP <=200ms, CLS <=0.1)

### Bulk Operations
- [ ] Bulk command execution — multi-server parallel script runner
- [ ] Binary deployment service

### Serial & Hardware
- [ ] Serial console connections (USB-to-serial, direct serial)
- [ ] Automated OS installation over serial interface

### Legacy SSH
- [ ] Legacy SSH connections (stored credentials, host key verification)
- [ ] SFTP over legacy SSH connections

### Orchestration
- [ ] Kubernetes / k3s orchestration UI

### OS Provisioning
- [ ] iPXE network boot provisioning (configure in dashboard, PXE boot, auto-enroll)
- [ ] Full OS profile builder

### Vault (Secrets & Key Management)
- [ ] Conduit Vault — secrets and key management with Shamir Secret Sharing
- [ ] TPM-sealed per-node secret delivery

### PKI & Certificates
- [ ] Full PKI management: root CAs, intermediate CAs, server/client/user/code-signing certs with guided wizard
- [ ] Advanced PKI features — CRL distribution, OCSP responder

### Mobile
- [ ] iOS app (native passkey + WKWebView terminal + native file manager)

### Cluster & Federation
- [ ] rqlite cluster mode — monolith to distributed
- [ ] Multi-master federation
- [ ] Encrypted SQLite (SQLCipher AES-256-GCM)
- [ ] Anycast and embedded DNS management
- [ ] Terraform provider

### AppSynergy QUIC Tunnel Service (Separate VPN Product)
- [ ] `appsynergy.io` migrated to Conduit platform stack
- [ ] Existing WireGuard customers linked to platform (Stripe migration)
- [ ] `appsynergy-gateway` binary deployed on AppSynergy infrastructure
- [ ] `appsynergy-tunnel` client binary (Linux, Windows, macOS)
- [ ] /24 IPv4 block management in platform database
- [ ] Tunnels product UI in dashboard shell
- [ ] WireGuard migration wizard
- [ ] Bandwidth metering + usage-based Stripe billing
- [ ] AppSynergy iOS app update (WireGuard → QUIC tunnel client)
- [ ] WireGuard service maintained in parallel per sunset timeline

### Network & Routing
- [ ] BGP peer management (GoBGP integration, dashboard, CLI)
- [ ] OSPF, static routes, routed IP allocation
- [ ] Real-time network topology visualization with live packet flow animations
- [ ] Per-tunnel metered bandwidth billing via Stripe usage-based pricing
- [ ] Out-of-band management (Redfish, IPMI, Intel AMT/ME, AMD DASH, iDRAC, iLO)

### Log Management
- [ ] Log collection agent subsystem (journald, syslog, Event Log ingestion)
- [ ] Log ingestion pipeline (parsing, indexing, FTS5 search)
- [ ] Log search UI and live tail
- [ ] Log alerting engine
- [ ] Log export + forwarding (S3, syslog, Splunk HEC)

### Edge, OS & Signage
- [ ] Conduit OS — minimal, immutable Linux distribution with Conduit agent as primary management interface
- [ ] Digital signage and edge device management

### Endpoint Security & CI/CD
- [ ] Endpoint virus scanning and security metrics
- [ ] CI/CD pipeline engine
- [ ] Container registry (OCI-compliant)
- [ ] Package repositories (npm, apt, rpm, helm, go module proxy)
- [ ] AI infrastructure management with hard human-approval gate before any destructive action
