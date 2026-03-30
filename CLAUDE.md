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

### NIST Standards Coverage

| Standard | Coverage |
|---|---|
| **NIST SP 800-53 Rev. 5** | AC-2, AC-3, AC-6, AC-7, AC-11, AC-12, AC-17, AU-2, AU-3, AU-6, AU-9, AU-10, AU-12, CM-2, CM-3, CM-6, CM-7, IA-2, IA-4, IA-5, IA-8, IA-12, SC-8, SC-12, SC-13, SC-18, SC-23, SC-28, SI-2, SI-4, SI-7, SI-10, SI-12 |
| **NIST SP 800-63B** | AAL2 baseline, AAL3 with hardware authenticator |
| **NIST SP 800-131A Rev. 2** | Cryptographic algorithm selection and transition |
| **NIST SP 800-57** | Key management lifecycle |
| **FIPS 203 (ML-KEM)** | X25519MLKEM768 hybrid TLS key exchange |
| **FIPS 204 (ML-DSA)** | ML-DSA-65 JWT signing (when Go stdlib ships) |
| **FIPS 205 (SLH-DSA)** | SLH-DSA-SHA2-256s binary signing |

### NIST SP 800-228 — API Security Controls

Every API endpoint must implement:

**Pre-Runtime:**
- REC-API-1: Documented API specification for every endpoint
- REC-API-2: Standardized interface definitions (OpenAPI/gRPC)
- REC-API-3: Request/response schema validation on all endpoints
- REC-API-4: Centralized API inventory with ownership and metadata
- REC-API-5: Input/output parameter validation in specs
- REC-API-6: Sensitivity and permission annotations on data fields
- REC-API-7: Semantic data type annotations for PII/PHI identification
- REC-API-8: Runtime information in API inventory

**Runtime (Basic):**
- REC-API-9: Encrypt ALL communication (TLS everywhere)
- REC-API-10: Bot detection and DoS mitigation
- REC-API-11: Auth credential handling with rate limiting, lockouts, MFA
- REC-API-12: Authorization verification on EVERY request
- REC-API-13: Syntactic validation of request/response structure
- REC-API-14: Input length validation (prevent buffer overflows)
- REC-API-15: Rate limiting per user/service
- REC-API-16: Circuit breakers with concurrency thresholds

**Runtime (Advanced):**
- REC-API-17: Fine-grained user/network blocking during incidents
- REC-API-18: API access monitoring (telemetry, logging, metrics)
- REC-API-19: Field-level validation via schema annotations
- REC-API-20: Authorization/filtering via schema annotations
- REC-API-21: Detailed telemetry (request/response logs, timestamps, status)
- REC-API-22: Non-signature payload scanning for data leaks
- REC-API-23: Error codes designed to prevent info disclosure
- REC-API-24: Resource enumeration attack mitigation
- REC-API-25: Data masking for sensitive info in responses/logs
- REC-API-26: Fine-grained request blocking for DoS/crash scenarios

**Zero-Trust (SP 800-207A):**
- Encrypt all traffic in transit
- Identity-based segmentation on all API communications
- Continuous identity verification at every API boundary
- Perimeter shrunk to service instance level

### NIST SP 800-53 Rev 5 — All 20 Control Families

| Family | Name |
|--------|------|
| AC | Access Control — RBAC, least privilege, separation of duties |
| AT | Awareness and Training |
| AU | Audit and Accountability — logging, report generation, audit protection |
| CA | Assessment, Authorization and Monitoring |
| CM | Configuration Management — baselines, inventories, impact analysis |
| CP | Contingency Planning — DR, backups, restoration |
| IA | Identification and Authentication |
| IR | Incident Response — training, testing, monitoring, reporting |
| MA | Maintenance |
| MP | Media Protection — access, labeling, storage, transport, sanitization |
| PE | Physical and Environmental Protection |
| PL | Planning |
| PM | Program Management |
| PS | Personnel Security |
| PT | PII Processing and Transparency — privacy, consent, data handling |
| RA | Risk Assessment — vulnerability scanning |
| SA | System and Services Acquisition — SAST, threat modeling, code review, pen testing, IAST |
| SC | System and Communications Protection — boundary protection, encryption, DDoS mitigation |
| SI | System and Information Integrity — flaw remediation, malware protection, monitoring |
| SR | Supply Chain Risk Management — dependency vetting, tampering prevention |

