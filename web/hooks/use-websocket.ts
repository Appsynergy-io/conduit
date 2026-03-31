"use client"

import { useCallback, useEffect, useRef, useState } from "react"

interface EventBusMessage {
  channel: string
  type: string
  data: Record<string, string>
}

type EventHandler = (event: EventBusMessage) => void

export function useWebSocket() {
  const wsRef = useRef<WebSocket | null>(null)
  const handlersRef = useRef<Map<string, Set<EventHandler>>>(new Map())
  const [connected, setConnected] = useState(false)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  const connect = useCallback(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
    const url = `${protocol}//${window.location.host}/api/v1/events/stream`

    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current)
        reconnectTimer.current = null
      }
    }

    ws.onmessage = (event) => {
      try {
        const msg: EventBusMessage = JSON.parse(event.data)
        const channelHandlers = handlersRef.current.get(msg.channel)
        if (channelHandlers) {
          for (const handler of channelHandlers) handler(msg)
        }
        // Also notify "*" wildcard subscribers
        const allHandlers = handlersRef.current.get("*")
        if (allHandlers) {
          for (const handler of allHandlers) handler(msg)
        }
      } catch {
        // Ignore malformed messages
      }
    }

    ws.onclose = () => {
      setConnected(false)
      wsRef.current = null
      // Reconnect after 2 seconds
      reconnectTimer.current = setTimeout(connect, 2000)
    }

    ws.onerror = () => {
      ws.close()
    }
  }, [])

  useEffect(() => {
    connect()
    return () => {
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current)
      }
      if (wsRef.current) {
        wsRef.current.close()
        wsRef.current = null
      }
    }
  }, [connect])

  const subscribe = useCallback((channel: string, handler: EventHandler) => {
    if (!handlersRef.current.has(channel)) {
      handlersRef.current.set(channel, new Set())
    }
    const handlers = handlersRef.current.get(channel)
    if (handlers) handlers.add(handler)

    // Return unsubscribe function
    return () => {
      const set = handlersRef.current.get(channel)
      if (set) {
        set.delete(handler)
        if (set.size === 0) {
          handlersRef.current.delete(channel)
        }
      }
    }
  }, [])

  return { connected, subscribe }
}
