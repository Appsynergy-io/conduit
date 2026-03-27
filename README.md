# Conduit by AppSynergy — OpenAPI 3.1.1 Specification

Complete REST API specification for the Conduit remote infrastructure management platform.

## Overview

Conduit gives operators secure, instant shell access to any machine — without opening inbound ports, managing SSH keys, VPNs, or trusting the network. Every managed machine runs a lightweight agent that connects outbound to the Conduit master server. Operators authenticate with passkeys and have a full terminal, file manager, and management dashboard in seconds.

This spec defines every API endpoint for the `conduit-server` binary.

## Spec Details

| | |
|---|---|
| **OpenAPI Version** | 3.1.1 (latest, full JSON Schema 2020-12 alignment) |
| **File** | `openapi.yaml` |
| **Lines** | ~10,100 |
| **Endpoints** | 150 paths |
| **Schemas** | 85 component schemas |
| **Webhooks** | 8 event types |
| **Tags** | 32 domain groups |
| **Error Format** | RFC 9457 Problem Details (`application/problem+json`) |
| **Pagination** | Cursor-based |
| **Auth** | JWT (Ed25519), WebAuthn, HMAC-SHA256 agent tokens, CI tokens |

## API Domain Coverage

### Server Core
- Health check and server status
- Setup wizard (first-run, localhost-only)
- Server configuration (`server.yaml`)
- ACME/TLS certificate management (Let's Encrypt)
- White-label branding
- Cluster management (single node or join via token)
- Multi-master federation

### Auth & Identity
- WebAuthn passkey registration and login (AAL3)
- Temporary password login (setup/dev only, Argon2id)
- JWT token refresh and revocation (Ed25519 signed)
- CLI browser-based device flow
- CI/automation tokens (scoped, revocable)
- SAML and OIDC SSO providers
- PIV / smart card authentication
- Session visibility and revocation (web, CLI, CI, iOS)

### Users, Groups & RBAC
- User CRUD, suspend/reinstate, invitation flow
- Group management with membership
- Role-based access control (platform_owner, org_owner, org_admin, org_member)
- Permission listing and role assignments

### Multi-Tenancy & Billing
- Tenant CRUD, self-service signup (SaaS mode)
- Sub-tenant creation with visibility modes (full, aggregate, opaque)
- Sub-tenant resource allocation enforcement
- Plan builder (admin creates all plans from scratch, no hardcoded tiers)
- Dynamic feature registry (migration-driven toggles)
- Stripe billing integration (subscriptions, portal, invoices, usage)

### Agents & Connections
- Agent registration with single-use and persistent join tokens
- Agent CRUD, labels, group assignments
- System metrics (CPU, RAM, disk, network time-series)
- Agent auto-update (SLH-DSA-SHA2-256s signed binaries)
- Reboot/poweroff (lights-out basic)
- BMC/IPMI out-of-band management (Redfish, iDRAC, iLO, AMT, DASH)
- WebSocket agent connection endpoint (CWP protocol)
- Universal resource labelling system

### Shell & Terminal
- Shell session lifecycle (create, terminate, list)
- WebSocket bidirectional I/O (xterm.js compatible)
- Terminal resize (CWP SHELL_RESIZE)
- Session recording and playback (asciicast v2)

### File Management
- Directory listing, file preview
- Upload (with resumable support), download
- Delete, rename, mkdir
- All transfers over HTTPS/QUIC (no SFTP dependency)

### Bulk Operations
- Multi-server parallel command execution with streaming WebSocket output
- Binary deployment service with post-deploy commands

### Serial & Hardware
- Serial port listing (USB-to-serial, direct)
- Serial console sessions with WebSocket I/O
- Automated OS installation over serial interface

### Legacy SSH
- SSH connection profiles (stored credentials, host key verification)
- SSH sessions through agents
- SFTP file operations (list, download, upload)

### Kubernetes / k3s
- Cluster listing and details
- Namespaces, pods, deployments, services, nodes
- kubectl command execution via agent

### PXE / iPXE Provisioning
- OS profile builder (disk layout, network, packages, post-install scripts)
- iPXE boot provisioning with auto-enrollment
- Provisioning job tracking

### Vault (Secrets & Key Management)
- Secrets CRUD (AES-256-GCM encrypted at rest)
- Shamir Secret Sharing initialization and unseal
- TPM-sealed per-node secret delivery

### PKI (Certificate Management)
- Root and intermediate CA creation
- Certificate issuance (server, client, user, code-signing)
- Certificate revocation with reason codes
- Certificate download (PEM, DER, PKCS12)
- CRL distribution endpoint
- OCSP responder

### Object Storage
- Per-tenant storage bucket (files, scripts, binaries, artifacts)
- Upload, download, delete

### Log Management
- Full-text search (FTS5) across journald, syslog, Windows Event Log
- Live log tail via WebSocket
- Alert rules with webhook/email notification
- Log export and forwarding (S3, syslog, Splunk HEC)

### Audit & Compliance
- Immutable, tenant-scoped audit log for all access events
- Query with filters (event type, user, agent, IP, date range, outcome)
- Full-text search across event details
- Export as CSV or JSON

### Webhooks
- Subscription management (create, update, delete, test)
- HMAC-SHA256 signed payloads
- Delivery history with retry tracking
- 8 webhook event types (agent, auth, shell, user, tenant, audit)

### EventBus (Real-Time)
- WebSocket stream for live dashboard updates
- Channel-based subscriptions (agents, shell, files, auth, audit, metrics, exec, system)

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

**Forbidden:** MD5, SHA-1, DES, 3DES, RC4, TLS 1.0/1.1/1.2, non-CSPRNG sources.

### Transport

| Connection | Primary | Fallback |
|---|---|---|
| Agent to master | QUIC (UDP 443) | WSS (TCP 443) |
| Browser real-time | WebSocket over HTTP/3 | WebSocket over HTTP/2 |
| Browser API | HTTP/3 | HTTP/2 |

## Usage

### Viewing the Spec

Open `openapi.yaml` in any OpenAPI-compatible tool:

- [Swagger Editor](https://editor.swagger.io/) — paste or import the file
- [Redocly](https://redocly.com/) — `npx @redocly/cli preview-docs openapi.yaml`
- [Stoplight Studio](https://stoplight.io/studio) — visual editor
- VS Code with the [OpenAPI extension](https://marketplace.visualstudio.com/items?itemName=42Crunch.vscode-openapi)

### Code Generation

Generate server stubs and client SDKs from the spec:

```bash
# Go server (oapi-codegen)
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest
oapi-codegen -generate types,server,spec -package api openapi.yaml > api/api.gen.go

# TypeScript client types
npx openapi-typescript openapi.yaml -o src/lib/api-types.ts

# Validation middleware
# The spec enforces additionalProperties: false on all input schemas,
# matching the directive requirement: "Unknown JSON fields rejected on all API endpoints"
```

### Linting

```bash
# Redocly linter
npx @redocly/cli lint openapi.yaml

# Spectral (custom rules)
npx @stoplight/spectral-cli lint openapi.yaml
```

## File Structure

```
files/
  directive.md     # Project directive (single source of truth)
  openapi.yaml     # This OpenAPI 3.1.1 specification
  README.md        # This file
```

## Key Design Decisions

1. **Single file** — the spec is one file rather than split across directories. This simplifies code generation and avoids `$ref` resolution issues across tools. At 10K lines it's large but manageable.

2. **Cursor-based pagination** — all list endpoints use opaque cursors rather than offset/limit. This handles concurrent inserts and scales to large datasets.

3. **RFC 9457 errors** — all error responses use `application/problem+json` with structured field-level validation errors.

4. **`additionalProperties: false`** — all input schemas reject unknown fields, matching the directive's "no silent ignore" requirement.

5. **`x-nist-controls` and `x-audit-event`** — custom extensions on every security-relevant operation for compliance traceability.

6. **Nullable via type arrays** — uses OpenAPI 3.1's `type: ["string", "null"]` syntax (not the deprecated `nullable: true` from 3.0).

7. **WebSocket documented as upgrade endpoints** — since OpenAPI has no native WebSocket support, WS endpoints are documented as GET with 101 response and message schemas in the description.
