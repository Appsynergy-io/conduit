"use client"

import { AlertCircle, WifiOff } from "lucide-react"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useCallback, useRef, useState } from "react"
import { SessionList } from "@/components/session-list"
import { TerminalView } from "@/components/terminal"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuth } from "@/hooks/use-auth"
import { useEffect } from "react"

interface Agent {
  id: string
  hostname: string
  displayName?: string
  status: string
}

interface ShellSession {
  id: string
  agentId: string
  status: string
}

function TerminalContent() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const { isAuthenticated } = useAuth()
  const agentId = searchParams.get("agent")
  const attachSessionId = searchParams.get("session")
  const [agent, setAgent] = useState<Agent | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(attachSessionId)
  // Track the resolved session ID (existing session to reuse, or null to create new)
  const [resolvedSessionId, setResolvedSessionId] = useState<string | undefined>(attachSessionId ?? undefined)
  const resolvedRef = useRef(false)

  useEffect(() => {
    if (!agentId) {
      setLoading(false)
      return
    }
    // Wait for auth check to complete before fetching agent
    if (!isAuthenticated) return

    const id = agentId
    const controller = new AbortController()

    async function fetchAgentAndSessions() {
      try {
        const res = await fetch(`/api/v1/agents/${encodeURIComponent(id)}`, {
          signal: controller.signal,
        })

        if (!res.ok) {
          if (res.status === 404) {
            setError("Agent not found")
          } else if (res.status === 401) {
            setError("Unauthorized")
          } else {
            setError("Failed to load agent")
          }
          return
        }

        const data = await res.json()
        setAgent(data)

        // If no session was explicitly requested, check for existing sessions to reuse
        if (!attachSessionId && !resolvedRef.current) {
          resolvedRef.current = true
          try {
            const sessRes = await fetch(
              `/api/v1/agents/${encodeURIComponent(id)}/shell/sessions`,
              { signal: controller.signal },
            )
            if (sessRes.ok) {
              const sessData = await sessRes.json()
              const sessions: ShellSession[] = sessData.data ?? []
              // Prefer active session, then most recent detached
              const active = sessions.find((s) => s.status === "active")
              const detached = sessions.find((s) => s.status === "detached")
              const existing = active ?? detached
              if (existing) {
                setResolvedSessionId(existing.id)
              }
            }
          } catch {
            // If session fetch fails, just create a new session
          }
        }
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return
        setError("Failed to connect to server")
      } finally {
        setLoading(false)
      }
    }

    fetchAgentAndSessions()
    return () => controller.abort()
  }, [isAuthenticated, agentId, attachSessionId])

  const handleClose = useCallback(() => {
    router.push("/dashboard")
  }, [router])

  const handleAttach = useCallback(
    (session: { id: string; agentId: string }) => {
      setResolvedSessionId(session.id)
      setCurrentSessionId(session.id)
      // Update URL to reflect the attached session
      router.replace(
        `/dashboard/terminal?agent=${encodeURIComponent(session.agentId)}&session=${encodeURIComponent(session.id)}`,
      )
    },
    [router],
  )

  const handleSessionReady = useCallback((sessionId: string) => {
    setCurrentSessionId(sessionId)
  }, [])

  if (!agentId) {
    return (
      <Alert className="mx-auto max-w-lg mt-12">
        <AlertCircle />
        <AlertTitle>No agent selected</AlertTitle>
        <AlertDescription>
          Select an agent from the dashboard to open a terminal session.
        </AlertDescription>
      </Alert>
    )
  }

  if (loading) {
    return (
      <div className="flex h-[calc(100vh-7rem)] flex-col gap-3">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="flex-1 w-full" />
      </div>
    )
  }

  if (error) {
    return (
      <Alert variant="destructive" className="mx-auto max-w-lg mt-12">
        <AlertCircle />
        <AlertTitle>{error}</AlertTitle>
        <AlertDescription>Return to the dashboard and try again.</AlertDescription>
      </Alert>
    )
  }

  if (!agent) {
    return null
  }

  if (agent.status !== "online") {
    return (
      <Alert className="mx-auto max-w-lg mt-12">
        <WifiOff />
        <AlertTitle>Agent is offline</AlertTitle>
        <AlertDescription>
          {agent.displayName ?? agent.hostname} is not currently connected.
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="flex h-[calc(100vh-7rem)] gap-4">
      {/* Session list sidebar */}
      <div className="w-64 shrink-0 overflow-y-auto rounded-lg border bg-card">
        <SessionList agentId={agentId} activeSessionId={currentSessionId ?? undefined} onAttach={handleAttach} />
      </div>

      {/* Terminal */}
      <div className="flex-1 overflow-hidden rounded-lg border">
        <TerminalView
          key={resolvedSessionId ?? "new"}
          agentId={agentId}
          agentHostname={agent?.displayName ?? agent?.hostname}
          sessionId={resolvedSessionId}
          onClose={handleClose}
          onSessionReady={handleSessionReady}
        />
      </div>
    </div>
  )
}

export default function TerminalPage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-[calc(100vh-7rem)] flex-col gap-3">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="flex-1 w-full" />
        </div>
      }
    >
      <TerminalContent />
    </Suspense>
  )
}
