# Project Directives

## Document Precedence
- `CLAUDE.md` is the single source of truth — security standards, product spec, coding rules
- `openapi.yaml` is the API contract (endpoint shapes, schemas, validation)
- `README.md` is for humans only — project overview, build instructions, success criteria

---

## Compliance Is Non-Negotiable
Every function, handler, query, and component must comply with the NIST and OWASP
enforcement rules below. Security is never traded for speed. No exceptions. No shortcuts.
Check every piece of code against both enforcement rule sets before considering it complete.

---

## Security Standards (MANDATORY)

### Project Security Overrides
- **TLS 1.3 ONLY** — overrides global SP 800-52 (TLS 1.2 minimum)
- **JWT signing: Ed25519** — overrides global SP 800-175B (RS256/ES256)
- **PQC-first** — classical crypto only when external party forces it

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

### NIST SP 800-228 — API Security Controls

Every API endpoint must implement REC-API-1 through REC-API-26:
- **Pre-Runtime (1-8):** Documented spec, OpenAPI definitions, schema validation, API inventory, parameter validation, sensitivity annotations, PII annotations, runtime metadata
- **Runtime Basic (9-16):** TLS everywhere, bot/DoS mitigation, auth with rate limiting/lockout/MFA, authz on every request, syntactic validation, input length limits, per-user rate limiting, circuit breakers
- **Runtime Advanced (17-26):** Fine-grained blocking, telemetry/monitoring, field-level validation, authz via annotations, detailed telemetry, payload scanning, info-disclosure-safe errors, enumeration mitigation, data masking, DoS-targeted blocking
- **Zero-Trust (SP 800-207A):** Encrypt all traffic, identity-based segmentation, continuous verification at every boundary, perimeter at service instance level

### NIST SP 800-63B — Digital Identity

AAL2 minimum for all endpoints. Phishing-resistant MFA (FIDO2 passkeys). Risk-based DIRM. Session management + credential lifecycle required.

### OWASP Top 10 (2021)

| ID | Risk | Mandatory Controls |
|---|---|---|
| **A01** | Broken Access Control | Deny by default. Server-side RBAC on every handler. Verify resource ownership on every DB query (`WHERE id = ? AND user_id = ?` or role check). Never trust client-supplied IDs. Short-lived JWTs. Functional access control tests for every role × endpoint. |
| **A02** | Cryptographic Failures | TLS 1.3 only. AES-256-GCM at rest. Argon2id for passwords. Ed25519 for JWTs. `crypto/rand` for all randomness (never `math/rand`). `Cache-Control: no-store` on sensitive responses. Keys in config/env, never source code. |
| **A03** | Injection | Parameterized SQL only (never `fmt.Sprintf` for SQL). `oapi-codegen` validates request schemas before handlers. No `os/exec` with user input. No `dangerouslySetInnerHTML` with user input. Server-side positive validation (allowlists). |
| **A04** | Insecure Design | Threat model (STRIDE) every feature. Business logic rate limits per operation. `tenant_id` on every table. Plausibility checks at every tier. Tests validate critical flows against threat model. |
| **A05** | Security Misconfiguration | No debug mode in production. Security headers on every response (CSP, HSTS, X-Content-Type-Options, X-Frame-Options, Referrer-Policy, Permissions-Policy). Custom error pages via RFC 9457. Restrict HTTP methods per route. CORS explicit allowlist only. |
| **A06** | Vulnerable & Outdated Components | `go.sum` + `pnpm-lock.yaml` for integrity. `govulncheck` + `npm audit` in CI. SBOM per release. Only approved dependencies per CLAUDE.md. |
| **A07** | Identification & Authentication Failures | WebAuthn/passkeys primary (AAL2 min). Anti-brute-force: max 100 failed/hour. Argon2id hashing. Session tokens: 64+ bits entropy, regenerated on auth state change. Identical error messages for login/registration (no enumeration). |
| **A08** | Software & Data Integrity Failures | All assets embedded (`embed.FS`), zero CDN. Signed releases. CI/CD branch protection + required reviews. JSON-only deserialization, validated against schemas. No untrusted deserialization into executable structures. |
| **A09** | Security Logging & Monitoring Failures | `slog` structured JSON. Log all auth events, authz decisions, data changes. Never log passwords/tokens/PII/keys. Log encoding prevents injection. Alerting on failed auth spikes, privilege escalation attempts. |
| **A10** | Server-Side Request Forgery | URL allowlists for outbound fetching. Block internal IPs (127/8, 10/8, 172.16/12, 192.168/16, 169.254.169.254). Validate URL scheme (https only). Disable HTTP redirects for server-side requests. Never return raw fetched responses. |

