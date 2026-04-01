"use client"

import { createContext, useContext } from "react"
import type { EventChannel } from "./use-websocket"

interface WebSocketState {
  connected: boolean
  subscribe: (
    channel: string,
    handler: (event: { channel: string; type: string; data: Record<string, string> }) => void,
  ) => () => void
  subscribeChannels: (channels: EventChannel[]) => void
  unsubscribeChannels: (channels: EventChannel[]) => void
}

const WebSocketContext = createContext<WebSocketState>({
  connected: false,
  subscribe: () => () => {},
  subscribeChannels: () => {},
  unsubscribeChannels: () => {},
})

export const WebSocketProvider = WebSocketContext.Provider

export function useEventBus() {
  return useContext(WebSocketContext)
}
