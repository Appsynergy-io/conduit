"use client"

import { AlertCircle, CheckCircle2, Fingerprint, Monitor, User } from "lucide-react"
import { useRouter, useSearchParams } from "next/navigation"
import { Suspense, useCallback, useEffect, useState } from "react"
import { useForm } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { z } from "zod"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
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

const loginSchema = z.object({
  email: z.string().min(1, "Email is required").email("Enter a valid email address"),
  password: z.string(),
})

type LoginFormValues = z.infer<typeof loginSchema>

function LoginContent() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { login, isAuthenticated } = useAuth()
  const savedEmail = typeof window !== "undefined" ? localStorage.getItem("conduit_email") ?? "" : ""

  const [error, setError] = useState<string | null>(null)
  const [passkeyLoading, setPasskeyLoading] = useState(false)
  const [webauthnAvailable, setWebauthnAvailable] = useState(false)
  const [passwordAuth, setPasswordAuth] = useState(false)
  const [showEmailInput, setShowEmailInput] = useState(!savedEmail)
  const [deviceCode, setDeviceCode] = useState<string | null>(null)
  const [deviceAuthorizing, setDeviceAuthorizing] = useState(false)
  const [deviceAuthorized, setDeviceAuthorized] = useState(false)

  const form = useForm<LoginFormValues>({
    resolver: standardSchemaResolver(loginSchema),
    defaultValues: { email: savedEmail, password: "" },
  })

  // Check for device flow param
  useEffect(() => {
    const code = searchParams.get("device")
    if (code) {
      setDeviceCode(code)
    }
  }, [searchParams])

  // If already authenticated and there's a device code, show authorize prompt
  useEffect(() => {
    if (isAuthenticated && deviceCode && !deviceAuthorized) {
      // Already logged in, just need to authorize the device
    }
  }, [isAuthenticated, deviceCode, deviceAuthorized])

  const authorizeDevice = useCallback(async () => {
    if (!deviceCode) return
    setDeviceAuthorizing(true)
    setError(null)
    try {
      const res = await fetch("/api/v1/auth/device/authorize", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ userCode: deviceCode }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => null)
        setError(body?.detail ?? "Failed to authorize device.")
        return
      }
      setDeviceAuthorized(true)
    } catch {
      setError("Failed to authorize device.")
    } finally {
      setDeviceAuthorizing(false)
    }
  }, [deviceCode])

  useEffect(() => {
    setWebauthnAvailable(
      typeof window !== "undefined" && !!window.PublicKeyCredential
    )
    fetch("/api/v1/auth/config")
      .then((r) => r.json())
      .then((d) => setPasswordAuth(d.passwordAuth === true))
      .catch(() => {})
  }, [])

  async function handlePasskeyLogin() {
    const email = form.getValues("email")
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
      localStorage.setItem("conduit_email", form.getValues("email"))
      login(data.user.id, data.user.tenantId, data.user.roles ?? [])
      if (!deviceCode) {
        router.push("/dashboard")
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setError("Passkey authentication was cancelled or timed out.")
      } else {
        setError("Passkey authentication failed.")
      }
    } finally {
      setPasskeyLoading(false)
    }
  }

  async function onPasswordSubmit(values: LoginFormValues) {
    setError(null)

    try {
      const res = await fetch("/api/v1/auth/password/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: values.email, password: values.password }),
      })

      if (!res.ok) {
        const body = await res.json().catch(() => null)
        setError(body?.detail ?? "Invalid email or password.")
        return
      }

      const data = await res.json()
      localStorage.setItem("conduit_email", values.email)
      login(data.user.id, data.user.tenantId, data.user.roles ?? [])
      if (!deviceCode) {
        router.push("/dashboard")
      }
    } catch {
      setError("Unable to reach the server. Check your connection.")
    }
  }

  // Device authorization UI (shown after login when device code is present)
  if (isAuthenticated && deviceCode) {
    if (deviceAuthorized) {
      return (
        <div className="flex min-h-screen items-center justify-center px-4">
          <Card className="w-full max-w-sm">
            <CardHeader className="text-center">
              <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-green-100 dark:bg-green-900">
                <CheckCircle2 className="h-6 w-6 text-green-600 dark:text-green-400" />
              </div>
              <CardTitle className="text-xl font-bold">Device Authorized</CardTitle>
              <CardDescription>
                You can close this window. The CLI is now authenticated.
              </CardDescription>
            </CardHeader>
          </Card>
        </div>
      )
    }

    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <Card className="w-full max-w-sm">
          <CardHeader className="text-center">
            <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
              <Monitor className="h-6 w-6 text-muted-foreground" />
            </div>
            <CardTitle className="text-xl font-bold">Authorize CLI</CardTitle>
            <CardDescription>
              A CLI session is requesting access to your account.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {error && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <div className="rounded-lg border bg-muted/50 p-4 text-center">
              <p className="text-xs text-muted-foreground">Confirm this code matches your CLI</p>
              <p className="mt-1 font-mono text-2xl font-bold tracking-widest">{deviceCode}</p>
            </div>
            <Button
              className="w-full"
              onClick={authorizeDevice}
              disabled={deviceAuthorizing}
            >
              {deviceAuthorizing ? "Authorizing..." : "Authorize this device"}
            </Button>
            <Button
              variant="outline"
              className="w-full"
              onClick={() => { setDeviceCode(null); router.push("/dashboard") }}
            >
              Cancel
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  // Returning user — show welcome back with one-click passkey
  if (savedEmail && !showEmailInput) {
    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <Card className="w-full max-w-sm">
          <CardHeader className="text-center">
            <CardTitle className="text-2xl font-bold tracking-tight">Welcome back</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-5">
              {error && (
                <Alert variant="destructive">
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}

              <div className="flex items-center justify-center">
                <div className="flex items-center gap-3 rounded-full border bg-muted/50 px-4 py-2">
                  <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary/10">
                    <User className="h-4 w-4 text-primary" />
                  </div>
                  <span className="text-sm font-medium">{savedEmail}</span>
                </div>
              </div>

              {webauthnAvailable && (
                <Button
                  type="button"
                  className="w-full"
                  size="lg"
                  disabled={passkeyLoading}
                  onClick={handlePasskeyLogin}
                >
                  <Fingerprint className="mr-2 h-4 w-4" />
                  {passkeyLoading ? "Waiting for passkey..." : "Sign in with passkey"}
                </Button>
              )}

              <button
                type="button"
                className="block w-full text-center text-sm text-muted-foreground hover:text-foreground"
                onClick={() => {
                  localStorage.removeItem("conduit_email")
                  form.setValue("email", "")
                  setShowEmailInput(true)
                  setError(null)
                }}
              >
                Not you? Use a different account
              </button>
            </div>
          </CardContent>
        </Card>
      </div>
    )
  }

  // Fresh login — email input + passkey/password
  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-bold tracking-tight">Conduit</CardTitle>
          <CardDescription>Sign in to your account</CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <div className="space-y-4">
              {error && (
                <Alert variant="destructive">
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}

              <FormField
                control={form.control}
                name="email"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Email</FormLabel>
                    <FormControl>
                      <Input
                        type="email"
                        autoComplete="email webauthn"
                        placeholder="admin@example.com"
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {webauthnAvailable && (
                <Button
                  type="button"
                  className="w-full"
                  disabled={passkeyLoading || form.formState.isSubmitting}
                  onClick={handlePasskeyLogin}
                >
                  <Fingerprint className="mr-2 h-4 w-4" />
                  {passkeyLoading ? "Waiting for passkey..." : "Sign in with passkey"}
                </Button>
              )}

              {passwordAuth && (
                <>
                  <div className="flex items-center gap-3">
                    <Separator className="flex-1" />
                    <span className="text-xs text-muted-foreground">or</span>
                    <Separator className="flex-1" />
                  </div>

                  <form onSubmit={form.handleSubmit(onPasswordSubmit)} className="space-y-4">
                    <FormField
                      control={form.control}
                      name="password"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>Password</FormLabel>
                          <FormControl>
                            <Input
                              type="password"
                              autoComplete="current-password"
                              placeholder="Setup token"
                              {...field}
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <Button
                      type="submit"
                      variant="outline"
                      className="w-full"
                      disabled={form.formState.isSubmitting || passkeyLoading}
                    >
                      {form.formState.isSubmitting ? "Signing in..." : "Sign in with password"}
                    </Button>
                  </form>

                  <p className="text-center text-xs text-muted-foreground">
                    Dev mode: use your admin email and setup token as the password.
                  </p>
                </>
              )}
            </div>
          </Form>
        </CardContent>
      </Card>
    </div>
  )
}

export default function LoginPage() {
  return (
    <Suspense>
      <LoginContent />
    </Suspense>
  )
}