### OWASP API Security Top 10 (2023)

| ID | Risk | Mandatory Controls |
|---|---|---|
| **API1** | Broken Object Level Authorization (BOLA) | Every DB read/write checks ownership or role. UUIDs for all resource IDs (never sequential). `middleware.UserFromCtx(ctx)` verified against resource owner. Table-driven tests: user A cannot access user B's resources. |
| **API2** | Broken Authentication | WebAuthn passkeys (AAL2). Stricter rate limits on auth endpoints than general. Account lockout after repeated failures. Re-confirm current password for sensitive changes. API keys for service auth only. JWT validation on every request. |
| **API3** | Broken Object Property Level Authorization | Explicit field selection in SQL (`SELECT id, name` — never `SELECT *`). Response DTOs from `oapi-codegen` define returned fields. Never bind raw request body to DB model. Map explicitly: `user.Name = req.Name`. |
| **API4** | Unrestricted Resource Consumption | Rate limiting middleware (per-user, per-IP). `http.MaxBytesReader` on every handler. Enforce max `page_size` (100). `maxLength`/`maxItems` in OpenAPI. `context.WithTimeout` on all DB queries and external calls. Upload file size limits. |
| **API5** | Broken Function Level Authorization | Default deny on all routes. RBAC middleware checks role before handler. Admin routes use separate route group. Never rely on client-side role checks. Audit all endpoints against role matrix in CI tests. |
| **API6** | Unrestricted Access to Sensitive Business Flows | Per-operation rate limits (not just per-endpoint). Pattern analysis for non-human timing. Business logic plausibility checks. Bot detection on sensitive flows. |
| **API7** | Server-Side Request Forgery | Same as A10 above. Validate webhook URLs against allowlists. Validate agent identity via token, never by network location. Block cloud metadata endpoints. |
| **API8** | Security Misconfiguration | Enforce `Content-Type: application/json`. CORS explicit origin allowlist (never `null`). RFC 9457 errors only. TLS 1.3 only. Strip `Server` header. |
| **API9** | Improper Inventory Management | Single OpenAPI spec is source of truth. `oapi-codegen` generates server interface — undocumented endpoints cannot exist. No beta endpoints without full security middleware. API inventory in spec with auth requirements and rate limits. |
| **API10** | Unsafe Consumption of APIs | Validate/sanitize all external API data before processing. TLS required for all outbound. Disable redirect following. Timeouts on all external HTTP calls (`http.Client{Timeout: 10 * time.Second}`). Treat external data as untrusted input. |

### OWASP ASVS v4.0.3 — L3 Requirements (Beyond Top 10)

Target L2 all chapters, **L3 for V2 (Auth), V3 (Sessions), V6 (Crypto)**:
- **V2 (L3):** 12+ char passwords, 64+ max, Unicode, no composition rules, no forced rotation, breach list check, allow paste. Max 100 failed/hour. Phishing-resistant MFA (FIDO2).
- **V3 (L3):** New token on auth. 64-bit min entropy. `Secure; HttpOnly; SameSite` cookies. `__Host-` prefix. 12hr/30min timeouts. Invalidate on logout. Re-auth before sensitive ops.
- **V6 (L3):** CSPRNG only. No ECB/MD5/SHA-1/3DES. Authenticated encryption (GCM). Constant-time comparisons. Crypto agility.
- **V7:** No credentials in logs. Generic errors with unique request IDs. Last-resort error handler (`recover()` middleware).
- **V11:** Sequential processing (no step skipping). TOCTOU protection (`sync.Mutex` or DB locking).
- **V12:** Max file size limits. Content-type by content (not extension). `filepath.Clean`. `Content-Disposition: attachment`.
- **V13:** No sensitive data in URLs (exception: OIDC callback per RFC 6749 §4.1.2). Reject unexpected content types (406/415).

