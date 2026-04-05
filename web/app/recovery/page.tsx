"use client"

import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { AlertCircle, CheckCircle2, Fingerprint, KeyRound, ShieldCheck } from "lucide-react"
import Link from "next/link"
import { useRouter } from "next/navigation"
import { useCallback, useState } from "react"
import { useForm } from "react-hook-form"
import { z } from "zod"
import { CaptchaWidget } from "@/components/captcha-widget"
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
import type { CaptchaSolution } from "@/lib/captcha"

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

const recoverySchema = z.object({
  email: z.string().min(1, "Email is required").email("Enter a valid email address"),
  code: z
    .string()
    .min(1, "Recovery code is required")
    .transform((s) => s.trim().toLowerCase()),
})

type RecoveryFormValues = z.infer<typeof recoverySchema>

type Step = "verify" | "passkey" | "complete"

export default function RecoveryPage() {
  const router = useRouter()
  const [step, setStep] = useState<Step>("verify")
  const [error, setError] = useState<string | null>(null)
  const [captchaSolution, setCaptchaSolution] = useState<CaptchaSolution | null>(null)
  const [captchaNonce, setCaptchaNonce] = useState(0)
  const [submitting, setSubmitting] = useState(false)
  const [scopedToken, setScopedToken] = useState<string | null>(null)
  const [remaining, setRemaining] = useState<number | null>(null)
  const [registering, setRegistering] = useState(false)

  const form = useForm<RecoveryFormValues>({
    resolver: standardSchemaResolver(recoverySchema),
    defaultValues: { email: "", code: "" },
  })

  const handleCaptchaSolved = useCallback((sol: CaptchaSolution) => {
    setCaptchaSolution(sol)
  }, [])

  async function onVerifySubmit(values: RecoveryFormValues) {
    if (!captchaSolution) {
      setError("Please wait for verification to complete.")
      return
    }
    setError(null)
    setSubmitting(true)

    try {
      const res = await fetch("/api/v1/auth/recovery/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          email: values.email,
          code: values.code,
          captcha: captchaSolution,
        }),
      })

      if (!res.ok) {
        // Identical error for every failure mode — no enumeration (OWASP A07).
        // Challenge is single-use on the server; clear it so the widget
        // fetches a fresh one for the retry.
        setError("Invalid email or recovery code.")
        setCaptchaSolution(null)
        setCaptchaNonce((n) => n + 1)
        return
      }

      const data = await res.json()
      setScopedToken(data.token)
      setRemaining(typeof data.remaining === "number" ? data.remaining : null)
      setStep("passkey")
    } catch {
      setError("Unable to verify the recovery code. Please try again.")
      setCaptchaSolution(null)
      setCaptchaNonce((n) => n + 1)
    } finally {
      setSubmitting(false)
    }
  }

  async function handlePasskeyRegister() {
    if (!scopedToken) {
      setError("Session expired. Start over.")
      setStep("verify")
      return
    }
    setError(null)
    setRegistering(true)

    try {
      const beginRes = await fetch("/api/v1/auth/webauthn/register/begin", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${scopedToken}`,
        },
      })

      if (!beginRes.ok) {
        const body = await beginRes.json().catch(() => null)
        setError(body?.detail ?? "Failed to start passkey registration.")
        return
      }

      const options = await beginRes.json()

      const publicKeyOptions: PublicKeyCredentialCreationOptions = {
        ...options.publicKey,
        challenge: base64urlToBuffer(options.publicKey.challenge),
        user: {
          ...options.publicKey.user,
          id: base64urlToBuffer(options.publicKey.user.id),
        },
      }

      if (options.publicKey.excludeCredentials) {
        publicKeyOptions.excludeCredentials = options.publicKey.excludeCredentials.map(
          (cred: { id: string; type: string; transports?: string[] }) => ({
            ...cred,
            id: base64urlToBuffer(cred.id),
          }),
        )
      }

      const credential = (await navigator.credentials.create({
        publicKey: publicKeyOptions,
      })) as PublicKeyCredential | null

      if (!credential) {
        setError("Passkey registration was cancelled.")
        return
      }

      const attestationResponse = credential.response as AuthenticatorAttestationResponse

      const finishBody = {
        id: credential.id,
        rawId: bufferToBase64url(credential.rawId),
        type: credential.type,
        response: {
          attestationObject: bufferToBase64url(attestationResponse.attestationObject),
          clientDataJSON: bufferToBase64url(attestationResponse.clientDataJSON),
        },
      }

      const finishRes = await fetch("/api/v1/auth/webauthn/register/finish", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${scopedToken}`,
        },
        body: JSON.stringify(finishBody),
      })

      if (!finishRes.ok) {
        const body = await finishRes.json().catch(() => null)
        setError(body?.detail ?? "Passkey registration failed.")
        return
      }

      setStep("complete")
    } catch (err) {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setError("Passkey registration was cancelled or timed out.")
      } else {
        setError("Passkey registration failed.")
      }
    } finally {
      setRegistering(false)
    }
  }

  if (step === "complete") {
    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <Card className="w-full max-w-sm">
          <CardHeader className="text-center">
            <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-green-100 dark:bg-green-900">
              <CheckCircle2 className="h-6 w-6 text-green-600 dark:text-green-400" />
            </div>
            <CardTitle className="text-xl font-bold">Passkey registered</CardTitle>
            <CardDescription>You can now sign in with your new passkey.</CardDescription>
          </CardHeader>
          <CardContent>
            <Button className="w-full" onClick={() => router.push("/login")}>
              Continue to sign in
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (step === "passkey") {
    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <Card className="w-full max-w-sm">
          <CardHeader className="text-center">
            <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
              <ShieldCheck className="h-6 w-6 text-primary" />
            </div>
            <CardTitle className="text-xl font-bold">Register a new passkey</CardTitle>
            <CardDescription>
              Recovery code accepted. Register a passkey on this device to restore access.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {error && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            {remaining !== null && (
              <Alert>
                <KeyRound className="h-4 w-4" />
                <AlertDescription>
                  {remaining === 0
                    ? "That was your last recovery code. Generate new ones after signing in."
                    : `${remaining} recovery code${remaining === 1 ? "" : "s"} remaining.`}
                </AlertDescription>
              </Alert>
            )}
            <Button
              className="w-full"
              size="lg"
              onClick={handlePasskeyRegister}
              disabled={registering}
            >
              <Fingerprint className="mr-2 h-4 w-4" />
              {registering ? "Waiting for passkey..." : "Register passkey"}
            </Button>
            <p className="text-center text-xs text-muted-foreground">
              This link expires in 5 minutes.
            </p>
          </CardContent>
        </Card>
      </div>
    )
  }

  // step === "verify"
  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-sm">
        <CardHeader className="text-center">
          <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
            <KeyRound className="h-6 w-6 text-muted-foreground" />
          </div>
          <CardTitle className="text-2xl font-bold tracking-tight">Recover access</CardTitle>
          <CardDescription>
            Lost your passkey? Enter your email and a one-time recovery code.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(onVerifySubmit)} className="space-y-4">
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
                        autoComplete="email"
                        placeholder="admin@example.com"
                        disabled={submitting}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="code"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Recovery code</FormLabel>
                    <FormControl>
                      <Input
                        type="text"
                        autoComplete="one-time-code"
                        autoCapitalize="none"
                        spellCheck={false}
                        placeholder="xxxx-xxxx-xxxx-xxxx"
                        className="font-mono tracking-wider"
                        disabled={submitting}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <CaptchaWidget
                key={captchaNonce}
                endpoint="/auth/recovery/verify"
                onSolved={handleCaptchaSolved}
              />

              <Button type="submit" className="w-full" disabled={submitting || !captchaSolution}>
                {submitting
                  ? "Verifying..."
                  : captchaSolution
                    ? "Verify recovery code"
                    : "Waiting for verification..."}
              </Button>

              <div className="text-center">
                <Link href="/login" className="text-sm text-muted-foreground hover:text-foreground">
                  Back to sign in
                </Link>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>
    </div>
  )
}
