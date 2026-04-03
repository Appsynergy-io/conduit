"use client"

import { Monitor, Pin, Plus, Square, RefreshCw } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"

interface ShellSession {
  id: string
  agentId: string
  userId: string
  status: "active" | "detached" | "closed"
  pinned: number
  idleTimeout: number
  cols: number | null
  rows: number | null
  createdAt: string
  detachedAt: string | null
  closedAt: string | null
}

interface SessionListProps {
  /** If provided, only show sessions for this agent */
  agentId?: string
  /** ID of the session currently displayed in the terminal */
  activeSessionId?: string
  /** Called when user clicks a session to attach/switch to it */
  onAttach?: (session: ShellSession) => void
  /** Called when a session is terminated */
  onTerminate?: (sessionId: string) => void
  /** Called when user clicks "New session" */
  onNewSession?: () => void
}

export function SessionList({ agentId, activeSessionId, onAttach, onTerminate, onNewSession }: SessionListProps) {
  const [sessions, setSessions] = useState<ShellSession[]>([])
  const [loading, setLoading] = useState(true)

  const fetchSessions = useCallback(async () => {
    try {
      const url = agentId
        ? `/api/v1/agents/${encodeURIComponent(agentId)}/shell/sessions`
        : "/api/v1/shell/sessions"
      const res = await fetch(url)
      if (res.ok) {
        const data = await res.json()
        setSessions(data.data ?? [])
      }
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [agentId])

  useEffect(() => {
    fetchSessions()
    const interval = setInterval(fetchSessions, 5000)
    return () => clearInterval(interval)
  }, [fetchSessions])

  const handleTerminate = useCallback(
    async (session: ShellSession) => {
      try {
        await fetch(
          `/api/v1/agents/${encodeURIComponent(session.agentId)}/shell/sessions/${encodeURIComponent(session.id)}`,
          { method: "DELETE" },
        )
        setSessions((prev) => prev.filter((s) => s.id !== session.id))
        onTerminate?.(session.id)
      } catch {
        // ignore
      }
    },
    [onTerminate],
  )

  if (loading) {
    return (
      <div className="text-sm text-muted-foreground p-3">Loading sessions...</div>
    )
  }

  if (sessions.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 p-4 text-center">
        <p className="text-sm text-muted-foreground">No active sessions</p>
        {onNewSession && (
          <Button variant="outline" size="sm" onClick={onNewSession}>
            <Plus className="mr-1.5 h-3.5 w-3.5" />
            New session
          </Button>
        )}
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between px-3 py-1.5">
        <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
          Sessions ({sessions.length})
        </span>
        <div className="flex items-center gap-0.5">
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button variant="ghost" size="icon" className="h-6 w-6" onClick={onNewSession}>
                  <Plus className="h-3 w-3" />
                </Button>
              </TooltipTrigger>
              <TooltipContent>New session</TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <Button variant="ghost" size="icon" className="h-6 w-6" onClick={fetchSessions}>
            <RefreshCw className="h-3 w-3" />
          </Button>
        </div>
      </div>
      {sessions.map((session) => (
        <SessionItem
          key={session.id}
          session={session}
          isActive={session.id === activeSessionId}
          onAttach={onAttach}
          onTerminate={handleTerminate}
        />
      ))}
    </div>
  )
}

function SessionItem({
  session,
  isActive,
  onAttach,
  onTerminate,
}: {
  session: ShellSession
  isActive: boolean
  onAttach?: (session: ShellSession) => void
  onTerminate: (session: ShellSession) => void
}) {
  const isDetached = session.status === "detached"
  const isPinned = session.pinned === 1
  const timeAgo = formatTimeAgo(session.createdAt)

  return (
    <div
      className={`flex items-center gap-2 rounded-md px-3 py-2 group ${
        isActive ? "bg-muted" : "hover:bg-muted/50 cursor-pointer"
      }`}
      onClick={() => {
        if (!isActive && onAttach) {
          onAttach(session)
        }
      }}
    >
      <Monitor className="h-4 w-4 shrink-0 text-muted-foreground" />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-1.5">
          <span className="text-xs font-mono truncate">{session.id.slice(0, 8)}</span>
          {isDetached ? (
            <Badge variant="secondary" className="text-[10px] px-1 py-0 bg-yellow-600/20 text-yellow-600">
              detached
            </Badge>
          ) : (
            <Badge variant="secondary" className="text-[10px] px-1 py-0 bg-green-600/20 text-green-600">
              active
            </Badge>
          )}
          {isPinned && (
            <Pin className="h-3 w-3 text-muted-foreground" />
          )}
        </div>
        <span className="text-[10px] text-muted-foreground">{timeAgo}</span>
      </div>
      <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="ghost"
                size="icon"
                className="h-6 w-6 text-destructive"
                onClick={(e) => {
                  e.stopPropagation()
                  onTerminate(session)
                }}
              >
                <Square className="h-3 w-3" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>Terminate</TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>
    </div>
  )
}

function formatTimeAgo(dateStr: string): string {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return "just now"
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  const diffDay = Math.floor(diffHr / 24)
  return `${diffDay}d ago`
}
