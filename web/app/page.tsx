import Link from "next/link"

export default function Home() {
  return (
    <div className="flex min-h-screen flex-col bg-background">
      {/* Header */}
      <header className="sticky top-0 z-50 border-b border-border bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-2">
            <span className="text-xl font-bold tracking-tight">Conduit</span>
            <span className="rounded-md bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
              CE
            </span>
          </div>
          <nav className="flex items-center gap-6">
            <a href="#features" className="text-sm text-muted-foreground transition-colors hover:text-foreground">Features</a>
            <a href="#quickstart" className="text-sm text-muted-foreground transition-colors hover:text-foreground">Quick Start</a>
            <a href="#security" className="text-sm text-muted-foreground transition-colors hover:text-foreground">Security</a>
            <Link
              href="/login"
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
            >
              Sign in
            </Link>
          </nav>
        </div>
      </header>

      <main className="flex-1">
        {/* Hero */}
        <section className="px-6 py-24 sm:py-32">
          <div className="mx-auto max-w-3xl text-center">
            <div className="mb-6 inline-flex items-center gap-2 rounded-full border border-border bg-muted/50 px-4 py-1.5 text-sm text-muted-foreground">
              <span className="inline-block h-2 w-2 rounded-full bg-emerald-500" />
              Open source &middot; Self-hosted &middot; Single binary
            </div>
            <h1 className="text-4xl font-bold tracking-tight sm:text-6xl">
              Secure remote access to every machine
            </h1>
            <p className="mt-6 text-lg leading-8 text-muted-foreground">
              No SSH keys. No VPNs. No inbound ports. Conduit gives you instant shell access, file
              management, and fleet visibility through a single self-hosted binary with
              post-quantum cryptography.
            </p>
            <div className="mt-10 flex items-center justify-center gap-x-6">
              <Link
                href="/login"
                className="rounded-md bg-primary px-6 py-3 text-sm font-semibold text-primary-foreground shadow-sm transition-colors hover:bg-primary/90"
              >
                Open Dashboard
              </Link>
              <a
                href="https://github.com/Appsynergy-io/conduit"
                target="_blank"
                rel="noopener noreferrer"
                className="text-sm font-semibold leading-6 text-muted-foreground transition-colors hover:text-foreground"
              >
                View Source <span aria-hidden="true">&rarr;</span>
              </a>
            </div>
          </div>
        </section>

        {/* Features Grid */}
        <section id="features" className="border-t border-border bg-muted/30 px-6 py-24">
          <div className="mx-auto max-w-6xl">
            <div className="text-center">
              <h2 className="text-3xl font-bold tracking-tight">Everything you need to manage remote machines</h2>
              <p className="mt-4 text-lg text-muted-foreground">
                One binary. One dashboard. Zero external dependencies.
              </p>
            </div>
            <div className="mt-16 grid grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3">
              <FeatureCard
                icon={<TerminalIcon />}
                title="Zero-Trust Shell"
                description="Passkey-authenticated terminal sessions over QUIC with full PTY support. Agents connect outbound — no inbound ports, no SSH, no VPN."
              />
              <FeatureCard
                icon={<FolderIcon />}
                title="File Management"
                description="Browse directories, upload, download, rename, and delete files on any connected machine through an intuitive browser interface."
              />
              <FeatureCard
                icon={<ActivityIcon />}
                title="Real-Time Dashboard"
                description="Live agent status, CPU/memory metrics, connection health, and session monitoring. WebSocket-powered EventBus — no polling."
              />
              <FeatureCard
                icon={<ShieldIcon />}
                title="Post-Quantum Cryptography"
                description="X25519MLKEM768 hybrid TLS key exchange. Ed25519 JWT signing. Argon2id password hashing. PQC-first — classical only when forced by external parties."
              />
              <FeatureCard
                icon={<BoxIcon />}
                title="Single Binary Deploy"
                description="One Go binary serves the dashboard, API, agent connections, and embedded SQLite database. No Redis, no Postgres, no Docker required."
              />
              <FeatureCard
                icon={<ScrollIcon />}
                title="Audit Everything"
                description="Every login, shell session, file operation, and configuration change is logged with who, what, when, source IP, and outcome. Immutable append-only logs."
              />
              <FeatureCard
                icon={<KeyIcon />}
                title="Passkey Authentication"
                description="WebAuthn/FIDO2 passkeys as the primary auth mechanism. Phishing-resistant, no passwords to manage. Recovery codes as backup."
              />
              <FeatureCard
                icon={<RecordIcon />}
                title="Session Recording"
                description="Every shell session is recorded in asciicast v2 format. Full playback capability for compliance, auditing, and incident review."
              />
              <FeatureCard
                icon={<WebhookIcon />}
                title="Webhooks & Events"
                description="Subscribe to agent connect/disconnect, auth events, and shell sessions. HMAC-SHA256 signed payloads with configurable retry."
              />
            </div>
          </div>
        </section>

        {/* How It Works */}
        <section className="border-t border-border px-6 py-24">
          <div className="mx-auto max-w-4xl">
            <div className="text-center">
              <h2 className="text-3xl font-bold tracking-tight">How it works</h2>
              <p className="mt-4 text-lg text-muted-foreground">Three steps to secure remote access.</p>
            </div>
            <div className="mt-16 grid grid-cols-1 gap-12 md:grid-cols-3">
              <Step
                number="1"
                title="Deploy the server"
                description="Run a single binary on any machine. It auto-provisions TLS via Let's Encrypt and serves the dashboard, API, and agent listener."
              />
              <Step
                number="2"
                title="Join your machines"
                description="Generate a join token and run 'conduit join' on each target. The agent installs as a system service, connects outbound, and stays connected."
              />
              <Step
                number="3"
                title="Access from anywhere"
                description="Open the dashboard, authenticate with your passkey, and get instant shell access or file management on any connected machine."
              />
            </div>
          </div>
        </section>

        {/* Quick Start */}
        <section id="quickstart" className="border-t border-border bg-muted/30 px-6 py-24">
          <div className="mx-auto max-w-3xl">
            <div className="text-center">
              <h2 className="text-3xl font-bold tracking-tight">Quick start</h2>
              <p className="mt-4 text-lg text-muted-foreground">Up and running in under a minute.</p>
            </div>
            <div className="mt-12 space-y-6">
              <CodeBlock
                title="Start the server"
                code={`# Download the latest release
curl -fsSL https://get.conduit.sh | sh

# Start in dev mode (self-signed TLS on port 8443)
conduit-server --dev`}
              />
              <CodeBlock
                title="Join an agent"
                code={`# On the target machine — uses the join token from the dashboard
conduit join https://your-server:8443 <join-token> --dev-insecure`}
              />
              <CodeBlock
                title="Or use the CLI"
                code={`# Direct shell access from your terminal
conduit shell web-01

# Or launch the TUI for interactive navigation
conduit`}
              />
            </div>
          </div>
        </section>

        {/* Security */}
        <section id="security" className="border-t border-border px-6 py-24">
          <div className="mx-auto max-w-4xl">
            <div className="text-center">
              <h2 className="text-3xl font-bold tracking-tight">Built for security teams</h2>
              <p className="mt-4 text-lg text-muted-foreground">
                NIST-compliant architecture with zero-trust principles throughout.
              </p>
            </div>
            <div className="mt-12 grid grid-cols-1 gap-6 sm:grid-cols-2">
              <ComplianceCard
                title="NIST SP 800-53 Rev. 5"
                items={["Access Control (AC)", "Audit & Accountability (AU)", "Identification & Auth (IA)", "System Protection (SC)"]}
              />
              <ComplianceCard
                title="OWASP Top 10 & API Top 10"
                items={["BOLA prevention on every handler", "No user enumeration", "RFC 9457 error responses", "Rate limiting & input validation"]}
              />
              <ComplianceCard
                title="NIST SP 800-63B (AAL2)"
                items={["WebAuthn/FIDO2 passkeys", "Argon2id credential hashing", "Recovery codes (single-use)", "Session management"]}
              />
              <ComplianceCard
                title="Zero Trust Architecture"
                items={["TLS 1.3 only — no downgrades", "Token validation at every boundary", "PQC hybrid key exchange", "All assets embedded — no CDN"]}
              />
            </div>
          </div>
        </section>

        {/* CTA */}
        <section className="border-t border-border bg-muted/30 px-6 py-24">
          <div className="mx-auto max-w-2xl text-center">
            <h2 className="text-3xl font-bold tracking-tight">Self-host Conduit today</h2>
            <p className="mt-4 text-lg text-muted-foreground">
              Open source, single binary, zero external dependencies. Deploy on your infrastructure
              and own your remote access stack.
            </p>
            <div className="mt-10 flex items-center justify-center gap-x-6">
              <a
                href="https://github.com/Appsynergy-io/conduit"
                target="_blank"
                rel="noopener noreferrer"
                className="rounded-md bg-primary px-6 py-3 text-sm font-semibold text-primary-foreground shadow-sm transition-colors hover:bg-primary/90"
              >
                Get Started on GitHub
              </a>
              <Link
                href="/login"
                className="text-sm font-semibold leading-6 text-muted-foreground transition-colors hover:text-foreground"
              >
                Open Dashboard <span aria-hidden="true">&rarr;</span>
              </Link>
            </div>
          </div>
        </section>
      </main>

      <footer className="border-t border-border py-8">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">Conduit</span>
            <span className="text-xs text-muted-foreground">Community Edition</span>
          </div>
          <div className="flex items-center gap-6 text-sm text-muted-foreground">
            <a
              href="https://github.com/Appsynergy-io/conduit"
              target="_blank"
              rel="noopener noreferrer"
              className="transition-colors hover:text-foreground"
            >
              GitHub
            </a>
            <a
              href="https://github.com/Appsynergy-io/conduit/issues"
              target="_blank"
              rel="noopener noreferrer"
              className="transition-colors hover:text-foreground"
            >
              Issues
            </a>
          </div>
        </div>
      </footer>
    </div>
  )
}