### NIST SP 800-63B — Digital Identity

- **IAL1**: No proofing | **IAL2**: Moderate verification | **IAL3**: Physical presence + supervised verification
- **AAL1**: Single-factor | **AAL2**: MFA mandatory (phishing-resistant preferred — FIDO Passkeys) | **AAL3**: Hardware authenticators
- **FAL1**: Basic federation | **FAL2**: Encrypted assertions | **FAL3**: Holder-of-key assertions
- Minimum AAL2 for all authenticated endpoints
- Risk-based Digital Identity Risk Management (DIRM)
- Session management and credential lifecycle management required

### NIST SP 800-218 — SSDF

**Prepare the Organization (PO):**
- PO.1: Define security roles/responsibilities
- PO.2: Implement security processes, policies, workflows
- PO.3: Define/communicate security requirements
- PO.4: Role-based security training

**Protect the Software (PS):**
- PS.1: Protect code from unauthorized access/tampering (version control + access controls)
- PS.2: Verify software release integrity (code signing)
- PS.3: Archive/protect release components (artifact repos + scanning)

**Produce Well-Secured Software (PW):**
- PW.1: Threat modeling in design (STRIDE)
- PW.2: Security design review
- PW.3: Vet open-source dependencies (SCA)
- PW.4: Secure coding standards (OWASP)
- PW.5: Hardened CI/CD pipelines, scan build artifacts
- PW.6: Mandatory security-focused code reviews
- PW.7: SAST, DAST, IAST, penetration testing
- PW.8: Secure defaults, IaC scanning

**Respond to Vulnerabilities (RV):**
- RV.1: Aggregate vulnerability findings (SAST, DAST, SCA, bug bounty)
- RV.2: Prioritize (CVSS/EPSS), remediate within SLAs
- RV.3: Root cause analysis, update processes

### NIST CSF 2.0 — Six Core Functions

1. **Govern (GV)**: Risk management strategy, expectations, policy
2. **Identify (ID)**: Assets, risks, supply chain understanding
3. **Protect (PR)**: Access control, data security, platform security, resilience
4. **Detect (DE)**: Continuous monitoring, anomaly detection
5. **Respond (RS)**: Incident management, analysis, mitigation, reporting
6. **Recover (RC)**: Recovery planning, execution, communication

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

### OWASP ASVS v4.0.3

Target **Level 2** for all chapters, **Level 3** for V2 (Auth), V3 (Sessions), V6 (Crypto).

| Chapter | Name | Key Requirements |
|---|---|---|
| **V1** | Architecture & Threat Modeling | Single vetted auth mechanism. Server-side enforcement only. Explicit key management policy. Consistent structured logging. Data classified into protection levels. |
| **V2** | Authentication (L3) | 12+ char passwords, 64+ max, Unicode, no composition rules, no forced rotation, breach list check, allow paste. Argon2id. Max 100 failed/hour. Phishing-resistant MFA (FIDO2). |
| **V3** | Session Management (L3) | New token on auth. 64-bit min entropy. `Secure; HttpOnly; SameSite` cookies. `__Host-` prefix. 12hr/30min timeouts. Invalidate on logout. Re-auth before sensitive ops. |
| **V4** | Access Control | Server-side enforcement. IDOR protection on CRUD. Anti-CSRF. No directory browsing. Segregation of duties. |
| **V5** | Validation & Encoding | Mass assignment protection. Schema validation. Parameterized queries. Context-specific output encoding. No `eval()`. SSRF prevention. |
| **V6** | Stored Cryptography (L3) | CSPRNG only. Approved algorithms only. No ECB/MD5/SHA-1/3DES. Authenticated encryption (GCM). Constant-time comparisons. Crypto agility. |
| **V7** | Error Handling & Logging | No credentials in logs. Log all auth/authz decisions. Encode logs to prevent injection. Generic errors with unique request IDs. Last-resort error handler (`recover()` middleware). |
| **V8** | Data Protection | Anti-caching headers on sensitive responses. No sensitive data in browser storage. Sensitive data in HTTP body only (never query strings). Data retention/auto-delete policies. |
| **V9** | Communications | TLS 1.3 on all connections. Strong cipher suites only. Trusted TLS certs. OCSP stapling. Log TLS failures. |
| **V10** | Malicious Code | `gosec` + `staticcheck` in CI. No hardcoded creds. All assets embedded. Signed releases. |
| **V11** | Business Logic | Sequential processing (no step skipping). Per-user rate limits. TOCTOU race condition protection (`sync.Mutex` or DB locking). Unusual activity monitoring. |
| **V12** | Files & Resources | Max file size limits. Content-type validation by content (not extension). Path traversal prevention (`filepath.Clean`). Store outside web root. `Content-Disposition: attachment` for downloads. |
| **V13** | API & Web Service | No sensitive data in URLs. Authorization at URI and resource level. Reject unexpected content types (406/415). JSON schema validation. CSRF via SameSite + Origin validation. |
| **V14** | Configuration | Debug disabled in prod. No version info in headers. Content-Type with charset. CSP, HSTS, X-Content-Type-Options, Referrer-Policy. CORS allowlist. `govulncheck` + `npm audit` in CI. SBOM maintained. |

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
Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: [restrict browser features]
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

