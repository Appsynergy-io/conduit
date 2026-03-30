# Project Directives

## MANDATORY: NIST Security Compliance

All code in this project MUST comply with the following NIST guidelines. These are non-negotiable.

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

### NIST SP 800-53 Rev 5 — All 20 Control Families Apply

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

### NIST SP 800-218 — Secure Software Development Framework (SSDF)

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

### Enforcement Rules

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

---

## MANDATORY: OWASP Compliance

All code MUST comply with OWASP Top 10 (2021), OWASP API Security Top 10 (2023), and OWASP ASVS v4.0.3 (Level 2, Level 3 for auth/crypto). These are non-negotiable.

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

### OWASP ASVS v4.0.3 — Applicable Chapters

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

---

## Development Standards

### Document Precedence
- `ce.md` is the product spec (what to build) — wins for product decisions
- `openapi.yaml` is the API contract (endpoint shapes, schemas, validation)
- `CLAUDE.md` is the coding rules (how to write code)

### Project Security Overrides
- **TLS 1.3 only** — TLS 1.2 is forbidden. The global NIST SP 800-52 guidance permitting TLS 1.2 as a minimum does not apply to Conduit.
- **JWT signing: Ed25519** — The global NIST SP 800-175B guidance recommending RS256/ES256 does not apply. This project uses Ed25519 (future: ML-DSA-65).
- See `ce.md` Cryptographic Policy for the full list of algorithms and forbidden primitives.

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

# Global Standards & Guidelines

## NIST Security Standards (MANDATORY)

All projects MUST follow NIST security guidelines. These are non-negotiable standards.

### NIST SP 800-53 Rev. 5 — Security Controls
Apply these control families to all software:
- **AC (Access Control)**: Enforce RBAC/ABAC, least privilege (AC-6), session termination (AC-12), failed login handling (AC-7)
- **AU (Audit & Accountability)**: Log all security events (AU-2), include who/what/when/where/outcome (AU-3), protect logs from tampering (AU-9)
- **IA (Identification & Authentication)**: MFA (IA-2), modern password policies per 800-63B (IA-5), re-authentication for sensitive ops (IA-11)
- **SC (System & Communications Protection)**: TLS 1.2+ everywhere (SC-8), encrypt at rest (SC-28), protect session tokens (SC-23), boundary protection via API gateways (SC-7)
- **SI (System & Information Integrity)**: Validate ALL inputs (SI-10), minimal error info to users (SI-11), patch vulnerabilities promptly (SI-2), integrity verification (SI-7)
- **CM (Configuration Management)**: Hardened configs (CM-6), disable unnecessary services (CM-7), maintain software BOM (CM-8)
- **SR (Supply Chain)**: Track component provenance (SR-4), verify authenticity (SR-11)

### NIST Cybersecurity Framework (CSF) 2.0 — 6 Functions
1. **GOVERN**: Establish secure coding policies, SDLC governance, supply chain policies
2. **IDENTIFY**: Maintain asset inventory (APIs, services, data stores, dependencies, SBOMs)
3. **PROTECT**: Authentication, authorization, encryption, input validation, secure headers
4. **DETECT**: Application monitoring, WAF, anomaly detection on API traffic
5. **RESPOND**: Incident response, vulnerability disclosure processes
6. **RECOVER**: Rollback capabilities, backup/restore, communication plans

### NIST SP 800-63B — Password & Authentication (CRITICAL)
- Minimum 8 chars (prefer 12+), support up to 64+ chars
- Accept ALL characters including spaces and Unicode
- NO composition rules (no forced uppercase/number/special char)
- NO periodic password expiration (only on evidence of compromise)
- NO security questions / knowledge-based auth
- Check against breached password lists (HaveIBeenPwned)
- Hash with **Argon2id** (preferred), scrypt, bcrypt, or PBKDF2 (600k+ iterations)
- Allow paste in password fields, provide show/hide toggle
- MFA: prefer **FIDO2/WebAuthn** > TOTP apps > push notifications > SMS (restricted)
- Session tokens: 64+ bits entropy, invalidate on logout, regenerate on auth state change
- Timeouts: AAL2 = 12hr inactivity / 30min idle