function FeatureCard({ icon, title, description }: { icon: React.ReactNode; title: string; description: string }) {
  return (
    <div className="group rounded-xl border border-border bg-card p-6 transition-colors hover:border-primary/30 hover:bg-accent/50">
      <div className="mb-3 inline-flex h-10 w-10 items-center justify-center rounded-lg bg-primary/10 text-primary">
        {icon}
      </div>
      <h3 className="font-semibold">{title}</h3>
      <p className="mt-2 text-sm leading-relaxed text-muted-foreground">{description}</p>
    </div>
  )
}

function Step({ number, title, description }: { number: string; title: string; description: string }) {
  return (
    <div className="text-center">
      <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-primary text-lg font-bold text-primary-foreground">
        {number}
      </div>
      <h3 className="mt-4 font-semibold">{title}</h3>
      <p className="mt-2 text-sm text-muted-foreground">{description}</p>
    </div>
  )
}

function CodeBlock({ title, code }: { title: string; code: string }) {
  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <div className="border-b border-border bg-muted px-4 py-2 text-sm font-medium">{title}</div>
      <pre className="overflow-x-auto bg-card p-4 text-sm text-muted-foreground">
        <code>{code}</code>
      </pre>
    </div>
  )
}

function ComplianceCard({ title, items }: { title: string; items: string[] }) {
  return (
    <div className="rounded-xl border border-border bg-card p-6">
      <h3 className="font-semibold">{title}</h3>
      <ul className="mt-3 space-y-2">
        {items.map((item) => (
          <li key={item} className="flex items-start gap-2 text-sm text-muted-foreground">
            <svg className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12.75l6 6 9-13.5" />
            </svg>
            {item}
          </li>
        ))}
      </ul>
    </div>
  )
}

