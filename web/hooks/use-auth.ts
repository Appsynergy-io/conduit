"use client";

import { useCallback, useEffect, useState } from "react";

interface AuthState {
  token: string | null;
  tenantId: string | null;
  userId: string | null;
  roles: string[];
  isAuthenticated: boolean;
}

const STORAGE_KEY = "conduit_auth";

function loadAuth(): AuthState {
  if (typeof window === "undefined") {
    return { token: null, tenantId: null, userId: null, roles: [], isAuthenticated: false };
  }
  try {
    const stored = sessionStorage.getItem(STORAGE_KEY);
    if (stored) {
      const parsed = JSON.parse(stored);
      return { ...parsed, isAuthenticated: !!parsed.token };
    }
  } catch {
    // Corrupted storage — clear it
    sessionStorage.removeItem(STORAGE_KEY);
  }
  return { token: null, tenantId: null, userId: null, roles: [], isAuthenticated: false };
}

export function useAuth() {
  const [auth, setAuth] = useState<AuthState>(loadAuth);

  // Re-sync from storage on mount (handles multiple tabs)
  useEffect(() => {
    setAuth(loadAuth());
  }, []);

  const login = useCallback(
    (token: string, tenantId: string, userId: string, roles: string[]) => {
      const state: AuthState = { token, tenantId, userId, roles, isAuthenticated: true };
      sessionStorage.setItem(STORAGE_KEY, JSON.stringify(state));
      setAuth(state);
    },
    [],
  );

  const logout = useCallback(() => {
    sessionStorage.removeItem(STORAGE_KEY);
    setAuth({ token: null, tenantId: null, userId: null, roles: [], isAuthenticated: false });
  }, []);

  return { ...auth, login, logout };
}