### NIST Enforcement Rules

When writing ANY code:
1. APIs follow SP 800-228 (REC-API-1 through REC-API-26)
2. Authentication meets SP 800-63B AAL2 minimum
3. All 20 SP 800-53 control families considered
4. Development follows SSDF practices (PO, PS, PW, RV)
5. Security posture aligns with CSF 2.0 six functions
6. Zero-trust on all API communications
7. PII handling respects PT controls
8. Dependencies vetted per SR supply chain controls
9. Error responses never leak internal details (REC-API-23)
10. All sensitive data masked in logs (REC-API-25)

### OWASP Enforcement Rules

When writing ANY code:
1. Every handler checks resource ownership (BOLA — API1, A01, V4)
2. No `SELECT *` — explicit field selection only (API3, V5)
3. All user input validated against OpenAPI schema before handler executes (A03, V5, API8)
4. All outbound HTTP requests use allowlists, block internal IPs, enforce TLS, set timeouts (A10, API7, V12.6)
5. Rate limits exist at both per-endpoint and per-business-operation levels (API4, API6, V11)
6. Every DB query uses parameterized statements — zero exceptions (A03, V5.3)
7. Error responses are RFC 9457 only — never expose internals (A05, API8, V7)
8. File operations validate paths with `filepath.Clean`, enforce size limits, set `Content-Disposition` (V12, A03)
9. All external API data treated as untrusted input (API10, V5)
10. Session cookies use `Secure; HttpOnly; SameSite=Strict; __Host-` prefix (V3, A07)

### Security Headers Quick Reference

```
Content-Security-Policy: [restrict script/style/frame sources]
Strict-Transport-Security: max-age=63072000; includeSubDomains; preload
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: camera=(), microphone=(), geolocation=()
X-Request-Id: <uuid> (request correlation, NIST AU-3)
Cache-Control: no-store (on sensitive responses — auth, user, session, audit)
```

---

## Development Standards

### Go Patterns
- **Router:** `chi/v5` — stdlib-compatible `http.Handler`, middleware chain, route groups
- **CLI:** `cobra` + `viper` — subcommands, flag parsing, shell completions, `server.yaml` config
- **Logging:** `log/slog` — stdlib structured JSON logging (NIST 800-92 compliant)
- **DI:** Constructor injection via struct fields — no frameworks (e.g., `server.New(db, logger, config)`)
- **Shutdown:** `signal.NotifyContext` + `context.Context` propagation through all layers
- **HTTP handlers:** Standard `http.HandlerFunc` via chi
- **Middleware order:** Rate limit → CORS → Auth (JWT) → Tenant → RBAC → Audit → Validation → Handler
- **Packages:** Singular names (`auth`, `db`, `server` — never `auths`, `database`, `servers`)
- **Env vars:** `CONDUIT_` prefix for all environment variables. Viper: `viper.SetEnvPrefix("CONDUIT")`
- **SQL safety:** Parameterized queries only. Never use `fmt.Sprintf` or string concatenation in SQL. `gosec` catches this (SI-10)
- **File naming:** Go files use snake_case per stdlib convention (`shell_linux.go`, `install_darwin.go`)
- **DB access:** Methods on a shared `*DB` struct, one file per entity (`users.go`, `agents.go`). No repository interfaces — tests use real in-memory SQLite
- **tenant_id:** `db.TenantID()` returns the single CE tenant UUID (loaded at startup). Every INSERT includes it. No WHERE tenant_id scoping in CE — single tenant, column exists for SaaS transferability only
- **Request context:** Typed context keys + helpers in `internal/middleware`: `middleware.UserFromCtx(ctx)`, `middleware.TenantIDFromCtx(ctx)`, `middleware.RequestIDFromCtx(ctx)`. Never use string keys

