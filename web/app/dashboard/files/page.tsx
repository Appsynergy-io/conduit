"use client"

import { AlertCircle, WifiOff } from "lucide-react"
import { useSearchParams } from "next/navigation"
import { Suspense, useEffect, useState } from "react"
import { FileBrowser } from "@/components/file-browser"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Skeleton } from "@/components/ui/skeleton"
import { useAuth } from "@/hooks/use-auth"

interface Agent {
  id: string
  hostname: string
  displayName?: string
  status: string
}

function FileBrowserContent() {
  const searchParams = useSearchParams()
  const { token } = useAuth()
  const agentId = searchParams.get("agent")
  const [agent, setAgent] = useState<Agent | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!token || !agentId) {
      setLoading(false)
      return
    }

    const id = agentId
    const controller = new AbortController()

    async function fetchAgent() {
      try {
        const res = await fetch(`/api/v1/agents/${encodeURIComponent(id)}`, {
          headers: { Authorization: `Bearer ${token}` },
          signal: controller.signal,
        })

        if (!res.ok) {
          if (res.status === 404) {
            setError("Agent not found")
          } else if (res.status === 401) {
            setError("Unauthorized")
          } else {
            setError("Failed to load agent")
          }
          return
        }

        const data = await res.json()
        setAgent(data)
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return
        setError("Failed to connect to server")
      } finally {
        setLoading(false)
      }
    }

    fetchAgent()
    return () => controller.abort()
  }, [token, agentId])

  if (!agentId) {
    return (
      <Alert className="mx-auto max-w-lg mt-12">
        <AlertCircle />
        <AlertTitle>No agent selected</AlertTitle>
        <AlertDescription>Select an agent from the dashboard to browse files.</AlertDescription>
      </Alert>
    )
  }

  if (loading) {
    return (
      <div className="flex h-[calc(100vh-7rem)] flex-col gap-3">
        <Skeleton className="h-10 w-full" />
        <Skeleton className="flex-1 w-full" />
      </div>
    )
  }

  if (error) {
    return (
      <Alert variant="destructive" className="mx-auto max-w-lg mt-12">
        <AlertCircle />
        <AlertTitle>{error}</AlertTitle>
        <AlertDescription>Return to the dashboard and try again.</AlertDescription>
      </Alert>
    )
  }

  if (agent && agent.status !== "online") {
    return (
      <Alert className="mx-auto max-w-lg mt-12">
        <WifiOff />
        <AlertTitle>Agent is offline</AlertTitle>
        <AlertDescription>
          {agent.displayName ?? agent.hostname} is not currently connected.
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="flex h-[calc(100vh-7rem)] flex-col overflow-hidden rounded-lg border">
      <FileBrowser agentId={agentId} />
    </div>
  )
}

export default function FileBrowserPage() {
  return (
    <Suspense
      fallback={
        <div className="flex h-[calc(100vh-7rem)] flex-col gap-3">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="flex-1 w-full" />
        </div>
      }
    >
      <FileBrowserContent />
    </Suspense>
  )
}
