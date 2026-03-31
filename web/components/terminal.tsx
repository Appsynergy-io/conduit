"use client"

import { X } from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"

type TerminalStatus = "connecting" | "connected" | "disconnected" | "error"

interface TerminalProps {
  agentId: string
  agentHostname?: string
  onClose?: () => void
}

export function TerminalView({ agentId, agentHostname, onClose }: TerminalProps) {
  const termRef = useRef<HTMLDivElement>(null)
  const wsRef = useRef<WebSocket | null>(null)
  const xtermRef = useRef<import("@xterm/xterm").Terminal | null>(null)
  const fitAddonRef = useRef<import("@xterm/addon-fit").FitAddon | null>(null)
  const [status, setStatus] = useState<TerminalStatus>("connecting")

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

  useEffect(() => {
    if (!termRef.current) return

    const container = termRef.current
    let disposed = false

    async function init() {
      // Dynamic import — xterm.js is browser-only (no SSR)
      const { Terminal } = await import("@xterm/xterm")
      const { FitAddon } = await import("@xterm/addon-fit")
      // CSS must be imported for xterm rendering
      await import("@xterm/xterm/css/xterm.css")

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

      // Build WebSocket URL — token passed as query param (server expects this)
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
      const { cols, rows } = term
      const wsUrl = `${protocol}//${window.location.host}/api/v1/shell/${encodeURIComponent(agentId)}?cols=${cols}&rows=${rows}`

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
          term.write(event.data)
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

      // Terminal → WebSocket: send keystrokes as binary
      term.onData((data) => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(new TextEncoder().encode(data))
        }
      })

      // Binary data path for paste operations
      term.onBinary((data) => {
        if (ws.readyState === WebSocket.OPEN) {
          const bytes = new Uint8Array(data.length)
          for (let i = 0; i < data.length; i++) {
            bytes[i] = data.charCodeAt(i)
          }
          ws.send(bytes)
        }
      })

      // Handle terminal resize → send resize command to server
      term.onResize(({ cols, rows }) => {
        if (ws.readyState === WebSocket.OPEN) {
          // Server expects JSON resize messages
          const resizeMsg = JSON.stringify({ type: "resize", cols, rows })
          ws.send(new TextEncoder().encode(resizeMsg))
        }
      })

      // Handle container resize via ResizeObserver
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

      // Store observer for cleanup
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
  }, [agentId, cleanup])

  return (
    <div className="flex h-full flex-col" data-testid="terminal-view">
      <div className="flex items-center justify-between border-b px-4 py-2">
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium">{agentHostname ?? agentId}</span>
          <StatusBadge status={status} />
        </div>
        {onClose && (
          <Button variant="ghost" size="icon" onClick={onClose} aria-label="Close terminal">
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>
      <div ref={termRef} className="flex-1 bg-[#09090b] p-1" data-testid="terminal-container" />
    </div>
  )
}

function StatusBadge({ status }: { status: TerminalStatus }) {
  switch (status) {
    case "connecting":
      return (
        <Badge variant="secondary" className="text-xs">
          Connecting...
        </Badge>
      )
    case "connected":
      return <Badge className="bg-green-600 text-white text-xs">Connected</Badge>
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
