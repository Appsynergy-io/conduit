import Link from "next/link"

export default function Home() {
  return (
    <div className="flex min-h-screen flex-col">
      <header className="border-b border-border">
        <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-6">
          <div className="flex items-center gap-2">
            <span className="text-xl font-bold tracking-tight">Conduit</span>
            <span className="rounded-md bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
              CE
            </span>
          </div>
          <Link
            href="/login"
            className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
          >
            Sign in
          </Link>
        </div>
      </header>

      <main className="flex flex-1 flex-col items-center justify-center px-6 py-24">
        <div className="mx-auto max-w-3xl text-center">
          <h1 className="text-4xl font-bold tracking-tight sm:text-6xl">
            Secure remote access to every machine
          </h1>
          <p className="mt-6 text-lg leading-8 text-muted-foreground">
            No SSH keys. No VPNs. No inbound ports. Conduit gives you instant shell access, file
            management, and fleet visibility through a single self-hosted binary.
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

        <div className="mx-auto mt-24 grid max-w-5xl grid-cols-1 gap-8 sm:grid-cols-2 lg:grid-cols-3">
          <FeatureCard
            title="Zero-Trust Shell"
            description="Passkey-authenticated terminal sessions over QUIC. Full PTY with resize, no agent inbound ports."
          />
          <FeatureCard
            title="File Management"
            description="Browse, upload, download, and manage files on any connected machine through your browser."
          />
          <FeatureCard
            title="Real-Time Dashboard"
            description="Live agent status, session monitoring, and audit logging. WebSocket-powered, no polling."
          />
          <FeatureCard
            title="Post-Quantum Ready"
            description="X25519MLKEM768 hybrid TLS key exchange. Ed25519 JWT signing. PQC-first cryptography."
          />
          <FeatureCard
            title="Single Binary"
            description="One binary serves the dashboard, API, and agent connections. SQLite database. Zero external dependencies."
          />
          <FeatureCard
            title="Audit Everything"
            description="Every login, shell session, file operation, and config change logged with who, what, when, and outcome."
          />
        </div>
      </main>

      <footer className="border-t border-border py-8 text-center text-sm text-muted-foreground">
        Conduit Community Edition
      </footer>
    </div>
  )
}

function FeatureCard({ title, description }: { title: string; description: string }) {
  return (
    <div className="rounded-xl border border-border bg-card p-6">
      <h3 className="font-semibold">{title}</h3>
      <p className="mt-2 text-sm text-muted-foreground">{description}</p>
    </div>
  )
}