### NIST SP 800-52 Rev. 2 — TLS Configuration
- TLS 1.3 preferred, TLS 1.2 minimum. NEVER use TLS 1.1/1.0/SSL
- Cipher suites: AEAD only (AES-GCM, ChaCha20-Poly1305), ECDHE for forward secrecy
- Certificates: RSA 2048+ bit, ECDSA P-256+, enable OCSP stapling
- Enable HSTS with 1+ year max-age, consider preloading
- Disable TLS compression, use secure renegotiation

### NIST SP 800-175B — Cryptographic Standards
- Symmetric: AES-256-GCM for encryption
- Hashing: SHA-256/SHA-3 (NEVER MD5, avoid SHA-1)
- Signatures: Ed25519, ECDSA P-256, RSA 2048+ (3072 recommended)
- JWTs: Sign with RS256 or ES256, short expiry (5-15min)
- NEVER implement custom crypto — use vetted libraries (libsodium, platform crypto)
- Store keys in secrets managers (KMS, Vault) — NEVER in source code
- Rotate keys on defined schedules
- Prepare for post-quantum (ML-KEM/Kyber, ML-DSA/Dilithium)

### NIST SP 800-218 — Secure Software Development Framework (SSDF)
- **Prepare**: SAST, DAST, SCA, secrets scanning in CI/CD
- **Protect**: Sign releases, verify integrity, archive releases
- **Produce**: Threat modeling, secure coding practices, use vetted libraries, secure defaults
- **Respond**: Continuous vulnerability scanning, root cause analysis, remediation

### NIST SP 800-207 — Zero Trust Architecture
- Every request authenticated and authorized — no implicit trust from network location
- Short-lived tokens (5-15min JWT expiry)
- Validate tokens at EACH service, not just the gateway
- mTLS for all service-to-service communication
- Per-request context evaluation (identity + device + location + risk)
- Classify data, encrypt at rest and in transit, access controls at data layer

### NIST SP 800-190 — Container Security
- Minimal base images (distroless/Alpine/scratch)
- Scan images for vulnerabilities, sign images, verify before deployment
- NEVER include secrets in images
- Pin image versions (no `latest` in production)
- Don't run containers as root, drop unnecessary capabilities
- Read-only root filesystems, resource limits, seccomp profiles
- Use PodSecurity Standards in Kubernetes

### NIST SP 800-204 — Microservices Security
- Every service must have strong identity (X.509, SPIFFE)
- mTLS for all service-to-service communication
- Authorization at gateway AND each service (defense in depth)
- Network segmentation, circuit breakers, rate limiting
- Distributed tracing with correlation IDs, centralized logging

### NIST SP 800-92 — Log Management
- Log: auth events, authorization decisions, data changes, system events
- Each entry: timestamp (UTC), source, event type, user, source IP, target, action, outcome, severity
- Structured JSON logging, centralized storage, tamper protection
- NEVER log: passwords, tokens, PII, credit cards, keys
- Retention policies, real-time alerting on critical security events

### NIST SP 800-44 & 800-123 — Server/Web Security
- Minimal services, least privilege processes, no default content
- Disable directory listing, custom error pages (no internals)
- Administrative interfaces on separate networks with strong auth
- DMZ/network segmentation architecture

### NIST API Security (from SP 800-204, 800-95, 800-207)
- Authenticate every API call (OAuth 2.0 access tokens)
- Authorize at gateway AND service level
- Validate inputs against schemas (JSON Schema, OpenAPI)
- Rate limiting and throttling
- TLS 1.2+ (mTLS for service-to-service)
- Restrictive CORS (whitelist origins, methods, headers)
- Validate Content-Type headers
- Standardized error responses — never expose stack traces or DB errors
- Log all API calls with metadata, monitor for anomalies

### Quick Reference: Security Headers
```
Content-Security-Policy: [restrict script/style/frame sources]
Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
Referrer-Policy: strict-origin-when-cross-origin
Permissions-Policy: [restrict browser features]
```

---

## shadcn/ui Standards (MANDATORY)

All projects using shadcn/ui MUST follow these rules.

### What shadcn/ui IS
- NOT a traditional npm component library — components are copied into YOUR codebase
- Built on **Radix UI** (accessibility/behavior) + **Tailwind CSS** (styling) + **CVA** (variants)
- There is NO `@shadcn/ui` npm package

