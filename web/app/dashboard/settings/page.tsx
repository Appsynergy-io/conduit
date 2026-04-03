"use client"

import {
  Copy,
  Check,
  Fingerprint,
  KeyRound,
  LogOut,
  Monitor,
  Plus,
  RefreshCw,
  Shield,
  Trash2,
  User,
} from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { useForm } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { z } from "zod"
import { formatDistanceToNow } from "date-fns"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { useAuth } from "@/hooks/use-auth"

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

interface UserProfile {
  id: string
  email: string
  firstName: string
  lastName: string
  displayName?: string | null
  role: string
  status: string
  passkeyCount: number
  lastLoginAt?: string | null
  createdAt: string
}

interface AuthSession {
  id: string
  type: string
  sourceIp?: string | null
  userAgent?: string | null
  createdAt: string
  lastActiveAt: string
  expiresAt: string
}

interface Passkey {
  id: string
  algorithm: string
  algorithmWarning?: string | null
  authenticatorType: string
  displayName?: string | null
  createdAt: string
  lastUsedAt?: string | null
}

interface CIToken {
  id: string
  name: string
  scopes: string[]
  expiresAt?: string | null
  createdAt: string
  lastUsedAt?: string | null
}

// ---------------------------------------------------------------------------
// Schemas
// ---------------------------------------------------------------------------

const profileSchema = z.object({
  firstName: z.string().min(1, "Required").max(100),
  lastName: z.string().min(1, "Required").max(100),
  displayName: z.string().max(255).optional(),
})

const ciTokenSchema = z.object({
  name: z.string().min(1, "Required").max(100),
  scopes: z.string().optional(),
})

type ProfileFormValues = z.infer<typeof profileSchema>
type CITokenFormValues = z.infer<typeof ciTokenSchema>

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

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

