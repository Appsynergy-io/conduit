"use client"

import { AlertCircle, CheckCircle2, Fingerprint, Info } from "lucide-react"
import { useRouter } from "next/navigation"
import { useEffect, useState } from "react"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"

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

type Step = "token" | "configure" | "passkey" | "complete"

export default function SetupPage() {
  const router = useRouter()
  const [step, setStep] = useState<Step>("token")
  const [setupToken, setSetupToken] = useState("")
  const [domain, setDomain] = useState("")
  const [adminEmail, setAdminEmail] = useState("")
  const [orgName, setOrgName] = useState("")
  const [firstName, setFirstName] = useState("")
  const [lastName, setLastName] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)
  const [passkeyRegistered, setPasskeyRegistered] = useState(false)
  const [passkeySkipped, setPasskeySkipped] = useState(false)
  const [webauthnAvailable, setWebauthnAvailable] = useState(false)

  useEffect(() => {
    setWebauthnAvailable(
      typeof window !== "undefined" && !!window.PublicKeyCredential
    )
  }, [])

  async function handleConfigure(e: React.FormEvent) {
    e.preventDefault()
    setError(null)
    setLoading(true)

    try {
      const res = await fetch("/api/v1/setup/configure", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          setupToken,
          domain,
          adminEmail,
          organizationName: orgName,
          firstName,
          lastName,
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
        body: JSON.stringify({ email: adminEmail, password: setupToken }),
      })

      if (!loginRes.ok) {
        // Login failed -- skip passkey, go straight to finalization
        await finalizeSetup()
        return
      }

      // Move to passkey registration step
      setStep("passkey")
    } catch {
      setError("Unable to reach the server.")
    } finally {
      setLoading(false)
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

      // Decode base64url fields to ArrayBuffers for the browser API
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
                <Label htmlFor="setupToken">Setup Token</Label>
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
            <form onSubmit={handleConfigure} className="space-y-4">
              {error && (
                <Alert variant="destructive">
                  <AlertCircle className="h-4 w-4" />
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              )}
              <div className="space-y-2">
                <Label htmlFor="domain">Domain</Label>
                <Input
                  id="domain"
                  required
                  value={domain}
                  onChange={(e) => setDomain(e.target.value)}
                  placeholder="conduit.example.com"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="orgName">Organization Name</Label>
                <Input
                  id="orgName"
                  required
                  value={orgName}
                  onChange={(e) => setOrgName(e.target.value)}
                  placeholder="Acme Corp"
                />
              </div>
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="firstName">First Name</Label>
                  <Input
                    id="firstName"
                    required
                    value={firstName}
                    onChange={(e) => setFirstName(e.target.value)}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="lastName">Last Name</Label>
                  <Input
                    id="lastName"
                    required
                    value={lastName}
                    onChange={(e) => setLastName(e.target.value)}
                  />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="adminEmail">Admin Email</Label>
                <Input
                  id="adminEmail"
                  type="email"
                  required
                  value={adminEmail}
                  onChange={(e) => setAdminEmail(e.target.value)}
                  placeholder="admin@example.com"
                />
              </div>
              <Button type="submit" className="w-full" disabled={loading}>
                {loading ? "Setting up..." : "Complete Setup"}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
