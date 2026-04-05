"use client"

import { ExternalLink, Hand, Pin, PinOff, X } from "lucide-react"
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
    { agentId, agentHostname, sessionId, onClose, onSessionReady, onRoleChange, hideHeader },
    ref,
  ) {
    const termRef = useRef<HTMLDivElement>(null)
    const wsRef = useRef<WebSocket | null>(null)
    const xtermRef = useRef<import("@xterm/xterm").Terminal | null>(null)
    const fitAddonRef = useRef<import("@xterm/addon-fit").FitAddon | null>(null)
    const [status, setStatus] = useState<TerminalStatus>("connecting")
    const [pinned, setPinned] = useState(false)
    const [activeSessionId, setActiveSessionId] = useState<string | null>(sessionId ?? null)
    // Start optimistically as controller — the server corrects to "watcher"
    // (standby) via the initial session message if the seat is taken.
    const [role, setRole] = useState<SessionRole>("controller")
    const roleRef = useRef<SessionRole>("controller")
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

        const wsUrl = sessionId
          ? `${wsProtocol}//${window.location.host}/api/v1/agents/${encodeURIComponent(agentId)}/shell/sessions/${encodeURIComponent(sessionId)}/ws`
          : `${wsProtocol}//${window.location.host}/api/v1/agents/${encodeURIComponent(agentId)}/shell/new?cols=${cols}&rows=${rows}`

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
            // Server only sends PTY data to the controller, so receiving
            // binary here implies we hold control. Render it.
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
                    updateRole(msg.role)
                  }
                  break
                case "detached":
                  setStatus("detached")
                  term.write("\r\n\x1b[90m--- Session detached ---\x1b[0m\r\n")
                  break
                case "control_transferred":
                  // Another user took control — go to standby state.
                  updateRole("watcher")
                  break
                case "control_granted": {
                  // We now hold control. Fit xterm to our container, then
                  // explicitly push our current viewport size to the PTY —
                  // fit() was already called on mount so it may be a no-op,
                  // and onResize won't re-fire. The server follows with a
                  // ring-buffer replay so we see recent session output.
                  updateRole("controller")
                  try {
                    fitAddonRef.current?.fit()
                  } catch {
                    // ignore
                  }
                  const t = xtermRef.current
                  if (t && ws.readyState === WebSocket.OPEN) {
                    ws.send(
                      new TextEncoder().encode(
                        JSON.stringify({ type: "resize", cols: t.cols, rows: t.rows }),
                      ),
                    )
                  }
                  break
                }
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
          // Standby clients may also fit() on container resize to keep
          // their xterm lined up visually, but only controllers drive the
          // PTY. The server discards resize messages from non-controllers.
          if (roleRef.current === "watcher") return
          if (ws.readyState === WebSocket.OPEN) {
            const resizeMsg = JSON.stringify({ type: "resize", cols, rows })
            ws.send(new TextEncoder().encode(resizeMsg))
          }
        })

        const resizeObserver = new ResizeObserver(() => {
          if (!xtermRef.current) return
          if (fitAddonRef.current) {
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
    }, [agentId, sessionId, cleanup, onSessionReady, updateRole])

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
              <StatusBadge status={status} pinned={pinned} role={role} />
            </div>
            <div className="flex items-center gap-1">
              {role === "watcher" && status === "connected" && (
                <Button variant="outline" size="sm" onClick={takeControl} className="h-7 text-xs">
                  <Hand className="mr-1 h-3.5 w-3.5" />
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
        <div className="relative flex-1 overflow-hidden">
          <div
            ref={termRef}
            className={
              role === "watcher"
                ? "absolute inset-0 overflow-auto bg-[#09090b] p-1 pointer-events-none opacity-25 transition-opacity"
                : "absolute inset-0 overflow-auto bg-[#09090b] p-1 transition-opacity"
            }
            data-testid="terminal-container"
          />
          {role === "watcher" && status === "connected" && (
            <div
              className="absolute inset-0 flex items-center justify-center bg-black/50 backdrop-blur-[2px]"
              data-testid="standby-overlay"
            >
              <div className="flex max-w-sm flex-col items-center gap-3 rounded-lg border bg-card px-6 py-5 text-center shadow-lg">
                <p className="text-sm text-muted-foreground">
                  Another connection has control of this session.
                </p>
                <Button onClick={takeControl} size="sm">
                  <Hand className="mr-2 h-4 w-4" />
                  Take Control
                </Button>
              </div>
            </div>
          )}
        </div>
      </div>
    )
  },
)

function StatusBadge({
  status,
  pinned,
  role,
}: {
  status: TerminalStatus
  pinned: boolean
  role: SessionRole
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
            <Badge variant="secondary" className="text-xs">
              Standby
            </Badge>
          ) : (
            <Badge className="bg-green-600 text-white text-xs">Connected</Badge>
          )}
          {pinned && (
            <Badge variant="outline" className="text-xs">
              Pinned
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