### Critical Rules
1. **NEVER** install or import from `@shadcn/ui`, `shadcn-ui`, or any package — it doesn't exist
2. **ALWAYS** run `npx shadcn@latest init` before adding components
3. **ALWAYS** run `npx shadcn@latest add <component>` — NEVER write components from scratch
4. **ALWAYS** import from local paths: `import { Button } from "@/components/ui/button"`
5. **NEVER** place custom components in `components/ui/` — that's for shadcn components only
6. **ALWAYS** use the `cn()` utility for merging Tailwind classes
7. **NEVER** strip Radix UI accessibility attributes (ARIA, keyboard handlers, focus management)
8. **ALWAYS** use the composition pattern (compound sub-components, not monolithic props)
9. **ALWAYS** use `npx shadcn@latest` (not old `npx shadcn-ui`)
10. **ALWAYS** check that dependencies are installed (each component has its own)

### Setup Checklist
```bash
# 1. Prerequisites: React project + Tailwind CSS + TypeScript + path aliases
# 2. Initialize
npx shadcn@latest init
# 3. Add components as needed
npx shadcn@latest add button card dialog form input label select
# 4. Check diff against upstream
npx shadcn@latest diff [component]
```

### `components.json` Must Be Correct
```json
{
  "$schema": "https://ui.shadcn.com/schema.json",
  "style": "new-york",
  "rsc": true,
  "tsx": true,
  "tailwind": {
    "config": "",
    "css": "app/globals.css",
    "cssVariables": true
  },
  "aliases": {
    "components": "@/components",
    "utils": "@/lib/utils",
    "ui": "@/components/ui",
    "hooks": "@/hooks"
  }
}
```

### Component Dependencies (Auto-installed by CLI)
- Dialog: `@radix-ui/react-dialog`
- Calendar: `react-day-picker`, `date-fns`
- Command: `cmdk`
- Carousel: `embla-carousel-react`
- Drawer: `vaul`
- Toast/Sonner: `sonner`
- Chart: `recharts`
- Data Table: `@tanstack/react-table`
- Form: `react-hook-form`, `@hookform/resolvers`, `zod`

### Architecture Layers
```
shadcn/ui (styled components you own)
  -> Radix UI (unstyled accessible primitives)
    -> Tailwind CSS (utility styling)
```

### The `cn()` Utility (ALWAYS use this)
```ts
import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
```

### Common Agent Mistakes to AVOID
- Writing component code from scratch instead of using CLI
- Importing from non-existent packages (`@shadcn/ui`)
- Forgetting peer dependencies (Radix, cmdk, vaul, etc.)
- Using string concatenation instead of `cn()` for classes
- Breaking accessibility by removing ARIA attributes or `forwardRef`
- Using monolithic prop APIs instead of composition pattern
- Putting custom components in `components/ui/`
- Using `tailwindcss-animate` instead of `tw-animate-css` (Tailwind v4 requires the CSS import, not the plugin)

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

### Dark Mode (Next.js)
```bash
pnpm add next-themes
```
Use `ThemeProvider` with `attribute="class"`, define `.dark` CSS variables in globals.css.

### Forms Pattern
Always use: `react-hook-form` + `zod` + `@hookform/resolvers` + shadcn Form component.

---

## Project Memory

### Conduit CE POC
Branch `ce` is the Conduit Community Edition — the production foundation for the full SaaS product. Every feature built here must be production-quality. No shortcuts that would need rewriting. The CE is the base layer the SaaS sits on. No multi-tenancy, no billing, no sub-tenants.

Key decisions:
- Multi-OS agents: Linux (systemd), macOS (launchd), Windows (service) — amd64 + arm64
- Self-contained binary (all assets embedded, zero CDN)
- Standard ports (443) in prod, 8443 in dev
- Single monorepo with both binaries (Go + Next.js static export)
- SQLite for persistence (single tenant — UUID generated at setup, tenantId on all records for SaaS transferability, no multi-tenant query scoping)
- TUI is bubbletea, shell-only for CE
- shadcn/ui for web frontend
- QUIC+WS wire protocol, passkeys, Let's Encrypt, OS service agent (systemd/launchd/Windows service)

### Branch Strategy
The `ce` branch is the main branch for all Community Edition work. Do not create PRs into `main` from CE branches. Always branch from `ce`. PRs target `ce`, not `main`.

### Branch Naming Convention
Dev branches from `ce` MUST use `ce-<random_hex>` format. Generate with: `openssl rand -hex 4` (e.g., `ce-4bb5d3a7`). Never use descriptive names like `ce-feature` or literal names like `ce-rand`.
