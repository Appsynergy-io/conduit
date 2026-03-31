"use client";

import { useCallback, useEffect, useState } from "react";
import { useAuth } from "@/hooks/use-auth";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { formatDistanceToNow } from "date-fns";

interface AuditEvent {
  id: string;
  eventType: string;
  userEmail?: string;
  agentHostname?: string;
  sourceIp?: string;
  outcome: string;
  timestamp: string;
}

export default function AuditPage() {
  const { token } = useAuth();
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchEvents = useCallback(async () => {
    if (!token) return;
    try {
      const res = await fetch("/api/v1/audit/events?limit=50", {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setEvents(data.items ?? []);
      }
    } finally {
      setLoading(false);
    }
  }, [token]);

  useEffect(() => {
    fetchEvents();
  }, [fetchEvents]);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Audit Log</h1>
        <p className="text-sm text-muted-foreground">
          Security events and access history.
        </p>
      </div>

      {loading ? (
        <div className="space-y-3">
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : events.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-12 text-center">
          <p className="text-lg font-medium">No audit events</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Events will appear here as users and agents interact with Conduit.
          </p>
        </div>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Event</TableHead>
                <TableHead>Outcome</TableHead>
                <TableHead className="hidden md:table-cell">User</TableHead>
                <TableHead className="hidden md:table-cell">Agent</TableHead>
                <TableHead className="hidden lg:table-cell">Source IP</TableHead>
                <TableHead>When</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {events.map((event) => (
                <TableRow key={event.id}>
                  <TableCell className="font-mono text-xs">
                    {event.eventType}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        event.outcome === "success" ? "default" : "destructive"
                      }
                    >
                      {event.outcome}
                    </Badge>
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    {event.userEmail ?? "-"}
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    {event.agentHostname ?? "-"}
                  </TableCell>
                  <TableCell className="hidden lg:table-cell">
                    {event.sourceIp ?? "-"}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {formatDistanceToNow(new Date(event.timestamp), {
                      addSuffix: true,
                    })}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
