"use client"

import { useRouter } from "next/navigation"
import { useEffect } from "react"
import { useAuth } from "@/hooks/use-auth"

export function AuthGuard({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, loading } = useAuth()
  const router = useRouter()

  useEffect(() => {
    if (!loading && !isAuthenticated) {
      router.replace("/login")
    }
  }, [isAuthenticated, loading, router])

  if (loading || !isAuthenticated) {
    return null
  }

  return <>{children}</>
}