### Testing Strategy
- **Go:** Table-driven tests, `httptest` for handlers, in-memory SQLite (`:memory:`) for DB tests
- **Assertions:** `testify` (`assert` + `require`)
- **Frontend:** `vitest` + `@testing-library/react`
- **E2E:** Playwright
- Test files live next to source: `foo.go` → `foo_test.go`
- No mocks for SQLite — use real in-memory databases
- Security-critical behavior (auth, tenant isolation, input validation) must have test coverage

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

Follow all rules in global `~/.claude/CLAUDE.md` shadcn/ui section. Conduit-specific additions:

### Project Structure (Frontend)
```
web/
├── app/                    # Next.js App Router pages
│   ├── layout.tsx
│   ├── page.tsx            # Promo landing page
│   ├── login/
│   ├── setup/
│   └── dashboard/          # Auth-required pages
├── components/
│   ├── ui/                 # shadcn components ONLY (CLI-generated)
│   └── *.tsx               # Custom components (kebab-case)
├── hooks/
│   └── use-*.ts            # Custom hooks (kebab-case)
├── lib/
│   ├── utils.ts            # cn() utility
│   └── api.ts              # openapi-fetch client
└── public/
    ├── fonts/              # Local font files (no CDN)
    └── og/                 # Pre-generated OG images
```

### Dark Mode
```bash
pnpm add next-themes
```
Use `ThemeProvider` with `attribute="class"`, define `.dark` CSS variables in globals.css.

### Forms Pattern
Always use: `react-hook-form` + `zod` + `@hookform/resolvers` + shadcn Form component.

---

## Product Spec

### Architecture

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

### Monorepo Layout

```
conduit/
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

### Agent Join Security

#### Join Tokens

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

#### Join Flow

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

#### Agent Identity (Post-Join)

After joining, the agent authenticates on every connection using its unique HMAC-SHA256 agent key. The join token is never used again — it was only for enrollment.

```
Agent connects → QUIC/WS handshake → CWP HELLO frame (hostname, OS, arch, agent-id)
→ CWP AUTH frame (HMAC-SHA256 signature of HELLO payload using agent key)
→ Server validates → AUTH OK/REJECT
```

#### Label Scoping

Labels assigned during enrollment are the primary mechanism for organizing and targeting agents:

- **Set at join time** via the token: `--labels env=production,role=web,dc=us-east-1`
- **Mutable after join** — operator can add/remove labels from the dashboard or CLI
- **Used for targeting** — shell access, bulk exec, file operations can target by label selector
  - `conduit exec --label env=production -- uptime`
  - `conduit shell --label role=web` (if multiple matches, shows picker)
- **Displayed in dashboard** — agents grouped/filterable by labels

#### System Service Installation

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

### Connection Persistence Strategy

The agent must stay connected to the server at all times. Disconnection = blind spot.

#### QUIC Primary Path
- Agent dials server on UDP 443 (prod) / UDP 8443 (dev)
- QUIC 0-RTT reconnection for fast recovery after brief disconnects
- QUIC handles packet loss and network migration natively
- If QUIC connection fails for ~5 seconds, agent falls back to WebSocket

#### WebSocket Fallback Path
- Agent dials `wss://<server>/agent/v1/connect` on TCP 443 (prod) / TCP 8443 (dev)
- Used when UDP is blocked (corporate firewalls, restrictive NAT)
- Same CWP frames, multiplexed via StreamID over single WS connection

