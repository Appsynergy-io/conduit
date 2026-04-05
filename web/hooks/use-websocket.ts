"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { refreshToken } from "@/lib/refresh"

interface EventBusMessage {
  channel: string
  type: string
  data: Record<string, string>
}

type EventHandler = (event: EventBusMessage) => void

/**
 * Valid EventBus channels. Clients subscribe to specific channels
 * to receive only relevant events (NIST SI-4).
 */
const ALL_CHANNELS = [
  "agents",
  "shell",
  "files",
  "auth",
  "audit",
  "metrics",
  "exec",
  "system",
] as const

export type EventChannel = (typeof ALL_CHANNELS)[number]

export function useWebSocket(initialChannels?: EventChannel[]) {
  const wsRef = useRef<WebSocket | null>(null)
  const handlersRef = useRef<Map<string, Set<EventHandler>>>(new Map())
  const [connected, setConnected] = useState(false)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const activeChannels = useRef<Set<string>>(
    new Set(initialChannels ?? ALL_CHANNELS),
  )
  // Track consecutive failures to avoid hammering the server.
  const failCount = useRef(0)

  const connect = useCallback(() => {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
    const channels = Array.from(activeChannels.current).join(",")
    const url = `${protocol}//${window.location.host}/api/v1/events/stream${channels ? `?channels=${channels}` : ""}`

    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onopen = () => {
      setConnected(true)
      failCount.current = 0
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current)
        reconnectTimer.current = null
      }
    }

    ws.onmessage = (event) => {
      try {
        const msg: EventBusMessage = JSON.parse(event.data)

        // Skip control messages (pong, error)
        if (msg.type === "pong" || msg.type === "error") return

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
      failCount.current++

      // Before reconnecting, try a silent token refresh. The connection may
      // have been rejected because the access cookie expired (401 on upgrade).
      const delay = Math.min(2000 * failCount.current, 10000)
      reconnectTimer.current = setTimeout(async () => {
        // Attempt refresh if we've failed — the access token likely expired.
        if (failCount.current >= 1) {
          await refreshToken()
        }
        connect()
      }, delay)
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

  /**
   * Subscribe to events on a specific channel. Returns an unsubscribe function.
   * Use "*" to receive all events regardless of channel.
   */
  const subscribe = useCallback((channel: string, handler: EventHandler) => {
    if (!handlersRef.current.has(channel)) {
      handlersRef.current.set(channel, new Set())
    }
    const handlers = handlersRef.current.get(channel)
    if (handlers) handlers.add(handler)

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

  /**
   * Dynamically subscribe to additional server-side channels.
   */
  const subscribeChannels = useCallback((channels: EventChannel[]) => {
    for (const ch of channels) activeChannels.current.add(ch)
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: "subscribe", channels }))
    }
  }, [])

  /**
   * Dynamically unsubscribe from server-side channels.
   */
  const unsubscribeChannels = useCallback((channels: EventChannel[]) => {
    for (const ch of channels) activeChannels.current.delete(ch)
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: "unsubscribe", channels }))
    }
  }, [])

  return { connected, subscribe, subscribeChannels, unsubscribeChannels }
}
