"use client"

import { useCallback, useEffect, useState } from "react"

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

export function useAuth() {
  const [auth, setAuth] = useState<AuthState>(initialState)

  // On mount, check if we have a valid session cookie
  useEffect(() => {
    let cancelled = false
    async function checkSession() {
      try {
        const res = await fetch("/api/v1/auth/me")
        if (res.ok) {
          const data = await res.json()
          if (!cancelled) {
            setAuth({
              userId: data.userId,
              tenantId: data.tenantId,
              roles: data.roles ?? [],
              isAuthenticated: true,
              loading: false,
            })
          }
          return
        }
      } catch {
        // Server unreachable — not authenticated
      }
      if (!cancelled) {
        setAuth({ ...initialState, loading: false })
      }
    }
    checkSession()
    return () => {
      cancelled = true
    }
  }, [])

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
