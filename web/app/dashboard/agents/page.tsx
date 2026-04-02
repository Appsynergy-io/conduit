"use client"

import { Suspense } from "react"
import { useSearchParams } from "next/navigation"
import { AgentDetail } from "@/components/agent-detail"
import { Skeleton } from "@/components/ui/skeleton"

function AgentDetailContent() {
  const searchParams = useSearchParams()
  const agentId = searchParams.get("id")

  if (!agentId) {
    return (
      <div className="flex flex-col items-center justify-center py-16 text-center">
        <p className="text-sm text-muted-foreground">No agent selected.</p>
      </div>
    )
  }

  return <AgentDetail agentId={agentId} />
}

export default function AgentDetailPage() {
  return (
    <Suspense
      fallback={
        <div className="space-y-4">
          <Skeleton className="h-8 w-48" />
          <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
            {["a", "b", "c", "d"].map((k) => (
              <Skeleton key={k} className="h-32" />
            ))}
          </div>
        </div>
      }
    >
      <AgentDetailContent />
    </Suspense>
  )
}
