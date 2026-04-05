"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { refreshToken } from "@/lib/refresh"

interface AuthState {
  userId: string | null
  tenantId: string | null
  roles: string[]
  isAuthenticated: boolean
  loading: boolean
}

const initialState: AuthState = {
  userId: null,
  tenantId: null,
  roles: [],
  isAuthenticated: false,
  loading: true,
}

// Poll interval for session validity check (2 minutes)
const AUTH_CHECK_INTERVAL = 2 * 60 * 1000

export function useAuth() {
  const [auth, setAuth] = useState<AuthState>(initialState)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const checkSession = useCallback(async () => {
    try {
      let res = await fetch("/api/v1/auth/me")

      // Access token expired — try silent refresh before giving up.
      if (res.status === 401) {
        const refreshed = await refreshToken()
        if (refreshed) {
          res = await fetch("/api/v1/auth/me")
        }
      }

      if (res.ok) {
        const data = await res.json()
        setAuth({
          userId: data.userId,
          tenantId: data.tenantId,
          roles: data.roles ?? [],
          isAuthenticated: true,
          loading: false,
        })
        return true
      }
    } catch {
      // Server unreachable — not authenticated
    }
    setAuth({ ...initialState, loading: false })
    return false
  }, [])

  // Initial check + periodic polling
  useEffect(() => {
    let cancelled = false

    async function init() {
      const ok = await checkSession()
      if (cancelled) return
      // Only start polling if authenticated
      if (ok) {
        intervalRef.current = setInterval(checkSession, AUTH_CHECK_INTERVAL)
      }
    }

    init()
    return () => {
      cancelled = true
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [checkSession])

  // Start/stop polling when auth state changes
  useEffect(() => {
    if (auth.isAuthenticated && !auth.loading && !intervalRef.current) {
      intervalRef.current = setInterval(checkSession, AUTH_CHECK_INTERVAL)
    }
    if (!auth.isAuthenticated && intervalRef.current) {
      clearInterval(intervalRef.current)
      intervalRef.current = null
    }
  }, [auth.isAuthenticated, auth.loading, checkSession])

  const login = useCallback((userId: string, tenantId: string, roles: string[]) => {
    setAuth({
      userId,
      tenantId,
      roles,
      isAuthenticated: true,
      loading: false,
    })
  }, [])

  const logout = useCallback(async () => {
    try {
      await fetch("/api/v1/auth/logout", { method: "POST" })
    } catch {
      // Best-effort — clear local state regardless
    }
    setAuth({ ...initialState, loading: false })
  }, [])

  return { ...auth, login, logout }
}
