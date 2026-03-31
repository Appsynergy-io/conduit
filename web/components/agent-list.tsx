"use client";

import Link from "next/link";
import type { Agent } from "@/app/dashboard/page";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Terminal, FolderOpen } from "lucide-react";
import { formatDistanceToNow } from "date-fns";

function statusBadge(status: string) {
  switch (status) {
    case "online":
      return <Badge className="bg-green-600 text-white">Online</Badge>;
    case "offline":
      return <Badge variant="secondary">Offline</Badge>;
    case "stale":
      return <Badge variant="destructive">Stale</Badge>;
    default:
      return <Badge variant="outline">{status}</Badge>;
  }
}

function formatTime(ts?: string) {
  if (!ts) return "-";
  try {
    return formatDistanceToNow(new Date(ts), { addSuffix: true });
  } catch {
    return ts;
  }
}

export function AgentList({
  agents,
  loading,
}: {
  agents: Agent[];
  loading: boolean;
}) {
  if (loading) {
    return (
      <div className="space-y-3">
        {Array.from({ length: 5 }).map((_, i) => (
          <Skeleton key={i} className="h-12 w-full" />
        ))}
      </div>
    );
  }

  if (agents.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-12 text-center">
        <p className="text-lg font-medium">No agents connected</p>
        <p className="mt-1 text-sm text-muted-foreground">
          Generate a join token and run{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
            conduit join
          </code>{" "}
          on a target machine.
        </p>
      </div>
    );
  }

  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Hostname</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="hidden md:table-cell">OS / Arch</TableHead>
            <TableHead className="hidden md:table-cell">IP</TableHead>
            <TableHead className="hidden lg:table-cell">Transport</TableHead>
            <TableHead className="hidden lg:table-cell">Last Seen</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {agents.map((agent) => (
            <TableRow key={agent.id}>
              <TableCell className="font-medium">
                {agent.displayName ?? agent.hostname}
              </TableCell>
              <TableCell>{statusBadge(agent.status)}</TableCell>
              <TableCell className="hidden md:table-cell">
                {[agent.os, agent.arch].filter(Boolean).join(" / ") || "-"}
              </TableCell>
              <TableCell className="hidden md:table-cell">
                {agent.ip ?? "-"}
              </TableCell>
              <TableCell className="hidden lg:table-cell">
                {agent.transport ?? "-"}
              </TableCell>
              <TableCell className="hidden lg:table-cell">
                {formatTime(agent.lastSeenAt ?? agent.connectedAt)}
              </TableCell>
              <TableCell className="text-right">
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
  );
}