### Error Handling
- All API handlers return RFC 9457 `application/problem+json` via a shared `internal/apierror` package
- Wrap errors with context: `fmt.Errorf("creating user: %w", err)`
- Never panic in handlers
- Never return raw error strings, stack traces, or DB errors to clients (SI-11, REC-API-23)
- Log full error details server-side via `slog`, return only the RFC 9457 shape to the client
- Use `errors.Is` / `errors.As` for error type checking

### API Code Generation
- **Go server:** `oapi-codegen` generates types + chi server interface + validation middleware from `openapi.yaml`
- **TS client:** `openapi-typescript` generates types, `openapi-fetch` provides type-safe fetch wrapper
- Never hand-write types that exist in the OpenAPI spec
- Spec is the source of truth — edit `openapi.yaml` first, regenerate, then update handlers
- Generated code goes in `internal/api/generated/` — never edit generated files

### Frontend Patterns
- **File naming:** kebab-case for all files (`agent-list.tsx`, `use-websocket.ts`, `login-form.tsx`)
- **Components:** PascalCase exports (`export function AgentList`) in kebab-case files
- **Hooks:** `use-` prefix, kebab-case (`use-auth.ts`, `use-websocket.ts`)
- **Data fetching:** Raw `openapi-fetch` + `useState`/`useEffect`. No SWR or React Query — the WebSocket EventBus handles real-time state. Fetch on mount, EventBus updates replace state
- **No caching libraries:** The EventBus is the revalidation layer. Two competing freshness systems create bugs
- **User-facing warnings:** All warnings MUST use styled shadcn/ui components — never `window.alert()`, `window.confirm()`, or unstyled browser dialogs. Use **Sonner toast** for transient notifications, **shadcn Alert** for inline persistent warnings, **shadcn AlertDialog** for blocking confirmations

### Testing Strategy
- **Go:** Table-driven tests, `httptest` for handlers, in-memory SQLite (`:memory:`) for DB tests
- **Assertions:** `testify` (`assert` + `require`)
- **Frontend:** `vitest` + `@testing-library/react`
- **E2E:** Playwright
- Test files live next to source: `foo.go` → `foo_test.go`
- No mocks for SQLite — use real in-memory databases
- Security-critical behavior (auth, tenant isolation, input validation) must have test coverage

### Pre-Commit Testing Requirement (MANDATORY)
Every endpoint or handler must have passing unit tests BEFORE committing. No exceptions.
- **Every HTTP handler:** At least one table-driven test per handler covering success path, auth failure, validation rejection, and not-found. Use `httptest.NewServer` + real in-memory SQLite.
- **Every DB method:** At least one test per CRUD operation verifying correct SQL behavior with real SQLite (`:memory:`).
- **Every middleware:** Test that it rejects unauthorized/invalid requests and passes valid ones.
- **Every frontend component that calls an API:** At least one test verifying render + basic interaction.
- **Run tests before committing:** `go test ./...` must pass for Go. `pnpm test` must pass for frontend. Do not commit code with failing or missing tests.
- **No skipping:** Do not use `t.Skip()`, `xit`, or `describe.skip` to bypass failing tests. Fix the code or fix the test.
- Tests are not optional polish — they are a gate. Untested endpoints do not ship.

### Implementation Status Tracking (MANDATORY)
After completing any feature, update its status in the README.md implementation status table before committing. Never commit code for a feature without updating its status.

**Status format — use colored checkmark emoji + label:**
- ✅ `done` — feature complete with passing tests
- 🟡 `in-progress` — actively being worked on
- ❌ `spec-only` — specified but not yet started