export default function SettingsPage() {
  const { userId, roles, isAuthenticated } = useAuth()
  const isAdmin = roles?.some((r: string) =>
    ["platform_owner", "org_owner", "org_admin"].includes(r),
  )

  // State
  const [profile, setProfile] = useState<UserProfile | null>(null)
  const [sessions, setSessions] = useState<AuthSession[]>([])
  const [passkeys, setPasskeys] = useState<Passkey[]>([])
  const [ciTokens, setCITokens] = useState<CIToken[]>([])
  const [loading, setLoading] = useState(true)

  // Dialogs
  const [revokeSessionId, setRevokeSessionId] = useState<string | null>(null)
  const [deletePasskeyId, setDeletePasskeyId] = useState<string | null>(null)
  const [revokeCITokenId, setRevokeCITokenId] = useState<string | null>(null)
  const [showCITokenDialog, setShowCITokenDialog] = useState(false)
  const [createdCIToken, setCreatedCIToken] = useState<string | null>(null)
  const [showRecoveryCodes, setShowRecoveryCodes] = useState(false)
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [copiedToken, setCopiedToken] = useState(false)
  const [profileSaving, setProfileSaving] = useState(false)
  const [profileSaved, setProfileSaved] = useState(false)
  const [registeringPasskey, setRegisteringPasskey] = useState(false)
  const [recoveryCodeCount, setRecoveryCodeCount] = useState<number | null>(null)

  // Forms
  const profileForm = useForm<ProfileFormValues>({
    resolver: standardSchemaResolver(profileSchema),
    defaultValues: { firstName: "", lastName: "", displayName: "" },
  })

  const ciTokenForm = useForm<CITokenFormValues>({
    resolver: standardSchemaResolver(ciTokenSchema),
    defaultValues: { name: "", scopes: "" },
  })

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  const fetchProfile = useCallback(async () => {
    if (!userId) return
    try {
      const res = await fetch(`/api/v1/users/${userId}`)
      if (res.ok) {
        const data = await res.json()
        setProfile(data)
        profileForm.reset({
          firstName: data.firstName || "",
          lastName: data.lastName || "",
          displayName: data.displayName || "",
        })
      }
    } catch {
      /* ignore */
    }
  }, [userId, profileForm])

  const fetchSessions = useCallback(async () => {
    try {
      const res = await fetch("/api/v1/sessions")
      if (res.ok) {
        const data = await res.json()
        setSessions(data.data ?? [])
      }
    } catch {
      /* ignore */
    }
  }, [])

  const fetchPasskeys = useCallback(async () => {
    try {
      const res = await fetch("/api/v1/auth/webauthn/credentials")
      if (res.ok) {
        const data = await res.json()
        setPasskeys(data.items ?? [])
      }
    } catch {
      /* ignore */
    }
  }, [])

  const fetchCITokens = useCallback(async () => {
    try {
      const res = await fetch("/api/v1/auth/ci-tokens")
      if (res.ok) {
        const data = await res.json()
        setCITokens(data.data ?? [])
      }
    } catch {
      /* ignore */
    }
  }, [])

  const fetchRecoveryCodeCount = useCallback(async () => {
    try {
      const res = await fetch("/api/v1/auth/recovery/count")
      if (res.ok) {
        const data = await res.json()
        setRecoveryCodeCount(data.count ?? 0)
      }
    } catch {
      /* ignore */
    }
  }, [])

  useEffect(() => {
    if (!isAuthenticated) return
    Promise.all([fetchProfile(), fetchSessions(), fetchPasskeys(), fetchCITokens(), fetchRecoveryCodeCount()]).finally(() =>
      setLoading(false),
    )
  }, [isAuthenticated, fetchProfile, fetchSessions, fetchPasskeys, fetchCITokens, fetchRecoveryCodeCount])

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  async function onProfileSave(values: ProfileFormValues) {
    if (!userId) return
    setProfileSaving(true)
    try {
      const res = await fetch(`/api/v1/users/${userId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(values),
      })
      if (res.ok) {
        const data = await res.json()
        setProfile(data)
        setProfileSaved(true)
        setTimeout(() => setProfileSaved(false), 2000)
      }
    } finally {
      setProfileSaving(false)
    }
  }

  async function revokeSession(id: string) {
    await fetch(`/api/v1/sessions/${id}`, { method: "DELETE" })
    setRevokeSessionId(null)
    fetchSessions()
  }

  async function deletePasskey(id: string) {
    await fetch(`/api/v1/auth/webauthn/credentials/${id}`, { method: "DELETE" })
    setDeletePasskeyId(null)
    fetchPasskeys()
    fetchProfile()
  }

  async function registerPasskey() {
    setRegisteringPasskey(true)
    try {
      const beginRes = await fetch("/api/v1/auth/webauthn/register/begin", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
      })
      if (!beginRes.ok) return

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
      if (!credential) return

      const attestation = credential.response as AuthenticatorAttestationResponse
      await fetch("/api/v1/auth/webauthn/register/finish", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          id: credential.id,
          rawId: bufferToBase64url(credential.rawId),
          type: credential.type,
          response: {
            attestationObject: bufferToBase64url(attestation.attestationObject),
            clientDataJSON: bufferToBase64url(attestation.clientDataJSON),
          },
        }),
      })

      fetchPasskeys()
      fetchProfile()
    } finally {
      setRegisteringPasskey(false)
    }
  }

  async function createCIToken(values: CITokenFormValues) {
    const body: Record<string, unknown> = { name: values.name }
    if (values.scopes) {
      body.scopes = values.scopes
        .split(",")
        .map((s) => s.trim())
        .filter(Boolean)
    }
    const res = await fetch("/api/v1/auth/ci-tokens", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })
    if (res.ok) {
      const data = await res.json()
      setCreatedCIToken(data.token)
      ciTokenForm.reset()
      fetchCITokens()
    }
  }

  async function revokeCIToken(id: string) {
    await fetch(`/api/v1/auth/ci-tokens/${id}`, { method: "DELETE" })
    setRevokeCITokenId(null)
    fetchCITokens()
  }

  async function generateRecoveryCodes() {
    const res = await fetch("/api/v1/auth/recovery/generate", { method: "POST" })
    if (res.ok) {
      const data = await res.json()
      setRecoveryCodes(data.codes ?? [])
      setRecoveryCodeCount(data.codes?.length ?? 0)
      setShowRecoveryCodes(true)
    }
  }

  function copyToClipboard(text: string) {
    navigator.clipboard.writeText(text)
    setCopiedToken(true)
    setTimeout(() => setCopiedToken(false), 2000)
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  if (loading) {
    return (
      <div className="space-y-6">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
          <p className="text-sm text-muted-foreground">Account and security settings.</p>
        </div>
        <div className="space-y-4">
          {[1, 2, 3, 4].map((k) => (
            <Skeleton key={k} className="h-40 w-full" />
          ))}
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">Account and security settings.</p>
      </div>

      {/* ── Profile ──────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <div className="flex items-center gap-2">
            <User className="h-5 w-5 text-muted-foreground" />
            <CardTitle>Profile</CardTitle>
          </div>
          <CardDescription>Your account information.</CardDescription>
        </CardHeader>
        <CardContent>
          <Form {...profileForm}>
            <form onSubmit={profileForm.handleSubmit(onProfileSave)} className="space-y-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <FormField
                  control={profileForm.control}
                  name="firstName"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>First name</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={profileForm.control}
                  name="lastName"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Last name</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
              <FormField
                control={profileForm.control}
                name="displayName"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Display name</FormLabel>
                    <FormControl>
                      <Input placeholder="Optional" {...field} />
                    </FormControl>
                    <FormDescription>Shown instead of first/last name if set.</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="flex items-center gap-4 pt-2">
                <div className="flex-1 space-y-1">
                  <Label className="text-sm text-muted-foreground">Email</Label>
                  <p className="text-sm font-medium">{profile?.email}</p>
                </div>
                <div className="flex-1 space-y-1">
                  <Label className="text-sm text-muted-foreground">Role</Label>
                  <p>
                    <Badge variant="outline">{profile?.role?.replace(/_/g, " ")}</Badge>
                  </p>
                </div>
              </div>

              <div className="flex justify-end pt-2">
                <Button type="submit" disabled={profileSaving}>
                  {profileSaved ? (
                    <>
                      <Check className="mr-1 h-4 w-4" /> Saved
                    </>
                  ) : profileSaving ? (
                    "Saving..."
                  ) : (
                    "Save changes"
                  )}
                </Button>
              </div>
            </form>
          </Form>
        </CardContent>
      </Card>

      {/* ── Passkeys ─────────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Fingerprint className="h-5 w-5 text-muted-foreground" />
              <CardTitle>Passkeys</CardTitle>
            </div>
            <Button size="sm" onClick={registerPasskey} disabled={registeringPasskey}>
              <Plus className="mr-1 h-4 w-4" />
              {registeringPasskey ? "Registering..." : "Add passkey"}
            </Button>
          </div>
          <CardDescription>
            FIDO2/WebAuthn credentials for passwordless authentication.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {passkeys.length === 0 ? (
            <p className="text-sm text-muted-foreground">No passkeys registered.</p>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead className="hidden sm:table-cell">Type</TableHead>
                    <TableHead className="hidden md:table-cell">Algorithm</TableHead>
                    <TableHead>Last used</TableHead>
                    <TableHead className="w-[60px]" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {passkeys.map((pk) => (
                    <TableRow key={pk.id}>
                      <TableCell className="font-medium">
                        {pk.displayName || pk.id.slice(0, 8)}
                      </TableCell>
                      <TableCell className="hidden sm:table-cell">
                        <Badge variant="outline">{pk.authenticatorType}</Badge>
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        {pk.algorithm}
                        {pk.algorithmWarning && (
                          <span className="ml-1 text-xs text-yellow-600">
                            ({pk.algorithmWarning})
                          </span>
                        )}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {pk.lastUsedAt
                          ? formatDistanceToNow(new Date(pk.lastUsedAt), { addSuffix: true })
                          : "Never"}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => setDeletePasskeyId(pk.id)}
                        >
                          <Trash2 className="h-4 w-4 text-destructive" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* ── Recovery Codes ────────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Shield className="h-5 w-5 text-muted-foreground" />
              <CardTitle>Recovery Codes</CardTitle>
            </div>
            <Button variant="outline" size="sm" onClick={generateRecoveryCodes}>
              <RefreshCw className="mr-1 h-4 w-4" /> Regenerate
            </Button>
          </div>
          <CardDescription>
            One-time backup codes for account recovery if you lose access to your passkeys.
            Regenerating replaces all existing codes.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {recoveryCodeCount === null ? (
            <p className="text-sm text-muted-foreground">Loading...</p>
          ) : recoveryCodeCount === 0 ? (
            <p className="text-sm text-muted-foreground">
              No recovery codes generated. Click Regenerate to create backup codes.
            </p>
          ) : (
            <p className="text-sm text-muted-foreground">
              {recoveryCodeCount} unused recovery code{recoveryCodeCount !== 1 ? "s" : ""} remaining.
            </p>
          )}
        </CardContent>
      </Card>

      {/* ── Active Sessions ──────────────────────────────────────── */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Monitor className="h-5 w-5 text-muted-foreground" />
              <CardTitle>Active Sessions</CardTitle>
            </div>
            <Button variant="outline" size="sm" onClick={fetchSessions}>
              <RefreshCw className="mr-1 h-4 w-4" /> Refresh
            </Button>
          </div>
          <CardDescription>Devices and clients currently signed in to your account.</CardDescription>
        </CardHeader>
        <CardContent>
          {sessions.length === 0 ? (
            <p className="text-sm text-muted-foreground">No active sessions.</p>
          ) : (
            <div className="rounded-md border">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Type</TableHead>
                    <TableHead className="hidden sm:table-cell">IP Address</TableHead>
                    <TableHead>Started</TableHead>
                    <TableHead>Expires</TableHead>
                    <TableHead className="w-[60px]" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sessions.map((sess) => (
                    <TableRow key={sess.id}>
                      <TableCell>
                        <Badge variant="outline">{sess.type}</Badge>
                      </TableCell>
                      <TableCell className="hidden sm:table-cell font-mono text-xs">
                        {sess.sourceIp || "—"}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatDistanceToNow(new Date(sess.createdAt), { addSuffix: true })}
                      </TableCell>
                      <TableCell className="text-muted-foreground">
                        {formatDistanceToNow(new Date(sess.expiresAt), { addSuffix: true })}
                      </TableCell>
                      <TableCell>
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => setRevokeSessionId(sess.id)}
                        >
                          <LogOut className="h-4 w-4 text-destructive" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {/* ── CI / API Tokens ──────────────────────────────────────── */}
      {isAdmin && (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-2">
                <KeyRound className="h-5 w-5 text-muted-foreground" />
                <CardTitle>API Tokens</CardTitle>
              </div>
              <Button size="sm" onClick={() => setShowCITokenDialog(true)}>
                <Plus className="mr-1 h-4 w-4" /> Create Token
              </Button>
            </div>
            <CardDescription>
              Machine-to-machine tokens for CI/CD pipelines and automation.
            </CardDescription>
          </CardHeader>
          <CardContent>
            {ciTokens.length === 0 ? (
              <p className="text-sm text-muted-foreground">No API tokens created.</p>
            ) : (
              <div className="rounded-md border">
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Name</TableHead>
                      <TableHead className="hidden sm:table-cell">Scopes</TableHead>
                      <TableHead>Created</TableHead>
                      <TableHead className="hidden md:table-cell">Last used</TableHead>
                      <TableHead className="w-[60px]" />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {ciTokens.map((tok) => (
                      <TableRow key={tok.id}>
                        <TableCell className="font-medium">{tok.name}</TableCell>
                        <TableCell className="hidden sm:table-cell">
                          {tok.scopes?.length > 0
                            ? tok.scopes.map((s) => (
                                <Badge key={s} variant="secondary" className="mr-1">
                                  {s}
                                </Badge>
                              ))
                            : <span className="text-muted-foreground">all</span>}
                        </TableCell>
                        <TableCell className="text-muted-foreground">
                          {formatDistanceToNow(new Date(tok.createdAt), { addSuffix: true })}
                        </TableCell>
                        <TableCell className="hidden md:table-cell text-muted-foreground">
                          {tok.lastUsedAt
                            ? formatDistanceToNow(new Date(tok.lastUsedAt), { addSuffix: true })
                            : "Never"}
                        </TableCell>
                        <TableCell>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => setRevokeCITokenId(tok.id)}
                          >
                            <Trash2 className="h-4 w-4 text-destructive" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* ── Server Info (admin only) ──────────────────────────────── */}
      {isAdmin && (
        <Card>
          <CardHeader>
            <div className="flex items-center gap-2">
              <Monitor className="h-5 w-5 text-muted-foreground" />
              <CardTitle>Server</CardTitle>
            </div>
            <CardDescription>Read-only server information.</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1">
                <Label className="text-sm text-muted-foreground">Account created</Label>
                <p className="text-sm">
                  {profile?.createdAt
                    ? formatDistanceToNow(new Date(profile.createdAt), { addSuffix: true })
                    : "—"}
                </p>
              </div>
              <div className="space-y-1">
                <Label className="text-sm text-muted-foreground">Last login</Label>
                <p className="text-sm">
                  {profile?.lastLoginAt
                    ? formatDistanceToNow(new Date(profile.lastLoginAt), { addSuffix: true })
                    : "—"}
                </p>
              </div>
              <div className="space-y-1">
                <Label className="text-sm text-muted-foreground">Passkeys registered</Label>
                <p className="text-sm">{profile?.passkeyCount ?? 0}</p>
              </div>
              <div className="space-y-1">
                <Label className="text-sm text-muted-foreground">Active sessions</Label>
                <p className="text-sm">{sessions.length}</p>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* ── Dialogs ──────────────────────────────────────────────── */}

      {/* Revoke session confirmation */}
      <AlertDialog
        open={!!revokeSessionId}
        onOpenChange={(open) => !open && setRevokeSessionId(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke session?</AlertDialogTitle>
            <AlertDialogDescription>
              This will immediately sign out the device associated with this session.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => revokeSessionId && revokeSession(revokeSessionId)}
            >
              Revoke
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Delete passkey confirmation */}
      <AlertDialog
        open={!!deletePasskeyId}
        onOpenChange={(open) => !open && setDeletePasskeyId(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Remove passkey?</AlertDialogTitle>
            <AlertDialogDescription>
              You will no longer be able to sign in with this passkey. Make sure you have another
              passkey or recovery codes before removing.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => deletePasskeyId && deletePasskey(deletePasskeyId)}
            >
              Remove
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Revoke CI token confirmation */}
      <AlertDialog
        open={!!revokeCITokenId}
        onOpenChange={(open) => !open && setRevokeCITokenId(null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke API token?</AlertDialogTitle>
            <AlertDialogDescription>
              Any automation using this token will immediately lose access.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              className="bg-destructive text-white hover:bg-destructive/90"
              onClick={() => revokeCITokenId && revokeCIToken(revokeCITokenId)}
            >
              Revoke
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Create CI token dialog */}
      <Dialog
        open={showCITokenDialog}
        onOpenChange={(open) => {
          if (!open) {
            setShowCITokenDialog(false)
            setCreatedCIToken(null)
            ciTokenForm.reset()
          }
        }}
      >
        <DialogContent>
          {createdCIToken ? (
            <>
              <DialogHeader>
                <DialogTitle>Token created</DialogTitle>
                <DialogDescription>
                  Copy this token now. It will not be shown again.
                </DialogDescription>
              </DialogHeader>
              <div className="flex items-center gap-2 rounded-md border bg-muted p-3">
                <code className="flex-1 break-all text-sm">{createdCIToken}</code>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => copyToClipboard(createdCIToken)}
                >
                  {copiedToken ? (
                    <Check className="h-4 w-4 text-green-500" />
                  ) : (
                    <Copy className="h-4 w-4" />
                  )}
                </Button>
              </div>
              <DialogFooter>
                <Button
                  onClick={() => {
                    setShowCITokenDialog(false)
                    setCreatedCIToken(null)
                  }}
                >
                  Done
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>Create API Token</DialogTitle>
                <DialogDescription>
                  Generate a token for CI/CD or automation. The token will be shown once.
                </DialogDescription>
              </DialogHeader>
              <Form {...ciTokenForm}>
                <form onSubmit={ciTokenForm.handleSubmit(createCIToken)} className="space-y-4">
                  <FormField
                    control={ciTokenForm.control}
                    name="name"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Name</FormLabel>
                        <FormControl>
                          <Input placeholder="e.g. GitHub Actions" {...field} />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <FormField
                    control={ciTokenForm.control}
                    name="scopes"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Scopes</FormLabel>
                        <FormControl>
                          <Input placeholder="Leave empty for all access" {...field} />
                        </FormControl>
                        <FormDescription>Comma-separated list of scopes.</FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                  <DialogFooter>
                    <Button type="submit">Create</Button>
                  </DialogFooter>
                </form>
              </Form>
            </>
          )}
        </DialogContent>
      </Dialog>

      {/* Recovery codes dialog */}
      <Dialog open={showRecoveryCodes} onOpenChange={(open) => !open && setShowRecoveryCodes(false)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Recovery Codes</DialogTitle>
            <DialogDescription>
              Save these codes in a secure location. Each code can only be used once.
            </DialogDescription>
          </DialogHeader>
          <div className="grid grid-cols-2 gap-2 rounded-md border bg-muted p-4">
            {recoveryCodes.map((code, i) => (
              <code key={i} className="text-sm font-mono">
                {code}
              </code>
            ))}
          </div>
          <div className="flex justify-end gap-2">
            <Button
              variant="outline"
              onClick={() => copyToClipboard(recoveryCodes.join("\n"))}
            >
              {copiedToken ? (
                <>
                  <Check className="mr-1 h-4 w-4" /> Copied
                </>
              ) : (
                <>
                  <Copy className="mr-1 h-4 w-4" /> Copy all
                </>
              )}
            </Button>
            <Button onClick={() => setShowRecoveryCodes(false)}>Done</Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
