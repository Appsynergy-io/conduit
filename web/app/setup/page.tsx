"use client"

import { AlertCircle, CheckCircle2, Fingerprint, Info } from "lucide-react"
import { useRouter } from "next/navigation"
import { useEffect, useState } from "react"
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

const configureSchema = z.object({
  domain: z.string().min(1, "Domain is required"),
  organizationName: z.string().min(1, "Organization name is required"),
  firstName: z.string().min(1, "First name is required"),
  lastName: z.string().min(1, "Last name is required"),
  adminEmail: z.string().min(1, "Email is required").email("Enter a valid email address"),
})

type ConfigureFormValues = z.infer<typeof configureSchema>

type Step = "token" | "configure" | "passkey" | "complete"

export default function SetupPage() {
  const router = useRouter()
  const [step, setStep] = useState<Step>("token")
  const [setupToken, setSetupToken] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [passkeyRegistered, setPasskeyRegistered] = useState(false)
  const [passkeySkipped, setPasskeySkipped] = useState(false)
  const [webauthnAvailable, setWebauthnAvailable] = useState(false)

  const configForm = useForm<ConfigureFormValues>({
    resolver: standardSchemaResolver(configureSchema),
    defaultValues: {
      domain: "",
      organizationName: "",
      firstName: "",
      lastName: "",
      adminEmail: "",
    },
  })

  useEffect(() => {
    setWebauthnAvailable(
      typeof window !== "undefined" && !!window.PublicKeyCredential
    )
  }, [])

  async function onConfigureSubmit(values: ConfigureFormValues) {
    setError(null)

    try {
      const res = await fetch("/api/v1/setup/configure", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          setupToken,
          ...values,
        }),
      })

      if (!res.ok) {
        const body = await res.json().catch(() => null)
        setError(body?.detail ?? "Setup failed.")
        return
      }

      // Auto-login with the admin email and setup token
      const loginRes = await fetch("/api/v1/auth/password/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: values.adminEmail, password: setupToken }),
      })

      if (!loginRes.ok) {
        await finalizeSetup()
        return
      }

      setStep("passkey")
    } catch {
      setError("Unable to reach the server.")
    }
  }

  async function handlePasskeyRegister() {
    setError(null)
    setLoading(true)

    try {
      const beginRes = await fetch("/api/v1/auth/webauthn/register/begin", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
      })

      if (!beginRes.ok) {
        const body = await beginRes.json().catch(() => null)
        setError(body?.detail ?? "Failed to start passkey registration.")
        setLoading(false)
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
          })
        )
      }

      const credential = (await navigator.credentials.create({
        publicKey: publicKeyOptions,
      })) as PublicKeyCredential | null

      if (!credential) {
        setError("Passkey registration was cancelled.")
        setLoading(false)
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
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(finishBody),
      })

      if (!finishRes.ok) {
        const body = await finishRes.json().catch(() => null)
        setError(body?.detail ?? "Passkey registration failed.")
        setLoading(false)
        return
      }

      setPasskeyRegistered(true)
      await finalizeSetup()
    } catch (err) {
      if (err instanceof DOMException && err.name === "NotAllowedError") {
        setError("Passkey registration was cancelled or timed out.")
      } else {
        setError("Passkey registration failed. You can skip and register one later.")
      }
    } finally {
      setLoading(false)
    }
  }

  async function handleSkipPasskey() {
    setLoading(true)
    setError(null)
    setPasskeySkipped(true)
    try {
      await finalizeSetup()
    } catch {
      setError("Unable to reach the server.")
    } finally {
      setLoading(false)
    }
  }

  async function finalizeSetup() {
    const passkeyRes = await fetch("/api/v1/setup/passkey", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ setupToken }),
    })

    if (!passkeyRes.ok) {
      const body = await passkeyRes.json().catch(() => null)
      setError(body?.detail ?? "Failed to finalize setup.")
      return
    }

    setStep("complete")
  }

  if (step === "complete") {
    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <Card className="w-full max-w-md">
          <CardHeader className="text-center">
            <CheckCircle2 className="mx-auto h-12 w-12 text-green-500" />
            <CardTitle className="mt-4 text-2xl">Setup Complete</CardTitle>
            <CardDescription>
              {passkeyRegistered
                ? "Conduit is ready. Your passkey has been registered."
                : "Conduit is ready. Sign in with your admin credentials."}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {passkeySkipped && (
              <Alert>
                <Info className="h-4 w-4" />
                <AlertDescription>
                  Passkey was not registered. You can register one later in Settings for stronger security.
                </AlertDescription>
              </Alert>
            )}
            <Button className="w-full" onClick={() => router.push("/login")}>
              Go to Login
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  if (step === "passkey") {
    return (
      <div className="flex min-h-screen items-center justify-center px-4">
        <Card className="w-full max-w-md">
          <CardHeader className="text-center">
            <Fingerprint className="mx-auto h-12 w-12 text-primary" />
            <CardTitle className="mt-4 text-2xl">Register a Passkey</CardTitle>
            <CardDescription>
              Passkeys provide phishing-resistant authentication. Register one now for secure access to your Conduit instance.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {error && (
              <Alert variant="destructive">
                <AlertCircle className="h-4 w-4" />
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}

            {!webauthnAvailable && (
              <Alert>
                <Info className="h-4 w-4" />
                <AlertDescription>
                  Your browser does not support passkeys. You can skip this step and use password authentication.
                </AlertDescription>
              </Alert>
            )}

            <Button
              className="w-full"
              disabled={loading || !webauthnAvailable}
              onClick={handlePasskeyRegister}
            >
              <Fingerprint className="mr-2 h-4 w-4" />
              {loading ? "Registering passkey..." : "Register Passkey"}
            </Button>

            <Button
              variant="outline"
              className="w-full"
              disabled={loading}
              onClick={handleSkipPasskey}
            >
              Skip for now
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <Card className="w-full max-w-md">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl font-bold tracking-tight">Conduit Setup</CardTitle>
          <CardDescription>
            {step === "token"
              ? "Enter the setup token printed in the server console."
              : "Configure your Conduit instance."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {step === "token" ? (
            <div className="space-y-4">
              {error && (
                <Alert variant="destructive">
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              <div className="space-y-2">
                <label htmlFor="setupToken" className="text-sm font-medium leading-none">
                  Setup Token
                </label>
                <Input
                  id="setupToken"
                  type="password"
                  value={setupToken}
                  onChange={(e) => setSetupToken(e.target.value)}
                  placeholder="Paste the token from your terminal"
                />
              </div>
              <Button
                className="w-full"
                disabled={!setupToken}
                onClick={() => {
                  setError(null)
                  setStep("configure")
                }}
              >
                Continue
              </Button>
            </div>
          ) : (
            <Form {...configForm}>
              <form onSubmit={configForm.handleSubmit(onConfigureSubmit)} className="space-y-4">
                {error && (
                  <Alert variant="destructive">
                    <AlertCircle className="h-4 w-4" />
                    <AlertDescription>{error}</AlertDescription>
                  </Alert>
                )}
                <FormField
                  control={configForm.control}
                  name="domain"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Domain</FormLabel>
                      <FormControl>
                        <Input placeholder="conduit.example.com" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={configForm.control}
                  name="organizationName"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Organization Name</FormLabel>
                      <FormControl>
                        <Input placeholder="Acme Corp" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <div className="grid grid-cols-2 gap-4">
                  <FormField
                    control={configForm.control}
                    name="firstName"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>First Name</FormLabel>
                        <FormControl>
                          <Input {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={configForm.control}
                    name="lastName"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Last Name</FormLabel>
                        <FormControl>
                          <Input {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <FormField
                  control={configForm.control}
                  name="adminEmail"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Admin Email</FormLabel>
                      <FormControl>
                        <Input type="email" placeholder="admin@example.com" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <Button type="submit" className="w-full" disabled={configForm.formState.isSubmitting}>
                  {configForm.formState.isSubmitting ? "Setting up..." : "Complete Setup"}
                </Button>
              </form>
            </Form>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
