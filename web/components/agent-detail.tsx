"use client"

import { formatDistanceToNow } from "date-fns"
import {
  ArrowLeft,
  Cpu,
  FolderOpen,
  HardDrive,
  MemoryStick,
  Terminal,
  Timer,
} from "lucide-react"
import Link from "next/link"
import { useCallback, useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuth } from "@/hooks/use-auth"
import { useEventBus } from "@/hooks/use-websocket-context"

interface AgentData {
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
  systemInfo?: SystemInfo
}

interface SystemInfo {
  cpuPercent: number
  memoryTotalBytes: number
  memoryUsedBytes: number
  diskTotalBytes: number
  diskUsedBytes: number
  uptimeSeconds: number
  loadAvg1?: number
  loadAvg5?: number
  loadAvg15?: number
}

function statusBadge(status: string) {
  switch (status) {
    case "online":
      return <Badge className="bg-green-600 text-white">Online</Badge>
    case "offline":
      return <Badge variant="secondary">Offline</Badge>
    case "stale":
      return <Badge variant="destructive">Stale</Badge>
    default:
      return <Badge variant="outline">{status}</Badge>
  }
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  const value = bytes / 1024 ** i
  return `${value.toFixed(i > 0 ? 1 : 0)} ${units[i]}`
}

function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h ${mins}m`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

export function AgentDetail({ agentId }: { agentId: string }) {
  const { isAuthenticated } = useAuth()
  const { subscribe } = useEventBus()
  const [agent, setAgent] = useState<AgentData | null>(null)
  const [metrics, setMetrics] = useState<SystemInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fetchAgent = useCallback(async () => {
    if (!isAuthenticated) return
    try {
      const res = await fetch(`/api/v1/agents/${encodeURIComponent(agentId)}`)
      if (!res.ok) {
        setError(res.status === 404 ? "Agent not found." : "Failed to load agent.")
        return
      }
      const data: AgentData = await res.json()
      setAgent(data)
      if (data.systemInfo) {
        setMetrics(data.systemInfo)
      }
    } catch {
      setError("Failed to load agent.")
    } finally {
      setLoading(false)
    }
  }, [isAuthenticated, agentId])

  useEffect(() => {
    fetchAgent()
  }, [fetchAgent])

  // Real-time metrics updates via EventBus
  useEffect(() => {
    const unsub = subscribe("metrics", (event) => {
      if (event.type === "agent.metrics" && event.data.agentId === agentId) {
        const d = event.data as Record<string, unknown>
        setMetrics({
          cpuPercent: d.cpuPercent as number,
          memoryTotalBytes: d.memTotal as number,
          memoryUsedBytes: d.memUsed as number,
          diskTotalBytes: d.diskTotal as number,
          diskUsedBytes: d.diskUsed as number,
          uptimeSeconds: d.uptime as number,
          loadAvg1: d.loadAvg1 as number,
          loadAvg5: d.loadAvg5 as number,
          loadAvg15: d.loadAvg15 as number,
        })
      }
    })
    return unsub
  }, [subscribe, agentId])

  // Track agent status changes
  useEffect(() => {
    const unsub = subscribe("agents", (event) => {
      if (
        event.data.agentId === agentId &&
        (event.type === "agent.connected" || event.type === "agent.disconnected")
      ) {
        fetchAgent()
      }
    })
    return unsub
  }, [subscribe, agentId, fetchAgent])

  if (loading) {
    return (
      <div className="space-y-6">
        <Skeleton className="h-8 w-48" />
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          {["a", "b", "c", "d"].map((k) => (
            <Skeleton key={k} className="h-32" />
          ))}
        </div>
      </div>
    )
  }

  if (error || !agent) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" size="sm" asChild>
          <Link href="/dashboard">
            <ArrowLeft className="mr-2 h-4 w-4" />
            Back to Agents
          </Link>
        </Button>
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-16 text-center">
          <p className="text-sm text-muted-foreground">{error ?? "Agent not found."}</p>
        </div>
      </div>
    )
  }

  const memPercent =
    metrics && metrics.memoryTotalBytes > 0
      ? (metrics.memoryUsedBytes / metrics.memoryTotalBytes) * 100
      : 0
  const diskPercent =
    metrics && metrics.diskTotalBytes > 0
      ? (metrics.diskUsedBytes / metrics.diskTotalBytes) * 100
      : 0

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <Button variant="ghost" size="icon" asChild>
            <Link href="/dashboard">
              <ArrowLeft className="h-4 w-4" />
            </Link>
          </Button>
          <div>
            <h1 className="text-2xl font-bold tracking-tight">
              {agent.displayName ?? agent.hostname}
            </h1>
            <div className="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
              {agent.displayName && <span>{agent.hostname}</span>}
              <span>{[agent.os, agent.arch].filter(Boolean).join(" / ")}</span>
              {agent.ip && <span>{agent.ip}</span>}
              {agent.version && <span>v{agent.version}</span>}
            </div>
          </div>
          <div className="ml-2">{statusBadge(agent.status)}</div>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" asChild>
            <Link href={`/dashboard/terminal?agent=${agent.id}`}>
              <Terminal className="mr-2 h-4 w-4" />
              Terminal
            </Link>
          </Button>
          <Button variant="outline" size="sm" asChild>
            <Link href={`/dashboard/files?agent=${agent.id}`}>
              <FolderOpen className="mr-2 h-4 w-4" />
              Files
            </Link>
          </Button>
        </div>
      </div>

      {/* Metrics cards */}
      {agent.status === "online" && metrics ? (
        <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
          {/* CPU */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">CPU Usage</CardTitle>
              <Cpu className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{metrics.cpuPercent.toFixed(1)}%</div>
              <Progress value={metrics.cpuPercent} className="mt-2" />
              {(metrics.loadAvg1 !== undefined || metrics.loadAvg5 !== undefined) && (
                <p className="mt-2 text-xs text-muted-foreground">
                  Load: {metrics.loadAvg1?.toFixed(2)} / {metrics.loadAvg5?.toFixed(2)} /{" "}
                  {metrics.loadAvg15?.toFixed(2)}
                </p>
              )}
            </CardContent>
          </Card>

          {/* Memory */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Memory</CardTitle>
              <MemoryStick className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{memPercent.toFixed(1)}%</div>
              <Progress value={memPercent} className="mt-2" />
              <p className="mt-2 text-xs text-muted-foreground">
                {formatBytes(metrics.memoryUsedBytes)} / {formatBytes(metrics.memoryTotalBytes)}
              </p>
            </CardContent>
          </Card>

          {/* Disk */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Disk</CardTitle>
              <HardDrive className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{diskPercent.toFixed(1)}%</div>
              <Progress value={diskPercent} className="mt-2" />
              <p className="mt-2 text-xs text-muted-foreground">
                {formatBytes(metrics.diskUsedBytes)} / {formatBytes(metrics.diskTotalBytes)}
              </p>
            </CardContent>
          </Card>

          {/* Uptime */}
          <Card>
            <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
              <CardTitle className="text-sm font-medium">Uptime</CardTitle>
              <Timer className="h-4 w-4 text-muted-foreground" />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">{formatUptime(metrics.uptimeSeconds)}</div>
              {agent.connectedAt && (
                <p className="mt-2 text-xs text-muted-foreground">
                  Connected{" "}
                  {formatDistanceToNow(new Date(agent.connectedAt), { addSuffix: true })}
                </p>
              )}
              {agent.transport && (
                <p className="mt-1 text-xs text-muted-foreground">
                  Transport: {agent.transport.toUpperCase()}
                </p>
              )}
            </CardContent>
          </Card>
        </div>
      ) : (
        <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
          {agent.status === "online"
            ? "Waiting for metrics..."
            : "Agent is offline. Metrics are available when the agent is connected."}
        </div>
      )}

      {/* Labels */}
      {agent.labels && Object.keys(agent.labels).length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="text-sm font-medium">Labels</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-wrap gap-2">
              {Object.entries(agent.labels).map(([key, value]) => (
                <Badge key={key} variant="outline">
                  {key}={value}
                </Badge>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Agent info */}
      <Card>
        <CardHeader>
          <CardTitle className="text-sm font-medium">Details</CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-1 gap-x-6 gap-y-3 text-sm sm:grid-cols-2 lg:grid-cols-3">
            <div>
              <dt className="text-muted-foreground">Agent ID</dt>
              <dd className="font-mono text-xs">{agent.id}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Hostname</dt>
              <dd>{agent.hostname}</dd>
            </div>
            {agent.os && (
              <div>
                <dt className="text-muted-foreground">OS / Arch</dt>
                <dd>{[agent.os, agent.arch].filter(Boolean).join(" / ")}</dd>
              </div>
            )}
            {agent.ip && (
              <div>
                <dt className="text-muted-foreground">IP Address</dt>
                <dd>{agent.ip}</dd>
              </div>
            )}
            {agent.transport && (
              <div>
                <dt className="text-muted-foreground">Transport</dt>
                <dd>{agent.transport.toUpperCase()}</dd>
              </div>
            )}
            {agent.version && (
              <div>
                <dt className="text-muted-foreground">Version</dt>
                <dd>{agent.version}</dd>
              </div>
            )}
            <div>
              <dt className="text-muted-foreground">Registered</dt>
              <dd>
                {formatDistanceToNow(new Date(agent.createdAt), { addSuffix: true })}
              </dd>
            </div>
            {agent.lastSeenAt && (
              <div>
                <dt className="text-muted-foreground">Last Seen</dt>
                <dd>
                  {formatDistanceToNow(new Date(agent.lastSeenAt), { addSuffix: true })}
                </dd>
              </div>
            )}
          </dl>
        </CardContent>
      </Card>
    </div>
  )
}