// Icons — inline SVGs to avoid external dependencies (NIST SP 800-218: all assets embedded)

function TerminalIcon() {
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="m6.75 7.5 3 2.25-3 2.25m4.5 0h3m-9 8.25h13.5A2.25 2.25 0 0 0 21 18V6a2.25 2.25 0 0 0-2.25-2.25H5.25A2.25 2.25 0 0 0 3 6v12a2.25 2.25 0 0 0 2.25 2.25Z" />
    </svg>
  )
}

function FolderIcon() {
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 12.75V12A2.25 2.25 0 0 1 4.5 9.75h15A2.25 2.25 0 0 1 21.75 12v.75m-8.69-6.44-2.12-2.12a1.5 1.5 0 0 0-1.061-.44H4.5A2.25 2.25 0 0 0 2.25 6v12a2.25 2.25 0 0 0 2.25 2.25h15A2.25 2.25 0 0 0 21.75 18V9a2.25 2.25 0 0 0-2.25-2.25h-5.379a1.5 1.5 0 0 1-1.06-.44Z" />
    </svg>
  )
}

function ActivityIcon() {
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 0 1 3 19.875v-6.75ZM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V8.625ZM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 0 1-1.125-1.125V4.125Z" />
    </svg>
  )
}

function ShieldIcon() {
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75 11.25 15 15 9.75m-3-7.036A11.959 11.959 0 0 1 3.598 6 11.99 11.99 0 0 0 3 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285Z" />
    </svg>
  )
}

function BoxIcon() {
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="m21 7.5-9-5.25L3 7.5m18 0-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-9v9" />
    </svg>
  )
}

function ScrollIcon() {
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
    </svg>
  )
}

function KeyIcon() {
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M7.864 4.243A7.5 7.5 0 0 1 19.5 10.5c0 2.92-.556 5.709-1.568 8.268M5.742 6.364A7.465 7.465 0 0 0 4.5 10.5a7.464 7.464 0 0 1-1.15 3.993m1.989 3.559A11.209 11.209 0 0 0 8.25 10.5a3.75 3.75 0 1 1 7.5 0c0 .527-.021 1.049-.064 1.565M12 10.5a14.94 14.94 0 0 1-3.6 9.75m6.633-4.596a18.666 18.666 0 0 1-2.485 5.33" />
    </svg>
  )
}

function RecordIcon() {
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="m15.75 10.5 4.72-4.72a.75.75 0 0 1 1.28.53v11.38a.75.75 0 0 1-1.28.53l-4.72-4.72M4.5 18.75h9a2.25 2.25 0 0 0 2.25-2.25v-9a2.25 2.25 0 0 0-2.25-2.25h-9A2.25 2.25 0 0 0 2.25 7.5v9a2.25 2.25 0 0 0 2.25 2.25Z" />
    </svg>
  )
}

function WebhookIcon() {
  return (
    <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
      <path strokeLinecap="round" strokeLinejoin="round" d="M14.25 9.75 16.5 12l-2.25 2.25m-4.5 0L7.5 12l2.25-2.25M6 20.25h12A2.25 2.25 0 0 0 20.25 18V6A2.25 2.25 0 0 0 18 3.75H6A2.25 2.25 0 0 0 3.75 6v12A2.25 2.25 0 0 0 6 20.25Z" />
    </svg>
  )
}