### Linting & Formatting
- **Go:** `golangci-lint` with project `.golangci.yml` — includes `gofumpt`, `govet`, `errcheck`, `staticcheck`, `gosec`, `bodyclose`, `sqlclosecheck`, `exhaustive`, `noctx`, `unparam`, `wastedassign`, `errorlint`, `tenv`
- **Frontend:** Biome (lint + format in one tool) with project `biome.json`
- All code must pass lint before committing
- `gofumpt` for Go formatting (stricter than `gofmt`)

### Migration Strategy
- Embedded SQL files in `internal/db/migrations/` via `embed.FS`
- Applied sequentially at startup (no migration CLI, no down migrations)
- Each migration is idempotent
- Schema version tracked in a `schema_version` table
- Hand-rolled runner (~50 lines of Go) — no migration library

### Approved Dependencies

Do NOT add dependencies without explicit approval.

**Go:**

| Package | Purpose |
|---------|---------|
| `github.com/go-chi/chi/v5` | Router + middleware |
| `github.com/quic-go/quic-go` | QUIC transport |
| `github.com/go-webauthn/webauthn` | WebAuthn/passkeys |
| `github.com/charmbracelet/bubbletea` | TUI |
| `github.com/charmbracelet/lipgloss` | TUI styling |
| `github.com/charmbracelet/bubbles` | TUI input components |
| `golang.org/x/crypto` | Argon2id, HKDF, ACME |
| `github.com/spf13/cobra` | CLI |
| `github.com/spf13/viper` | Config (server.yaml) |
| `github.com/golang-jwt/jwt/v5` | JWT Ed25519 |
| `github.com/oapi-codegen/oapi-codegen` | API codegen (build tool) |
| `github.com/stretchr/testify` | Test assertions |
| `github.com/creack/pty` | PTY (Linux/macOS agent) |
| `github.com/coder/websocket` | WebSocket transport |
| `modernc.org/sqlite` | SQLite (pure Go, no CGO) |

**Frontend (npm):**

| Package | Purpose |
|---------|---------|
| `next`, `react`, `react-dom` | Framework |
| `tailwindcss`, `tw-animate-css` | Styling |
| `@radix-ui/*` | shadcn primitives (auto-installed by CLI) |
| `zod` | Validation |
| `react-hook-form`, `@hookform/resolvers` | Forms |
| `@xterm/xterm`, `@xterm/addon-fit`, `@xterm/addon-webgl` | Terminal |
| `date-fns` | Dates |
| `cmdk`, `sonner`, `vaul` | shadcn component deps |
| `openapi-typescript`, `openapi-fetch` | Type-safe API client |
| `next-themes` | Dark mode |
| `class-variance-authority`, `clsx`, `tailwind-merge` | CVA + cn() |

**Dev only (npm):**

| Package | Purpose |
|---------|---------|
| `@biomejs/biome` | Lint + format |
| `vitest`, `@testing-library/react`, `@testing-library/jest-dom` | Testing |
| `typescript`, `@types/react`, `@types/react-dom` | Types |

---

## shadcn/ui Standards

Follow all rules in global `~/.claude/CLAUDE.md` shadcn/ui section. Conduit additions:
- Dark mode: `next-themes` with `ThemeProvider attribute="class"`, `.dark` CSS variables in globals.css
- Forms: Always use `react-hook-form` + `zod` + `@hookform/resolvers` + shadcn Form component

---

## Product Spec

### Agent Join Security

**Two token types:** single-use (one machine, revoked after first join, default 1hr TTL) and persistent (fleet enrollment, configurable TTL/manual revocation).

Token is a signed JWT: `{jti, type, labels, exp, iss}`. Usage: `conduit join <server-url> <token>`.

**Join flow:** Agent validates token with server (`POST /api/v1/agents/register`) → server validates signature/expiry/single-use → generates agent UUID + HMAC-SHA256 key → applies labels → returns agent ID + key + endpoints → agent stores creds + installs as system service → connects.

