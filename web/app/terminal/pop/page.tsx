"use client"

import { AlertCircle } from "lucide-react"
import { useSearchParams } from "next/navigation"
import { Suspense, useCallback, useEffect, useState } from "react"
import { TerminalView } from "@/components/terminal"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { useAuth } from "@/hooks/use-auth"

interface SessionInfo {
  id: string
  agentId: string
  status: string
  pinned: number
}

/**
 * Pop-out terminal page — minimal chrome, no sidebar, full-screen terminal.
 * Opened via window.open() from the main terminal view.
 * Reattaches to an existing session by sessionId from the URL query param.
 *
 * Closing this window = detach (session stays alive), not terminate.
 * Refreshing the page reconnects via the session query param in the URL.
 */
function PopOutTerminalContent() {
  const searchParams = useSearchParams()
  const sessionId = searchParams.get("session")
  const { isAuthenticated } = useAuth()
  const [session, setSession] = useState<SessionInfo | null>(null)
  const [agentHostname, setAgentHostname] = useState<string | undefined>()
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!isAuthenticated || !sessionId) {
      setLoading(false)
      if (!sessionId) setError("No session specified")
      return
    }

    async function fetchSession() {
      try {
        const res = await fetch(`/api/v1/shell/sessions?status=all`)
        if (!res.ok) {
          setError("Failed to load session")
          return
        }
        const data = await res.json()
        const found = (data.data as SessionInfo[])?.find(
          (s: SessionInfo) => s.id === sessionId,
        )
        if (!found) {
          setError("Session not found")
          return
        }
        if (found.status === "closed") {
          setError("Session has ended")
          return
        }
        setSession(found)

        // Fetch agent hostname for display
        try {
          const agentRes = await fetch(
            `/api/v1/agents/${encodeURIComponent(found.agentId)}`,
          )
          if (agentRes.ok) {
            const agent = await agentRes.json()
            setAgentHostname(agent.displayName ?? agent.hostname)
          }
        } catch {
          // Agent hostname is optional for display
        }
      } catch {
        setError("Failed to connect to server")
      } finally {
        setLoading(false)
      }
    }

    fetchSession()
  }, [isAuthenticated, sessionId])

  // Set window title
  useEffect(() => {
    const name = agentHostname ?? sessionId?.slice(0, 8) ?? "Terminal"
    document.title = `${name} — Conduit Terminal`
  }, [agentHostname, sessionId])

  // BroadcastChannel coordination with parent window
  const handleSessionReady = useCallback(
    (sid: string) => {
      try {
        const bc = new BroadcastChannel("conduit-terminal")
        bc.postMessage({ type: "pop-attached", sessionId: sid })
        bc.close()
      } catch {
        // BroadcastChannel not supported
      }
    },
    [],
  )

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center bg-[#09090b]">
        <div className="text-sm text-zinc-500">Loading session...</div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex h-screen items-center justify-center bg-background p-4">
        <Alert variant="destructive" className="max-w-md">
          <AlertCircle />
          <AlertTitle>{error}</AlertTitle>
          <AlertDescription>This window can be closed.</AlertDescription>
        </Alert>
      </div>
    )
  }

  if (!session) {
    return null
  }

  return (
    <div className="flex h-screen flex-col">
      <TerminalView
        agentId={session.agentId}
        agentHostname={agentHostname}
        sessionId={session.id}
        onSessionReady={handleSessionReady}
      />
    </div>
  )
}

export default function PopOutTerminalPage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-screen items-center justify-center bg-[#09090b]">
          <div className="text-sm text-zinc-500">Loading...</div>
        </div>
      }
    >
      <PopOutTerminalContent />
    </Suspense>
  )
}
