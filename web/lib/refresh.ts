// Silent token refresh via httpOnly refresh cookie.
//
// The server sets both __Host-conduit_token (15min access) and
// __Host-conduit_refresh (8h refresh) as httpOnly cookies. When the
// access token expires, calling refreshToken() hits POST /auth/refresh
// which reads the refresh cookie and sets new access + refresh cookies.
//
// Deduplication: only one in-flight refresh at a time. Concurrent callers
// share the same promise so the server isn't hammered with N parallel
// refreshes when N fetches all 401 simultaneously.

let inflight: Promise<boolean> | null = null

/**
 * Attempt a silent token refresh. Returns true if successful (new cookies
 * set automatically by the browser), false if the refresh token is also
 * expired and the user must re-authenticate.
 */
export async function refreshToken(): Promise<boolean> {
  if (inflight) return inflight

  inflight = (async () => {
    try {
      const res = await fetch("/api/v1/auth/refresh", { method: "POST" })
      return res.ok
    } catch {
      return false
    } finally {
      inflight = null
    }
  })()

  return inflight
}
