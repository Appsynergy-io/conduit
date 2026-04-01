"use client"

import { Check, Copy, KeyRound, Plus, Trash2 } from "lucide-react"
import { useCallback, useEffect, useState } from "react"
import { useForm } from "react-hook-form"
import { standardSchemaResolver } from "@hookform/resolvers/standard-schema"
import { z } from "zod"
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
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
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

interface JoinToken {
  id: string
  type: string
  name: string
  labels?: Record<string, string>
  usedCount: number
  maxUses?: number
  expiresAt?: string
  revoked: boolean
  revokedAt?: string
  createdBy?: string
  createdAt: string
}

interface Platform {
  os: string
  arch: string
}

type SelectedOS = "linux" | "darwin" | "windows"

// ---------------------------------------------------------------------------
// Zod schema for the create token form
// ---------------------------------------------------------------------------

const createTokenSchema = z.object({
  name: z
    .string()
    .min(1, "Name is required")
    .max(100, "Name must be 100 characters or fewer")
    .regex(
      /^[a-zA-Z0-9][a-zA-Z0-9 _\-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$/,
      "Name must start and end with alphanumeric characters"
    ),
  type: z.enum(["single_use", "persistent"]),
  ttlHours: z.number().int().min(1, "Minimum 1 hour").max(8760, "Maximum 8760 hours (1 year)"),
  labels: z.string().optional(),
})

type CreateTokenValues = z.infer<typeof createTokenSchema>

// ---------------------------------------------------------------------------
// Copy button with independent state
// ---------------------------------------------------------------------------

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <Button variant="ghost" size="icon" className="absolute right-2 top-2 h-7 w-7" onClick={handleCopy}>
      {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
    </Button>
  )
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function TokensPage() {
  const { isAuthenticated } = useAuth()
  const [tokens, setTokens] = useState<JoinToken[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [showInstall, setShowInstall] = useState(false)
  const [createdToken, setCreatedToken] = useState<string | null>(null)
  const [platforms, setPlatforms] = useState<Platform[]>([])
  const [installScriptURL, setInstallScriptURL] = useState("")
  const [isDevMode, setIsDevMode] = useState(false)
  const [selectedOS, setSelectedOS] = useState<SelectedOS>("linux")
  const [revokeTarget, setRevokeTarget] = useState<JoinToken | null>(null)
  const [createError, setCreateError] = useState("")

  const form = useForm<CreateTokenValues>({
    resolver: standardSchemaResolver(createTokenSchema),
    defaultValues: {
      name: "",
      type: "single_use",
      ttlHours: 24,
      labels: "",
    },
  })

  // ---------------------------------------------------------------------------
  // Data fetching
  // ---------------------------------------------------------------------------

  const fetchTokens = useCallback(async () => {
    if (!isAuthenticated) return
    try {
      const res = await fetch("/api/v1/agents/tokens")
      if (res.ok) {
        const data = await res.json()
        setTokens(data.data ?? [])
      }
    } finally {
      setLoading(false)
    }
  }, [isAuthenticated])

  const fetchPlatforms = useCallback(async () => {
    if (!isAuthenticated) return
    try {
      const res = await fetch("/api/v1/download/agent/platforms")
      if (res.ok) {
        const data = await res.json()
        setPlatforms(data.platforms ?? [])
        setInstallScriptURL(data.installScript ?? "")
        setIsDevMode(data.devMode === true)
      }
    } catch {
      // Non-critical — install commands still work with manual download
    }
  }, [isAuthenticated])

  useEffect(() => {
    fetchTokens()
    fetchPlatforms()
  }, [fetchTokens, fetchPlatforms])

  // ---------------------------------------------------------------------------
  // Actions
  // ---------------------------------------------------------------------------

  const handleCreate = async (values: CreateTokenValues) => {
    setCreateError("")

    const labels: Record<string, string> = {}
    if (values.labels?.trim()) {
      for (const pair of values.labels.split(",")) {
        const [k, v] = pair.split("=").map((s) => s.trim())
        if (k && v) labels[k] = v
      }
    }

    const body: Record<string, unknown> = {
      name: values.name,
      type: values.type,
      ttlHours: values.ttlHours,
    }
    if (Object.keys(labels).length > 0) body.labels = labels

    const res = await fetch("/api/v1/agents/tokens", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })

    if (res.ok) {
      const data = await res.json()
      setCreatedToken(data.token)
      setShowCreate(false)
      setShowInstall(true)
      form.reset()
      fetchTokens()
    } else {
      const err = await res.json().catch(() => ({ detail: "Failed to create token." }))
      setCreateError(err.detail ?? "Failed to create token.")
    }
  }

  const handleRevoke = async () => {
    if (!revokeTarget) return
    const res = await fetch(`/api/v1/agents/tokens/${revokeTarget.id}`, { method: "DELETE" })
    if (res.ok || res.status === 204) {
      fetchTokens()
    }
    setRevokeTarget(null)
  }

  // ---------------------------------------------------------------------------
  // Install command helpers
  // ---------------------------------------------------------------------------

  const getInstallCommand = (os: SelectedOS): string => {
    if (!createdToken) return ""

    const curlFlag = isDevMode ? "-sSLk" : "-sSL"
    const devFlag = isDevMode ? " --dev-insecure" : ""

    if (installScriptURL) {
      return `curl ${curlFlag} ${installScriptURL} | sh -s --${devFlag} ${createdToken}`
    }

    const baseURL = window.location.origin
    const binary = os === "windows" ? "conduit.exe" : "conduit"
    return [
      `curl ${curlFlag} -o ${binary} "${baseURL}/api/v1/download/agent?os=${os}&arch=amd64"`,
      os !== "windows" ? `chmod +x ${binary}` : "",
      `${os === "windows" ? ".\\" : "./"}${binary} join ${baseURL} ${createdToken}${devFlag}`,
    ]
      .filter(Boolean)
      .join(" && ")
  }

  const getManualJoinCommand = (): string => {
    if (!createdToken) return ""
    const baseURL = installScriptURL ? installScriptURL.replace("/install.sh", "") : window.location.origin
    const devFlag = isDevMode ? " --dev-insecure" : ""
    return `conduit join ${baseURL} ${createdToken}${devFlag}`
  }

  const tokenStatus = (t: JoinToken): { label: string; variant: "default" | "secondary" | "destructive" } => {
    if (t.revoked) return { label: "Revoked", variant: "destructive" }
    if (t.expiresAt && new Date(t.expiresAt) < new Date()) return { label: "Expired", variant: "secondary" }
    return { label: "Active", variant: "default" }
  }

  const hasPlatform = (os: string): boolean => {
    return platforms.some((p) => p.os === os)
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Join Tokens</h1>
          <p className="text-sm text-muted-foreground">
            Generate tokens to enroll new machines as agents.
          </p>
        </div>
        <Button onClick={() => setShowCreate(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Create Token
        </Button>
      </div>

      {/* Token list */}
      {loading ? (
        <div className="space-y-3">
          {["a", "b", "c"].map((k) => (
            <Skeleton key={k} className="h-10 w-full" />
          ))}
        </div>
      ) : tokens.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-16 text-center">
          <KeyRound className="h-12 w-12 text-muted-foreground/50" />
          <h3 className="mt-4 text-lg font-medium">No join tokens</h3>
          <p className="mt-1 max-w-sm text-sm text-muted-foreground">
            Create a join token to get an install command for enrolling machines.
          </p>
          <Button className="mt-4" onClick={() => setShowCreate(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Create Token
          </Button>
        </div>
      ) : (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Labels</TableHead>
                <TableHead>Used</TableHead>
                <TableHead>Expires</TableHead>
                <TableHead>Status</TableHead>
                <TableHead className="w-12" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {tokens.map((t) => {
                const status = tokenStatus(t)
                return (
                  <TableRow key={t.id}>
                    <TableCell className="font-medium">{t.name}</TableCell>
                    <TableCell>
                      <Badge variant="outline">{t.type === "single_use" ? "Single use" : "Persistent"}</Badge>
                    </TableCell>
                    <TableCell>
                      {t.labels ? (
                        <div className="flex flex-wrap gap-1">
                          {Object.entries(t.labels).map(([k, v]) => (
                            <Badge key={k} variant="outline" className="text-xs">
                              {k}={v}
                            </Badge>
                          ))}
                        </div>
                      ) : (
                        <span className="text-muted-foreground">-</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {t.usedCount}
                      {t.maxUses != null ? ` / ${t.maxUses}` : ""}
                    </TableCell>
                    <TableCell>
                      {t.expiresAt ? (
                        <span className="text-xs">{new Date(t.expiresAt).toLocaleString()}</span>
                      ) : (
                        <span className="text-muted-foreground">Never</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <Badge variant={status.variant}>{status.label}</Badge>
                    </TableCell>
                    <TableCell>
                      {!t.revoked && (
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 text-muted-foreground hover:text-destructive"
                          onClick={() => setRevokeTarget(t)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}

      {/* Revoke confirmation dialog (OWASP — blocking confirmation for destructive actions) */}
      <AlertDialog open={!!revokeTarget} onOpenChange={(open) => { if (!open) setRevokeTarget(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Revoke join token?</AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently revoke <strong>{revokeTarget?.name}</strong>. Machines that
              haven&apos;t joined yet will no longer be able to use this token. Already-joined
              agents are unaffected.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={handleRevoke}>
              Revoke Token
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Create token dialog with react-hook-form + zod */}
      <Dialog open={showCreate} onOpenChange={(open) => {
        setShowCreate(open)
        if (!open) {
          form.reset()
          setCreateError("")
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Join Token</DialogTitle>
            <DialogDescription>
              Generate a token to enroll machines as Conduit agents.
            </DialogDescription>
          </DialogHeader>
          <Form {...form}>
            <form onSubmit={form.handleSubmit(handleCreate)} className="space-y-4 py-2">
              <FormField
                control={form.control}
                name="name"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Name</FormLabel>
                    <FormControl>
                      <Input placeholder="e.g. Production servers" maxLength={100} {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="type"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Type</FormLabel>
                    <Select onValueChange={field.onChange} defaultValue={field.value}>
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        <SelectItem value="single_use">Single use (one machine)</SelectItem>
                        <SelectItem value="persistent">Persistent (multiple machines)</SelectItem>
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="ttlHours"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Expires in (hours)</FormLabel>
                    <FormControl>
                      <Input
                        type="number"
                        min={1}
                        max={8760}
                        {...field}
                        onChange={(e) => field.onChange(e.target.valueAsNumber || 0)}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name="labels"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Labels (optional)</FormLabel>
                    <FormControl>
                      <Input placeholder="env=production, role=web, dc=us-east-1" {...field} />
                    </FormControl>
                    <FormDescription>
                      Comma-separated key=value pairs. Applied to agents on join.
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              {createError && <p className="text-sm text-destructive">{createError}</p>}
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setShowCreate(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={form.formState.isSubmitting}>
                  {form.formState.isSubmitting ? "Creating..." : "Create Token"}
                </Button>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>

      {/* Install command dialog */}
      <Dialog
        open={showInstall}
        onOpenChange={(open) => {
          setShowInstall(open)
          if (!open) setCreatedToken(null)
        }}
      >
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Install Agent</DialogTitle>
            <DialogDescription>
              Copy the command below to install and register the agent on your machine.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4 py-2">
            {/* OS Selector */}
            <div className="space-y-2">
              <Label>Operating System</Label>
              <div className="flex gap-2">
                {(["linux", "darwin", "windows"] as const).map((os) => (
                  <Button
                    key={os}
                    variant={selectedOS === os ? "default" : "outline"}
                    size="sm"
                    onClick={() => setSelectedOS(os)}
                    className="min-w-[100px]"
                  >
                    {os === "darwin" ? "macOS" : os === "linux" ? "Linux" : "Windows"}
                    {hasPlatform(os) && (
                      <Check className="ml-1.5 h-3 w-3" />
                    )}
                  </Button>
                ))}
              </div>
              {!hasPlatform(selectedOS) && platforms.length > 0 && (
                <p className="text-xs text-muted-foreground">
                  Binary not available for this platform. Upload it to the server&apos;s binaries directory.
                </p>
              )}
            </div>

            {/* One-liner install */}
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm">One-line install</CardTitle>
                <CardDescription className="text-xs">
                  Downloads the agent binary and joins this server automatically.
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="relative">
                  <pre className="overflow-x-auto rounded-md bg-muted p-3 pr-12 font-mono text-xs leading-relaxed">
                    {getInstallCommand(selectedOS)}
                  </pre>
                  <CopyButton text={getInstallCommand(selectedOS)} />
                </div>
              </CardContent>
            </Card>

            {/* Manual join (if binary already installed) */}
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-sm">Already have the binary?</CardTitle>
                <CardDescription className="text-xs">
                  If the conduit binary is already installed, just run this:
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="relative">
                  <pre className="overflow-x-auto rounded-md bg-muted p-3 pr-12 font-mono text-xs leading-relaxed">
                    {getManualJoinCommand()}
                  </pre>
                  <CopyButton text={getManualJoinCommand()} />
                </div>
              </CardContent>
            </Card>

            {/* Raw token (for reference) */}
            <div className="space-y-1">
              <Label className="text-xs text-muted-foreground">Raw token (shown once)</Label>
              <div className="relative">
                <pre className="overflow-x-auto rounded-md bg-muted p-3 pr-12 font-mono text-xs">
                  {createdToken}
                </pre>
                <CopyButton text={createdToken ?? ""} />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button
              onClick={() => {
                setShowInstall(false)
                setCreatedToken(null)
              }}
            >
              Done
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