**Post-join auth:** Agent authenticates every connection via HMAC-SHA256 of CWP HELLO payload. Join token never reused.

**Labels:** Set at join via `--labels`, mutable after join, used for targeting (`conduit exec --label env=production -- uptime`).

**Service paths:** Linux: `/usr/local/bin/conduit` + `/etc/conduit/agent.yaml` (systemd). macOS: same binary + `/Library/Application Support/Conduit/agent.yaml` (launchd). Windows: `C:\Program Files\Conduit\conduit.exe` + `C:\ProgramData\Conduit\agent.yaml` (Windows service).

### Connection Persistence

- **QUIC primary** (UDP 443/8443), WebSocket fallback (`wss://<server>/agent/v1/connect` TCP 443/8443)
- **Reconnect:** Exponential backoff 1s→30s cap, ±30% jitter, transport alternation after 3 consecutive failures
- **Heartbeat:** PING/PONG every 15s, 3 missed = dead → reconnect. Network interface change = immediate reconnect
- **Server states:** `online` → `offline` (45s no PONG) → `stale` (24h default). Reconnecting agent resumes identity

### Auth Model

**Production (Passkeys):** WebAuthn sole human auth. Endpoints: `/api/v1/auth/webauthn/{register,login}/{begin,finish}`. JWT: Ed25519, 15min access + 24hr refresh with rotation. Claims: `sub`, `tid`, `services`, `roles`, `permissions`, `iat`, `exp`. Middleware checks `jwt.services` includes endpoint's `x-service` tag.

**Dev Mode:** `--dev` flag or `server.mode: dev`. Setup token persists as Argon2id password_hash. `POST /api/v1/auth/password/login` with `{email, password}`. Both passkey + token auth available. Port 8443, self-signed certs. Webhook URLs allow `http://localhost`. Agent joins with `--dev-insecure`.

**Setup Wizard:** Server generates CSPRNG setup token → localhost:8080 → collects token + domain + admin info → validates token (constant-time) → ACME cert → creates tenant + admin (platform_owner) + seeds remote-access service → restarts on 443 → admin registers passkey → production: token DELETE'd, dev: token persists → setup permanently deactivated.

**Recovery:** (1) 10 one-time codes (Argon2id hashed), using one returns 5-min JWT scoped to `passkey:register`. (2) Admin reset via `/users/{userId}/recovery/reset`.

**CLI Auth:** Browser device flow → scoped CLI token → stored encrypted in `~/.config/conduit/credentials.yaml`.

### Wire Protocol (CWP)

Binary frame: `[Type 1B][StreamID 4B][Length 4B][Payload]`. QUIC: one stream per op. WebSocket: multiplexed by StreamID.

| Type | ID | Direction | Purpose |
|---|---|---|---|
| HELLO | 0x01 | A→S | Agent identity (hostname, OS, arch, agent-id) |
| AUTH | 0x02 | Both | HMAC auth; server OK/REJECT |
| SHELL_DATA | 0x10 | Both | PTY stdin/stdout bytes |
| SHELL_RESIZE | 0x11 | S→A | Terminal resize (cols, rows) |
| SHELL_START | 0x12 | S→A | Request new shell session |
| SHELL_EXIT | 0x13 | A→S | Shell exited (exit code) |
| FILE_LIST | 0x20 | Both | Directory listing |
| FILE_READ | 0x21 | Both | File content (chunked) |
| FILE_WRITE | 0x22 | S→A | Write file (chunked) |
| FILE_STAT | 0x23 | Both | File metadata |
| AGENT_INFO | 0x30 | A→S | System metrics (CPU, mem, disk) |
| EXEC_START | 0x40 | S→A | Start bulk exec |
| EXEC_DATA | 0x41 | A→S | Streaming exec output |
| EXEC_EXIT | 0x42 | A→S | Exec exited (exit code) |
| PING | 0xF0 | Both | Keepalive |
| PONG | 0xF1 | Both | Keepalive response |

