"use client"

import { ExternalLink, Eye, Pin, PinOff, X } from "lucide-react"
import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/components/ui/tooltip"

type TerminalStatus = "connecting" | "connected" | "disconnected" | "error" | "detached"
type SessionRole = "controller" | "watcher"

interface TerminalProps {
  agentId: string
  agentHostname?: string
  /** If provided, attaches to an existing session instead of creating one */
  sessionId?: string
  /** Connection mode: "control" (default) or "watch" (read-only) */
  mode?: "control" | "watch"
  onClose?: () => void
  /** Called when session is created or attached, provides session ID for other controls */
  onSessionReady?: (sessionId: string) => void
  /** Called whenever the server-granted role changes (controller/watcher) */
  onRoleChange?: (role: SessionRole) => void
}

/** Imperative methods exposed on the TerminalView ref. */
export interface TerminalHandle {
  /** Request control of the session from the current controller. */
  takeControl: () => void
}

export const TerminalView = forwardRef<TerminalHandle, TerminalProps & { hideHeader?: boolean }>(
  function TerminalView(
    {
      agentId,
      agentHostname,
      sessionId,
      mode = "control",
      onClose,
      onSessionReady,
      onRoleChange,
      hideHeader,
    },
    ref,
  ) {
    const termRef = useRef<HTMLDivElement>(null)
    const wsRef = useRef<WebSocket | null>(null)
    const xtermRef = useRef<import("@xterm/xterm").Terminal | null>(null)
    const fitAddonRef = useRef<import("@xterm/addon-fit").FitAddon | null>(null)
    const [status, setStatus] = useState<TerminalStatus>("connecting")
    const [pinned, setPinned] = useState(false)
    const [activeSessionId, setActiveSessionId] = useState<string | null>(sessionId ?? null)
    const [role, setRole] = useState<SessionRole>(mode === "watch" ? "watcher" : "controller")
    const [watcherCount, setWatcherCount] = useState(0)
    const roleRef = useRef<SessionRole>(mode === "watch" ? "watcher" : "controller")
    const onRoleChangeRef = useRef(onRoleChange)
    useEffect(() => {
      onRoleChangeRef.current = onRoleChange
    }, [onRoleChange])

    const updateRole = useCallback((next: SessionRole) => {
      setRole(next)
      roleRef.current = next
      onRoleChangeRef.current?.(next)
    }, [])

    const cleanup = useCallback(() => {
      if (wsRef.current) {
        wsRef.current.close(1000, "session ended")
        wsRef.current = null
      }
      if (xtermRef.current) {
        xtermRef.current.dispose()
        xtermRef.current = null
      }
      fitAddonRef.current = null
    }, [])

    const togglePin = useCallback(async () => {
      if (!activeSessionId) return
      try {
        const res = await fetch(
          `/api/v1/agents/${encodeURIComponent(agentId)}/shell/sessions/${encodeURIComponent(activeSessionId)}`,
          {
            method: "PATCH",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ pinned: !pinned }),
          },
        )
        if (res.ok) {
          setPinned(!pinned)
        }
      } catch {
        // ignore
      }
    }, [activeSessionId, agentId, pinned])

    const popOut = useCallback(() => {
      if (!activeSessionId) return
      const url = `/terminal/pop?session=${encodeURIComponent(activeSessionId)}`
      window.open(
        url,
        `conduit-terminal-${activeSessionId}`,
        "width=900,height=600,menubar=no,toolbar=no",
      )
    }, [activeSessionId])

    useEffect(() => {
      if (!termRef.current) return

      const container = termRef.current
      let disposed = false

      async function init() {
        const { Terminal } = await import("@xterm/xterm")
        const { FitAddon } = await import("@xterm/addon-fit")

        if (disposed) return

        const fitAddon = new FitAddon()
        const term = new Terminal({
          cursorBlink: true,
          fontFamily:
            "'JetBrains Mono', 'Fira Code', 'Cascadia Code', Menlo, Monaco, 'Courier New', monospace",
          fontSize: 14,
          theme: {
            background: "#09090b",
            foreground: "#fafafa",
            cursor: "#fafafa",
            selectionBackground: "#27272a",
            black: "#09090b",
            red: "#ef4444",
            green: "#22c55e",
            yellow: "#eab308",
            blue: "#3b82f6",
            magenta: "#a855f7",
            cyan: "#06b6d4",
            white: "#fafafa",
            brightBlack: "#71717a",
            brightRed: "#f87171",
            brightGreen: "#4ade80",
            brightYellow: "#facc15",
            brightBlue: "#60a5fa",
            brightMagenta: "#c084fc",
            brightCyan: "#22d3ee",
            brightWhite: "#ffffff",
          },
          allowProposedApi: true,
          scrollback: 10000,
          convertEol: true,
        })

        term.loadAddon(fitAddon)
        term.open(container)
        fitAddon.fit()

        xtermRef.current = term
        fitAddonRef.current = fitAddon

        // Build WebSocket URL
        const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:"
        const { cols, rows } = term

        let wsUrl: string
        if (sessionId) {
          // Reattach to existing session
          const wsMode = mode === "watch" ? "watch" : "control"
          if (mode === "watch") {
            // Use dedicated watch endpoint
            wsUrl = `${wsProtocol}//${window.location.host}/api/v1/agents/${encodeURIComponent(agentId)}/shell/sessions/${encodeURIComponent(sessionId)}/watch`
          } else {
            wsUrl = `${wsProtocol}//${window.location.host}/api/v1/agents/${encodeURIComponent(agentId)}/shell/sessions/${encodeURIComponent(sessionId)}/ws?mode=${wsMode}`
          }
        } else {
          // Create new session (legacy path, handled by server)
          wsUrl = `${wsProtocol}//${window.location.host}/api/v1/agents/${encodeURIComponent(agentId)}/shell/new?cols=${cols}&rows=${rows}`
        }

        const ws = new WebSocket(wsUrl, "conduit-shell-v1")
        ws.binaryType = "arraybuffer"
        wsRef.current = ws

        ws.onopen = () => {
          if (disposed) return
          setStatus("connected")
          term.focus()
        }

        ws.onmessage = (event) => {
          if (disposed) return
          if (event.data instanceof ArrayBuffer) {
            term.write(new Uint8Array(event.data))
          } else if (typeof event.data === "string") {
            try {
              const msg = JSON.parse(event.data)
              switch (msg.type) {
                case "session":
                  if (msg.sessionId) {
                    setActiveSessionId(msg.sessionId)
                    onSessionReady?.(msg.sessionId)
                  }
                  if (msg.role) {
                    // If the user asked for control but the server granted
                    // watcher (someone else already holds control), surface it
                    // so they understand why input is disabled.
                    if (
                      mode !== "watch" &&
                      msg.role === "watcher" &&
                      roleRef.current !== "watcher"
                    ) {
                      term.write(
                        `\r\n\x1b[93m--- Another user has control. Click "Take Control" to take over. ---\x1b[0m\r\n`,
                      )
                    }
                    updateRole(msg.role)
                  }
                  break
                case "detached":
                  setStatus("detached")
                  term.write("\r\n\x1b[90m--- Session detached ---\x1b[0m\r\n")
                  break
                case "control_transferred":
                  updateRole("watcher")
                  term.write(`\r\n\x1b[93m--- Control transferred to another user ---\x1b[0m\r\n`)
                  break
                case "control_granted":
                  updateRole("controller")
                  term.write(`\r\n\x1b[92m--- You now have control ---\x1b[0m\r\n`)
                  break
                case "controller_changed":
                  // Informational for watchers
                  break
                case "watcher_joined":
                  setWatcherCount(msg.watcherCount ?? 0)
                  break
                case "watcher_left":
                  setWatcherCount(msg.watcherCount ?? 0)
                  break
                case "exit":
                  setStatus("disconnected")
                  term.write("\r\n\x1b[90m--- Session ended ---\x1b[0m\r\n")
                  break
              }
            } catch {
              term.write(event.data)
            }
          }
        }

        ws.onclose = (event) => {
          if (disposed) return
          setStatus("disconnected")
          term.write("\r\n\x1b[90m--- Session ended ---\x1b[0m\r\n")
          if (event.code !== 1000) {
            term.write(
              `\x1b[90m(code: ${event.code}${event.reason ? `, reason: ${event.reason}` : ""})\x1b[0m\r\n`,
            )
          }
        }

        ws.onerror = () => {
          if (disposed) return
          setStatus("error")
        }

        // Terminal -> WebSocket (controller only — watchers silently discarded)
        term.onData((data) => {
          if (roleRef.current === "watcher") return
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(new TextEncoder().encode(data))
          }
        })

        term.onBinary((data) => {
          if (roleRef.current === "watcher") return
          if (ws.readyState === WebSocket.OPEN) {
            const bytes = new Uint8Array(data.length)
            for (let i = 0; i < data.length; i++) {
              bytes[i] = data.charCodeAt(i)
            }
            ws.send(bytes)
          }
        })

        term.onResize(({ cols, rows }) => {
          if (roleRef.current === "watcher") return
          if (ws.readyState === WebSocket.OPEN) {
            const resizeMsg = JSON.stringify({ type: "resize", cols, rows })
            ws.send(new TextEncoder().encode(resizeMsg))
          }
        })

        const resizeObserver = new ResizeObserver(() => {
          if (fitAddonRef.current && xtermRef.current) {
            try {
              fitAddonRef.current.fit()
            } catch {
              // Ignore fit errors during disposal
            }
          }
        })
        resizeObserver.observe(container)

        return () => {
          resizeObserver.disconnect()
        }
      }

      let cleanupObserver: (() => void) | undefined
      init().then((fn) => {
        cleanupObserver = fn
      })

      return () => {
        disposed = true
        cleanupObserver?.()
        cleanup()
      }
    }, [agentId, sessionId, mode, cleanup, onSessionReady, updateRole])

    const takeControl = useCallback(() => {
      const ws = wsRef.current
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      ws.send(new TextEncoder().encode(JSON.stringify({ type: "take_control" })))
    }, [])

    useImperativeHandle(ref, () => ({ takeControl }), [takeControl])

    return (
      <div className="flex h-full flex-col" data-testid="terminal-view">
        {!hideHeader && (
          <div className="flex items-center justify-between border-b px-4 py-2">
            <div className="flex items-center gap-3">
              <span className="text-sm font-medium">{agentHostname ?? agentId}</span>
              <StatusBadge
                status={status}
                pinned={pinned}
                role={role}
                watcherCount={watcherCount}
              />
            </div>
            <div className="flex items-center gap-1">
              {role === "watcher" && status === "connected" && (
                <Button variant="outline" size="sm" onClick={takeControl} className="h-7 text-xs">
                  Take Control
                </Button>
              )}
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="icon"
                      onClick={togglePin}
                      disabled={!activeSessionId || role === "watcher"}
                      aria-label={pinned ? "Unpin session" : "Pin session"}
                      className="h-7 w-7"
                    >
                      {pinned ? (
                        <PinOff className="h-3.5 w-3.5" />
                      ) : (
                        <Pin className="h-3.5 w-3.5" />
                      )}
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
                      disabled={!activeSessionId}
                      aria-label="Pop out"
                      className="h-7 w-7"
                    >
                      <ExternalLink className="h-3.5 w-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Pop out to new window</TooltipContent>
                </Tooltip>
              </TooltipProvider>
              {onClose && (
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={onClose}
                  aria-label="Close terminal"
                  className="h-7 w-7"
                >
                  <X className="h-3.5 w-3.5" />
                </Button>
              )}
            </div>
          </div>
        )}
        <div ref={termRef} className="flex-1 bg-[#09090b] p-1" data-testid="terminal-container" />
      </div>
    )
  },
)

