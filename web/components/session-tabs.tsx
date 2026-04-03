"use client"

import { Circle, Pin, Plus, X } from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
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

interface SessionTabsProps {
  agentId: string
  activeSessionId?: string
  onSelect: (session: ShellSession) => void
  onNewSession: () => void
  onTerminate?: (sessionId: string) => void
}

export function SessionTabs({
  agentId,
  activeSessionId,
  onSelect,
  onNewSession,
  onTerminate,
}: SessionTabsProps) {
  const [sessions, setSessions] = useState<ShellSession[]>([])
  const scrollRef = useRef<HTMLDivElement>(null)

  const fetchSessions = useCallback(async () => {
    try {
      const res = await fetch(
        `/api/v1/agents/${encodeURIComponent(agentId)}/shell/sessions`,
      )
      if (res.ok) {
        const data = await res.json()
        setSessions(data.data ?? [])
      }
    } catch {
      // ignore
    }
  }, [agentId])

  useEffect(() => {
    fetchSessions()
    const interval = setInterval(fetchSessions, 5000)
    return () => clearInterval(interval)
  }, [fetchSessions])

  const handleTerminate = useCallback(
    async (e: React.MouseEvent, session: ShellSession) => {
      e.stopPropagation()
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

  return (
    <div className="flex items-center gap-0 overflow-hidden" role="tablist">
      <div
        ref={scrollRef}
        className="flex items-center gap-0.5 overflow-x-auto scrollbar-none"
      >
        {sessions.map((session) => {
          const isActive = session.id === activeSessionId
          const isDetached = session.status === "detached"
          const isPinned = session.pinned === 1

          return (
            <button
              key={session.id}
              role="tab"
              aria-selected={isActive}
              onClick={() => {
                if (!isActive) onSelect(session)
              }}
              className={`group relative flex items-center gap-1.5 whitespace-nowrap rounded-t-md px-3 py-1.5 text-xs font-mono transition-colors ${
                isActive
                  ? "bg-[#09090b] text-zinc-100"
                  : "text-muted-foreground hover:bg-muted/60 hover:text-foreground"
              }`}
            >
              <Circle
                className={`h-2 w-2 shrink-0 fill-current ${
                  isDetached
                    ? "text-yellow-500"
                    : "text-green-500"
                }`}
              />
              <span>{session.id.slice(0, 8)}</span>
              {isPinned && (
                <Pin className="h-2.5 w-2.5 shrink-0 text-muted-foreground" />
              )}
              <span
                role="button"
                tabIndex={-1}
                onClick={(e) => handleTerminate(e, session)}
                className={`ml-0.5 shrink-0 rounded p-0.5 hover:bg-zinc-700/60 ${
                  isActive
                    ? "text-zinc-400 hover:text-zinc-200"
                    : "opacity-0 group-hover:opacity-100 text-muted-foreground hover:text-foreground"
                }`}
              >
                <X className="h-3 w-3" />
              </span>
              {isActive && (
                <span className="absolute bottom-0 left-1 right-1 h-px bg-primary" />
              )}
            </button>
          )
        })}
      </div>

      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="h-7 w-7 shrink-0 ml-0.5 text-muted-foreground hover:text-foreground"
              onClick={onNewSession}
            >
              <Plus className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>New session</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  )
}
