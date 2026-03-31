"use client"

import { Webhook } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { useAuth } from "@/hooks/use-auth"

interface WebhookSub {
  id: string
  url: string
  events: string[]
  enabled: boolean
  createdAt: string
}

export default function WebhooksPage() {
  const { isAuthenticated } = useAuth()
  const [webhooks, setWebhooks] = useState<WebhookSub[]>([])
  const [loading, setLoading] = useState(true)

  const fetchWebhooks = useCallback(async () => {
    if (!isAuthenticated) return
    try {
      const res = await fetch("/api/v1/webhooks")
      if (res.ok) {
        const data = await res.json()
        setWebhooks(data.items ?? [])
      }
    } finally {
      setLoading(false)
    }
  }, [isAuthenticated])

  useEffect(() => {
    fetchWebhooks()
  }, [fetchWebhooks])

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Webhooks</h1>
        <p className="text-sm text-muted-foreground">Event subscriptions and delivery history.</p>
      </div>

      {loading ? (
        <div className="space-y-3">
          {["a", "b", "c"].map((k) => (
            <Skeleton key={k} className="h-10 w-full" />
          ))}
        </div>
      ) : webhooks.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-16 text-center">
          <Webhook className="h-12 w-12 text-muted-foreground/50" />
          <h3 className="mt-4 text-lg font-medium">No webhooks configured</h3>
          <p className="mt-1 max-w-sm text-sm text-muted-foreground">
            Create a webhook subscription to receive real-time event notifications over HTTPS.
          </p>
        </div>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>URL</TableHead>
                <TableHead>Events</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {webhooks.map((wh) => (
                <TableRow key={wh.id}>
                  <TableCell className="max-w-xs truncate font-mono text-xs">{wh.url}</TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {wh.events.map((e) => (
                        <Badge key={e} variant="outline" className="text-xs">
                          {e}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={wh.enabled ? "default" : "secondary"}>
                      {wh.enabled ? "Active" : "Disabled"}
                    </Badge>
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
