"use client"

import { AlertCircle, ExternalLink, Hand, Pin, PinOff, WifiOff, X } from "lucide-react"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useCallback, useEffect, useRef, useState } from "react"
import { SessionTabs } from "@/components/session-tabs"
import { type TerminalHandle, TerminalView } from "@/components/terminal"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"
import { useAuth } from "@/hooks/use-auth"

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
  const modeParam = searchParams.get("mode")
  const connectMode = modeParam === "watch" ? "watch" : ("control" as "control" | "watch")
  const [agent, setAgent] = useState<Agent | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(attachSessionId)
  // Track the resolved session ID (existing session to reuse, or null to create new)
  const [resolvedSessionId, setResolvedSessionId] = useState<string | undefined>(
    attachSessionId ?? undefined,
  )
  const [newSessionCounter, setNewSessionCounter] = useState(0)
  const resolvedRef = useRef(false)
  const terminalRef = useRef<TerminalHandle>(null)
  const [role, setRole] = useState<"controller" | "watcher">(
    connectMode === "watch" ? "watcher" : "controller",
  )

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
            const sessRes = await fetch(`/api/v1/agents/${encodeURIComponent(id)}/shell/sessions`, {
              signal: controller.signal,
            })
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
      // Optimistically assume controller until the server's session
      // message corrects us (e.g. to watcher if someone else holds control).
      setRole(connectMode === "watch" ? "watcher" : "controller")
      // Update URL to reflect the attached session
      router.replace(
        `/dashboard/terminal?agent=${encodeURIComponent(session.agentId)}&session=${encodeURIComponent(session.id)}`,
      )
    },
    [connectMode, router],
  )

  const [pinned, setPinned] = useState(false)

  const handleSessionReady = useCallback((sessionId: string) => {
    setCurrentSessionId(sessionId)
    setPinned(false)
  }, [])

  const handleNewSession = useCallback(() => {
    setResolvedSessionId(undefined)
    setCurrentSessionId(null)
    setPinned(false)
    setRole("controller")
    setNewSessionCounter((c) => c + 1)
    router.replace(`/dashboard/terminal?agent=${encodeURIComponent(agentId!)}`)
  }, [agentId, router])

  const togglePin = useCallback(async () => {
    if (!currentSessionId || !agentId) return
    try {
      const res = await fetch(
        `/api/v1/agents/${encodeURIComponent(agentId)}/shell/sessions/${encodeURIComponent(currentSessionId)}`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ pinned: !pinned }),
        },
      )
      if (res.ok) setPinned(!pinned)
    } catch {
      // ignore
    }
  }, [currentSessionId, agentId, pinned])

  const popOut = useCallback(() => {
    if (!currentSessionId) return
    const url = `/terminal/pop?session=${encodeURIComponent(currentSessionId)}`
    window.open(
      url,
      `conduit-terminal-${currentSessionId}`,
      "width=900,height=600,menubar=no,toolbar=no",
    )
  }, [currentSessionId])

  const handleTakeControl = useCallback(() => {
    terminalRef.current?.takeControl()
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
    <div className="flex h-[calc(100vh-7rem)] flex-col overflow-hidden rounded-lg border">
      {/* Tab bar */}
      <div className="flex items-center justify-between border-b bg-card px-2">
        <div className="flex items-center gap-2 overflow-hidden">
          <span className="shrink-0 text-sm font-medium pl-2 text-muted-foreground">
            {agent?.displayName ?? agent?.hostname}
          </span>
          <span className="text-border">|</span>
          <SessionTabs
            agentId={agentId}
            activeSessionId={currentSessionId ?? undefined}
            onSelect={handleAttach}
            onNewSession={handleNewSession}
          />
        </div>
        <div className="flex items-center gap-0.5 shrink-0 pl-2">
          {role === "watcher" && currentSessionId && (
            <Button
              variant="outline"
              size="sm"
              onClick={handleTakeControl}
              aria-label="Take control"
              className="mr-1 h-7 px-2 text-xs sm:mr-2 sm:px-3"
            >
              <Hand className="h-3.5 w-3.5 sm:mr-1.5" />
              <span className="hidden sm:inline">Take Control</span>
            </Button>
          )}
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={togglePin}
                  disabled={!currentSessionId || role === "watcher"}
                  className="h-7 w-7 text-muted-foreground hover:text-foreground"
                >
                  {pinned ? <PinOff className="h-3.5 w-3.5" /> : <Pin className="h-3.5 w-3.5" />}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                {pinned ? "Unpin (allow idle timeout)" : "Pin (keep alive forever)"}
              </TooltipContent>
            </Tooltip>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={popOut}
                  disabled={!currentSessionId}
                  className="h-7 w-7 text-muted-foreground hover:text-foreground"
                >
                  <ExternalLink className="h-3.5 w-3.5" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>Pop out to new window</TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <Button
            variant="ghost"
            size="icon"
            onClick={handleClose}
            className="h-7 w-7 text-muted-foreground hover:text-foreground"
          >
            <X className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Terminal */}
      <div className="flex-1 overflow-hidden">
        <TerminalView
          key={resolvedSessionId ?? `new-${newSessionCounter}`}
          ref={terminalRef}
          agentId={agentId}
          agentHostname={agent?.displayName ?? agent?.hostname}
          sessionId={resolvedSessionId}
          mode={connectMode}
          onClose={handleClose}
          onSessionReady={handleSessionReady}
          onRoleChange={setRole}
          hideHeader
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