### TUI (bubbletea)

`conduit` with no args launches TUI: agent list → Enter for shell. Vim navigation (j/k, /, Esc). Status bar with user + connection state. Direct CLI: `conduit shell <agent>`, `conduit join`, `conduit token create`, `conduit agent`.

### Persistent & Resumable Shell Sessions

Sessions decouple from browser connections. PTY runs independently on agent.

**Lifecycle:** `active` (browser connected) → `detached` (browser disconnects, output buffered in ring buffer, idle timer starts) → `active` (browser reattaches, ring buffer replayed) → `closed` (idle timeout or explicit terminate, recording saved, PTY killed).

**Ring buffer:** 256KB circular buffer per session for output replay on reattach.

**Idle reaper:** Runs every 30s. Non-pinned detached sessions close after `idleTimeout` (default 1hr). Pinned sessions never reaped.

**Pin mode:** `pinned: true` at creation or via PATCH. Survives indefinitely until explicit terminate or agent disconnect.

**Pop-out windows:** `/terminal/pop?session={sessionId}` — minimal chrome, no sidebar. BroadcastChannel coordinates with dashboard. Close = detach, not terminate.

**Security:**

| Control | Implementation |
|---------|----------------|
| AC-3 | Only session owner can attach. org_admin+ can view/terminate any. |
| AC-12 | Idle timeout on detached sessions (unless pinned). |
| AU-2 | All lifecycle events logged: start, detach, attach, idle_timeout, terminate. |
| AU-3 | Recordings continue during detached state — no audit gaps. |
| IA-11 | JWT re-validated on every attach. |
| API1 | Session ownership verified on every attach/update/terminate. |

### Transport & TLS

| Connection | Primary | Fallback |
|---|---|---|
| Agent to server | QUIC (UDP 443) | WSS (TCP 443) |
| Browser real-time | WebSocket over HTTP/3 | WebSocket over HTTP/2 |
| Browser API | HTTP/3 | HTTP/2 |

**Production:** ACME, X25519MLKEM768, TLS 1.3 only, ciphers: AES-256-GCM, AES-128-GCM, ChaCha20-Poly1305. **Dev:** Self-signed TLS 1.3, port 8443.

### Frontend

Next.js 15 App Router → static export → `embed.FS`. Style: new-york, Tailwind v4, all fonts local, `output: 'export'`, CVA for variants.

**Pages — Public:** `/` (promo landing), `/login` (passkey + dev password), `/setup` (first-run wizard).
**Pages — Dashboard:** `/dashboard` (agent list), `/dashboard/terminal` (xterm.js session), `/dashboard/files/[agentId]` (file browser), `/dashboard/audit`, `/dashboard/users`, `/dashboard/webhooks`, `/dashboard/settings`.

**Real-time:** WebSocket EventBus at `/api/v1/events/stream`. No polling.

### Audit Logging

All access events logged (append-only, immutable): logins, sessions, shell commands, file ops, agent connections, permission changes. Each entry: who, what, when, where (source IP, agent), outcome.

### SQLite Schema

Schema defined in `internal/db/migrations/*.sql` (embedded, applied at startup). All tables include `tenant_id` for SaaS transferability.

---

## Project Memory

### Conduit CE
Branch `ce` is the Conduit Community Edition — the production foundation for the full SaaS product. Every feature built here must be production-quality. No shortcuts that would need rewriting. The CE is the base layer the SaaS sits on. No multi-tenancy, no billing, no sub-tenants.

### Branch Strategy
The `ce` branch is the main branch for all Community Edition work. Do not create PRs into `main` from CE branches. Always branch from `ce`. PRs target `ce`, not `main`.

### Branch Naming Convention
Dev branches from `ce` MUST use `ce-<random_hex>` format. Generate with: `openssl rand -hex 4` (e.g., `ce-4bb5d3a7`). Never use descriptive names like `ce-feature` or literal names like `ce-rand`.
