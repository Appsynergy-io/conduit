"use client"

import { AlertCircle, Fingerprint } from "lucide-react"
import { useRouter } from "next/navigation"
import { useEffect, useState } from "react"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { useAuth } from "@/hooks/use-auth"

function base64urlToBuffer(base64url: string): ArrayBuffer {
  const base64 = base64url.replace(/-/g, "+").replace(/_/g, "/")
  const pad = base64.length % 4 === 0 ? "" : "=".repeat(4 - (base64.length % 4))
  const binary = atob(base64 + pad)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes.buffer
}

function bufferToBase64url(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ""
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "")
}

export default function LoginPage() {
  const router = useRouter()
  const { login } = useAuth()
  const [email, setEmail] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [passkeyLoading, setPasskeyLoading] = useState(false)
  const [webauthnAvailable, setWebauthnAvailable] = useState(false)

  useEffect(() => {
    setWebauthnAvailable(
      typeof window !== "undefined" && !!window.PublicKeyCredential
    )
  }, [])

  async function handlePasskeyLogin() {
    if (!email) {
      setError("Enter your email address first.")
      return
    }
    setError(null)
    setPasskeyLoading(true)

    try {
      const beginRes = await fetch("/api/v1/auth/webauthn/login/begin", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email }),
      })

      if (!beginRes.ok) {
        const body = await beginRes.json().catch(() => null)
        setError(body?.detail ?? "Failed to start passkey authentication.")
        return
      }

      const options = await beginRes.json()

      // Decode base64url fields to ArrayBuffers for the browser API
      const publicKeyOptions: PublicKeyCredentialRequestOptions = {
        ...options.publicKey,
        challenge: base64urlToBuffer(options.publicKey.challenge),
      }

      if (options.publicKey.allowCredentials) {
        publicKeyOptions.allowCredentials = options.publicKey.allowCredentials.map(
          (cred: { id: string; type: string; transports?: string[] }) => ({
            ...cred,
            id: base64urlToBuffer(cred.id),
          })
        )
      }

      const credential = (await navigator.credentials.get({
        publicKey: publicKeyOptions,
      })) as PublicKeyCredential | null

      if (!credential) {
        setError("Passkey authentication was cancelled.")
        return
      }

      const assertionResponse = credential.response as AuthenticatorAssertionResponse

      const finishBody = {
        id: credential.id,
        rawId: bufferToBase64url(credential.rawId),
        type: credential.type,
        response: {
          authenticatorData: bufferToBase64url(assertionResponse.authenticatorData),
          clientDataJSON: bufferToBase64url(assertionResponse.clientDataJSON),
          signature: bufferToBase64url(assertionResponse.signature),
          userHandle: assertionResponse.userHandle
            ? bufferToBase64url(assertionResponse.userHandle)
            : undefined,
        },
      }

      const finishRes = await fetch("/api/v1/auth/webauthn/login/finish", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(finishBody),
      })

      if (!finishRes.ok) {
        const body = await finishRes.json().catch(() => null)
        setError(body?.detail ?? "Passkey verification failed.")
        return
      }

      const data = await finishRes.json()
      login(data.user.id, data.user.tenantId, data.user.roles ?? [])
      router.push("/dashboard")
    } catch (err) {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setError("Passkey authentication was cancelled or timed out.")
      } else {
        setError("Passkey authentication failed. Try signing in with a password.")
      }
    } finally {
      setPasskeyLoading(false)
    }
  }

  async function handlePasswordLogin(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)

    try {
      const res = await fetch("/api/v1/auth/password/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, password }),
      })

      if (!res.ok) {
        const body = await res.json().catch(() => null)
        setError(body?.detail ?? "Invalid email or password.")
        return
      }

      const data = await res.json()
      login(data.user.id, data.user.tenantId, data.user.roles ?? [])
      router.push("/dashboard")
    } catch {
      setError("Unable to reach the server. Check your connection.")
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-bold tracking-tight">Conduit</CardTitle>
          <CardDescription>Sign in to your account</CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            {error && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            <div className="space-y-2">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                autoComplete="email webauthn"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="admin@example.com"
              />
            </div>

            {webauthnAvailable && (
              <Button
                type="button"
                className="w-full"
                disabled={passkeyLoading || loading}
                onClick={handlePasskeyLogin}
              >
                <Fingerprint className="mr-2 h-4 w-4" />
                {passkeyLoading ? "Waiting for passkey..." : "Sign in with passkey"}
              </Button>
            )}

            <div className="flex items-center gap-3">
              <Separator className="flex-1" />
              <span className="text-xs text-muted-foreground">or</span>
              <Separator className="flex-1" />
            </div>

            <form onSubmit={handlePasswordLogin} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <Input
                  id="password"
                  type="password"
                  autoComplete="current-password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="Setup token"
                />
              </div>
              <Button
                type="submit"
                variant="outline"
                className="w-full"
                disabled={loading || passkeyLoading}
              >
                {loading ? "Signing in..." : "Sign in with password"}
              </Button>
            </form>

            <p className="text-center text-xs text-muted-foreground">
              Dev mode: use your admin email and setup token as the password.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
