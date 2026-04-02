"use client"

import { formatDistanceToNow } from "date-fns"
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  FolderOpen,
  Monitor,
  Search,
  Terminal,
} from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useMemo, useState } from "react"
import type { Agent } from "@/app/dashboard/page"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

// ── Types ──

type SortField = "hostname" | "status" | "os" | "lastSeen"
type SortDir = "asc" | "desc"
type StatusFilter = "all" | "online" | "offline" | "stale"

// ── Helpers ──

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

function formatTime(ts?: string) {
  if (!ts) return "-"
  try {
    return formatDistanceToNow(new Date(ts), { addSuffix: true })
  } catch {
    return ts
  }
}

const STATUS_ORDER: Record<string, number> = { online: 0, offline: 1, stale: 2 }

function sortAgents(agents: Agent[], field: SortField, dir: SortDir): Agent[] {
  return [...agents].sort((a, b) => {
    let cmp = 0
    switch (field) {
      case "hostname":
        cmp = (a.displayName ?? a.hostname).localeCompare(b.displayName ?? b.hostname)
        break
      case "status":
        cmp = (STATUS_ORDER[a.status] ?? 3) - (STATUS_ORDER[b.status] ?? 3)
        break
      case "os":
        cmp = (a.os ?? "").localeCompare(b.os ?? "")
        break
      case "lastSeen": {
        const aTime = a.lastSeenAt ?? a.connectedAt ?? ""
        const bTime = b.lastSeenAt ?? b.connectedAt ?? ""
        cmp = aTime.localeCompare(bTime)
        break
      }
    }
    return dir === "asc" ? cmp : -cmp
  })
}

// ── Component ──

export function AgentList({ agents, loading }: { agents: Agent[]; loading: boolean }) {
  const router = useRouter()
  const [search, setSearch] = useState("")
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all")
  const [sortField, setSortField] = useState<SortField>("hostname")
  const [sortDir, setSortDir] = useState<SortDir>("asc")

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir(sortDir === "asc" ? "desc" : "asc")
    } else {
      setSortField(field)
      setSortDir("asc")
    }
  }

  const sortIcon = (field: SortField) => {
    if (sortField !== field) return <ArrowUpDown className="ml-1 h-3 w-3" />
    return sortDir === "asc" ? (
      <ArrowUp className="ml-1 h-3 w-3" />
    ) : (
      <ArrowDown className="ml-1 h-3 w-3" />
    )
  }

  const filtered = useMemo(() => {
    let result = agents

    // Status filter
    if (statusFilter !== "all") {
      result = result.filter((a) => a.status === statusFilter)
    }

    // Search filter (hostname, display name, IP, OS)
    if (search.trim()) {
      const q = search.toLowerCase()
      result = result.filter(
        (a) =>
          (a.displayName ?? a.hostname).toLowerCase().includes(q) ||
          a.hostname.toLowerCase().includes(q) ||
          (a.ip ?? "").toLowerCase().includes(q) ||
          (a.os ?? "").toLowerCase().includes(q),
      )
    }

    return sortAgents(result, sortField, sortDir)
  }, [agents, search, statusFilter, sortField, sortDir])

  if (loading) {
    return (
      <div className="space-y-3">
        {["a", "b", "c", "d", "e"].map((k) => (
          <Skeleton key={k} className="h-12 w-full" />
        ))}
      </div>
    )
  }

  if (agents.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-16 text-center">
        <Monitor className="h-12 w-12 text-muted-foreground/50" />
        <h3 className="mt-4 text-lg font-medium">No agents connected</h3>
        <p className="mt-1 max-w-sm text-sm text-muted-foreground">
          Generate a join token and run{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">conduit join</code> on a target
          machine to register it.
        </p>
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Search + Filter toolbar */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center" data-testid="agent-toolbar">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search by hostname, IP, or OS..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
            data-testid="agent-search"
          />
        </div>
        <Select value={statusFilter} onValueChange={(v) => setStatusFilter(v as StatusFilter)}>
          <SelectTrigger className="w-full sm:w-[140px]" data-testid="status-filter">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Status</SelectItem>
            <SelectItem value="online">Online</SelectItem>
            <SelectItem value="offline">Offline</SelectItem>
            <SelectItem value="stale">Stale</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Filtered empty state */}
      {filtered.length === 0 && (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-12 text-center">
          <Search className="h-8 w-8 text-muted-foreground/50" />
          <p className="mt-2 text-sm text-muted-foreground">
            No agents match your search or filter.
          </p>
        </div>
      )}

      {/* Agent table */}
      {filtered.length > 0 && (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>
                  <button
                    type="button"
                    className="inline-flex items-center text-xs font-medium"
                    onClick={() => handleSort("hostname")}
                  >
                    Hostname {sortIcon("hostname")}
                  </button>
                </TableHead>
                <TableHead>
                  <button
                    type="button"
                    className="inline-flex items-center text-xs font-medium"
                    onClick={() => handleSort("status")}
                  >
                    Status {sortIcon("status")}
                  </button>
                </TableHead>
                <TableHead className="hidden md:table-cell">
                  <button
                    type="button"
                    className="inline-flex items-center text-xs font-medium"
                    onClick={() => handleSort("os")}
                  >
                    OS / Arch {sortIcon("os")}
                  </button>
                </TableHead>
                <TableHead className="hidden md:table-cell">IP</TableHead>
                <TableHead className="hidden lg:table-cell">Transport</TableHead>
                <TableHead className="hidden lg:table-cell">
                  <button
                    type="button"
                    className="inline-flex items-center text-xs font-medium"
                    onClick={() => handleSort("lastSeen")}
                  >
                    Last Seen {sortIcon("lastSeen")}
                  </button>
                </TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((agent) => (
                <TableRow
                  key={agent.id}
                  className="cursor-pointer hover:bg-muted/50"
                  onClick={() => router.push(`/dashboard/agents?id=${agent.id}`)}
                >
                  <TableCell className="font-medium">
                    {agent.displayName ?? agent.hostname}
                  </TableCell>
                  <TableCell>{statusBadge(agent.status)}</TableCell>
                  <TableCell className="hidden md:table-cell">
                    {[agent.os, agent.arch].filter(Boolean).join(" / ") || "-"}
                  </TableCell>
                  <TableCell className="hidden md:table-cell">{agent.ip ?? "-"}</TableCell>
                  <TableCell className="hidden lg:table-cell">{agent.transport ?? "-"}</TableCell>
                  <TableCell className="hidden lg:table-cell">
                    {formatTime(agent.lastSeenAt ?? agent.connectedAt)}
                  </TableCell>
                  <TableCell className="text-right" onClick={(e) => e.stopPropagation()}>
                    <div className="flex items-center justify-end gap-1">
                      <Button variant="ghost" size="icon" asChild>
                        <Link href={`/dashboard/terminal?agent=${agent.id}`}>
                          <Terminal className="h-4 w-4" />
                        </Link>
                      </Button>
                      <Button variant="ghost" size="icon" asChild>
                        <Link href={`/dashboard/files?agent=${agent.id}`}>
                          <FolderOpen className="h-4 w-4" />
                        </Link>
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
