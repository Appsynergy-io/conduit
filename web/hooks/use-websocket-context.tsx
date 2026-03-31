"use client"

import { createContext, useContext } from "react"

interface WebSocketState {
  connected: boolean
  subscribe: (
    channel: string,
    handler: (event: { channel: string; type: string; data: Record<string, string> }) => void,
  ) => () => void
}

const WebSocketContext = createContext<WebSocketState>({
  connected: false,
  subscribe: () => () => {},
})

export const WebSocketProvider = WebSocketContext.Provider

export function useEventBus() {
  return useContext(WebSocketContext)
}