#### Reconnect Strategy
- **Exponential backoff with jitter:** 1s → 2s → 4s → 8s → 16s → 30s (cap)
- **Jitter:** +/- 30% randomization to prevent thundering herd
- **Transport alternation:** If QUIC fails 3 consecutive times, try WebSocket. If WebSocket fails 3 times, try QUIC again.
- **On success:** Reset backoff timer, log reconnection event
- **Heartbeat:** PING/PONG every 15 seconds. If 3 consecutive PINGs unanswered, consider connection dead and reconnect.
- **Network change detection:** Agent monitors network interfaces; on change, immediately attempt reconnect (don't wait for heartbeat timeout)

#### Connection State (Server-Side)
- Server tracks each agent: `online`, `offline`, `stale`
- `offline` after heartbeat timeout (45s with no PONG)
- `stale` after extended disconnect (configurable, default 24h)
- Dashboard shows real-time connection status via EventBus
- Reconnecting agent resumes its identity — same agent ID, same labels

### Auth Model

#### Production Mode (Passkeys)

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

#### Dev Mode (Setup Token as Password)

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

#### Setup Wizard (First Run)

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

#### CLI Authentication

- TUI/CLI opens browser to server's login page
- User authenticates with passkey in browser
- Browser-based device flow returns scoped CLI token
- Token stored encrypted in `~/.config/conduit/credentials.yaml`
- CLI uses token for all API calls and WebSocket connections

### Wire Protocol (CWP)

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

### TUI (bubbletea) — Shell Only

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

### Frontend — shadcn/ui + Next.js Static Export

**Next.js 15 App Router → static export → embedded in Go binary via `embed.FS`**

#### Setup
- Style: `new-york`
- `rsc: true` (ensures `"use client"` directives present)
- Tailwind v4 (CSS-first config, no `tailwind.config.js`)
- `pnpm dlx skills add shadcn/ui` for AI accuracy
- All fonts local in `public/fonts/` — zero CDN dependencies
- `output: 'export'` + `images: { unoptimized: true }`
- Component variants via CVA — no hardcoded className strings

#### Pages

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

#### Real-Time Updates
- WebSocket EventBus at `/api/v1/events/stream`
- Agent connect/disconnect events push to all browsers immediately
- No polling. No manual refresh.

### Transport & TLS

Every endpoint in `openapi.yaml` is tagged with `x-nist-controls` and `x-owasp` annotations.

| Connection | Primary | Fallback |
|---|---|---|
| Agent to master | QUIC (UDP 443) | WSS (TCP 443) |
| Browser real-time | WebSocket over HTTP/3 | WebSocket over HTTP/2 |
| Browser API | HTTP/3 | HTTP/2 |

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

### SQLite Schema (Community Edition)

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

### Open Questions

None remaining — ready to build. If anything surfaces during implementation, we'll address it inline.

---

## Project Memory

### Conduit CE
Branch `ce` is the Conduit Community Edition — the production foundation for the full SaaS product. Every feature built here must be production-quality. No shortcuts that would need rewriting. The CE is the base layer the SaaS sits on. No multi-tenancy, no billing, no sub-tenants.

### Branch Strategy
The `ce` branch is the main branch for all Community Edition work. Do not create PRs into `main` from CE branches. Always branch from `ce`. PRs target `ce`, not `main`.

### Branch Naming Convention
Dev branches from `ce` MUST use `ce-<random_hex>` format. Generate with: `openssl rand -hex 4` (e.g., `ce-4bb5d3a7`). Never use descriptive names like `ce-feature` or literal names like `ce-rand`.
