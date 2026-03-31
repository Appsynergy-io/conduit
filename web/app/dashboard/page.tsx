"use client"

import { useCallback, useEffect, useState } from "react"
import { AgentList } from "@/components/agent-list"
import { useAuth } from "@/hooks/use-auth"
import { useEventBus } from "@/hooks/use-websocket-context"

export interface Agent {
  id: string
  hostname: string
  displayName?: string
  os?: string
  arch?: string
  labels?: Record<string, string>
  ip?: string
  status: string
  transport?: string
  version?: string
  lastSeenAt?: string
  connectedAt?: string
  createdAt: string
}

export default function DashboardPage() {
  const { token } = useAuth()
  const { subscribe } = useEventBus()
  const [agents, setAgents] = useState<Agent[]>([])
  const [loading, setLoading] = useState(true)

  const fetchAgents = useCallback(async () => {
    if (!token) return
    try {
      const res = await fetch("/api/v1/agents", {
        headers: { Authorization: `Bearer ${token}` },
      })
      if (res.ok) {
        const data = await res.json()
        setAgents(data.items ?? [])
      }
    } finally {
      setLoading(false)
    }
  }, [token])

  useEffect(() => {
    fetchAgents()
  }, [fetchAgents])

  // Real-time updates from EventBus
  useEffect(() => {
    const unsub = subscribe("agents", (event) => {
      if (event.type === "agent.connected" || event.type === "agent.disconnected") {
        fetchAgents()
      }
    })
    return unsub
  }, [subscribe, fetchAgents])

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Agents</h1>
        <p className="text-sm text-muted-foreground">Connected machines and their status.</p>
      </div>
      <AgentList agents={agents} loading={loading} />
    </div>
  )
}