function StatusBadge({
  status,
  pinned,
  role,
  watcherCount,
}: {
  status: TerminalStatus
  pinned: boolean
  role: SessionRole
  watcherCount: number
}) {
  switch (status) {
    case "connecting":
      return (
        <Badge variant="secondary" className="text-xs">
          Connecting...
        </Badge>
      )
    case "connected":
      return (
        <div className="flex items-center gap-1.5">
          {role === "watcher" ? (
            <Badge className="bg-blue-600 text-white text-xs">
              <Eye className="mr-1 h-3 w-3" />
              Watching
            </Badge>
          ) : (
            <Badge className="bg-green-600 text-white text-xs">Connected</Badge>
          )}
          {pinned && (
            <Badge variant="outline" className="text-xs">
              Pinned
            </Badge>
          )}
          {watcherCount > 0 && role === "controller" && (
            <Badge variant="outline" className="text-xs">
              <Eye className="mr-1 h-3 w-3" />
              {watcherCount}
            </Badge>
          )}
        </div>
      )
    case "detached":
      return (
        <Badge variant="secondary" className="text-xs bg-yellow-600/20 text-yellow-600">
          Detached
        </Badge>
      )
    case "disconnected":
      return (
        <Badge variant="secondary" className="text-xs">
          Disconnected
        </Badge>
      )
    case "error":
      return (
        <Badge variant="destructive" className="text-xs">
          Error
        </Badge>
      )
  }
}
