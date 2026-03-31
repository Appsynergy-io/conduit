"use client"

import {
  AlertCircle,
  ArrowLeft,
  ChevronRight,
  Download,
  Eye,
  File,
  FileText,
  Folder,
  FolderPlus,
  Link2,
  Loader2,
  MoreHorizontal,
  Pencil,
  RefreshCw,
  Trash2,
  Upload,
} from "lucide-react"
import { useCallback, useEffect, useRef, useState } from "react"
import { toast } from "sonner"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { useAuth } from "@/hooks/use-auth"

// ── Types ────────────────────────────────────────────

interface FileEntry {
  name: string
  type: "file" | "directory" | "symlink"
  size: number
  permissions?: string
  owner?: string
  group?: string
  modifiedAt: string
  isHidden?: boolean
}

interface FilePreview {
  path: string
  content: string
  truncated: boolean
  totalSize: number
  mimeType: string
  encoding: "utf-8" | "base64"
}

interface FileBrowserProps {
  agentId: string
}

// ── Helpers ──────────────────────────────────────────

function formatFileSize(bytes: number): string {
  if (bytes === 0) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / 1024 ** i
  return `${value.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

function formatTime(ts: string): string {
  try {
    return new Date(ts).toLocaleString()
  } catch {
    return ts
  }
}

function fileIcon(entry: FileEntry) {
  if (entry.type === "directory") return <Folder className="h-4 w-4 text-blue-500" />
  if (entry.type === "symlink") return <Link2 className="h-4 w-4 text-purple-500" />
  if (entry.name.match(/\.(txt|md|log|json|yaml|yml|toml|xml|csv|ini|cfg|conf)$/i)) {
    return <FileText className="h-4 w-4 text-muted-foreground" />
  }
  return <File className="h-4 w-4 text-muted-foreground" />
}

/** Join path segments, avoiding double slashes */
function joinPath(base: string, name: string): string {
  if (base === "/") return `/${name}`
  return `${base}/${name}`
}

/** Split an absolute path into breadcrumb segments */
function pathSegments(path: string): { name: string; path: string }[] {
  const parts = path.split("/").filter(Boolean)
  const segments: { name: string; path: string }[] = [{ name: "/", path: "/" }]
  for (let i = 0; i < parts.length; i++) {
    segments.push({ name: parts[i], path: `/${parts.slice(0, i + 1).join("/")}` })
  }
  return segments
}

/** Get parent directory path */
function parentPath(path: string): string {
  if (path === "/") return "/"
  const parts = path.split("/").filter(Boolean)
  parts.pop()
  return parts.length === 0 ? "/" : `/${parts.join("/")}`
}

// ── Component ────────────────────────────────────────

export function FileBrowser({ agentId }: FileBrowserProps) {
  const { token } = useAuth()
  const [currentPath, setCurrentPath] = useState("/")
  const [entries, setEntries] = useState<FileEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [showHidden, setShowHidden] = useState(false)

  // Dialog states
  const [mkdirOpen, setMkdirOpen] = useState(false)
  const [mkdirName, setMkdirName] = useState("")
  const [renameOpen, setRenameOpen] = useState(false)
  const [renameTarget, setRenameTarget] = useState<FileEntry | null>(null)
  const [renameName, setRenameName] = useState("")
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<FileEntry | null>(null)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [preview, setPreview] = useState<FilePreview | null>(null)
  const [previewLoading, setPreviewLoading] = useState(false)
  const [actionLoading, setActionLoading] = useState(false)

  const uploadRef = useRef<HTMLInputElement>(null)

  // ── Fetch directory listing ──

  const fetchEntries = useCallback(
    async (path: string) => {
      if (!token) return
      setLoading(true)
      setError(null)

      try {
        const params = new URLSearchParams({
          path,
          showHidden: String(showHidden),
        })
        const res = await fetch(
          `/api/v1/agents/${encodeURIComponent(agentId)}/files?${params.toString()}`,
          { headers: { Authorization: `Bearer ${token}` } },
        )

        if (!res.ok) {
          if (res.status === 404) {
            setError("Path not found")
          } else if (res.status === 401) {
            setError("Unauthorized")
          } else if (res.status === 403) {
            setError("Access denied")
          } else {
            setError("Failed to list directory")
          }
          return
        }

        const data = await res.json()
        setEntries(data.entries ?? [])
        setCurrentPath(data.path ?? path)
      } catch {
        setError("Failed to connect to server")
      } finally {
        setLoading(false)
      }
    },
    [token, agentId, showHidden],
  )

  useEffect(() => {
    fetchEntries(currentPath)
  }, [fetchEntries, currentPath])

  // ── Navigation ──

  const navigateTo = useCallback((path: string) => {
    setCurrentPath(path)
  }, [])

  const handleEntryClick = useCallback(
    (entry: FileEntry) => {
      if (entry.type === "directory") {
        navigateTo(joinPath(currentPath, entry.name))
      }
    },
    [currentPath, navigateTo],
  )

  // ── Download ──

  const handleDownload = useCallback(
    async (entry: FileEntry) => {
      if (!token) return
      const filePath = joinPath(currentPath, entry.name)
      const params = new URLSearchParams({ path: filePath })
      const res = await fetch(
        `/api/v1/agents/${encodeURIComponent(agentId)}/files/download?${params.toString()}`,
        { headers: { Authorization: `Bearer ${token}` } },
      )

      if (!res.ok) {
        toast.error("Download failed")
        return
      }

      const blob = await res.blob()
      const url = URL.createObjectURL(blob)
      const a = document.createElement("a")
      a.href = url
      a.download = entry.name
      a.click()
      URL.revokeObjectURL(url)
    },
    [token, agentId, currentPath],
  )

  // ── Upload ──

  const handleUpload = useCallback(
    async (file: globalThis.File) => {
      if (!token) return
      setActionLoading(true)

      try {
        const targetPath = joinPath(currentPath, file.name)
        const params = new URLSearchParams({ path: targetPath })
        const res = await fetch(
          `/api/v1/agents/${encodeURIComponent(agentId)}/files/upload?${params.toString()}`,
          {
            method: "POST",
            headers: {
              Authorization: `Bearer ${token}`,
              "Content-Type": "application/octet-stream",
            },
            body: file,
          },
        )

        if (!res.ok) {
          toast.error(res.status === 413 ? "File exceeds maximum upload size" : "Upload failed")
          return
        }

        toast.success(`Uploaded ${file.name}`)
        await fetchEntries(currentPath)
      } catch {
        toast.error("Upload failed")
      } finally {
        setActionLoading(false)
      }
    },
    [token, agentId, currentPath, fetchEntries],
  )

  const onFileSelected = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0]
      if (file) handleUpload(file)
      // Reset input so same file can be re-selected
      e.target.value = ""
    },
    [handleUpload],
  )

  // ── Delete ──

  const handleDelete = useCallback(async () => {
    if (!token || !deleteTarget) return
    setActionLoading(true)

    try {
      const filePath = joinPath(currentPath, deleteTarget.name)
      const res = await fetch(`/api/v1/agents/${encodeURIComponent(agentId)}/files/delete`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          path: filePath,
          recursive: deleteTarget.type === "directory",
        }),
      })

      if (!res.ok) {
        toast.error("Delete failed")
        return
      }

      toast.success(`Deleted ${deleteTarget.name}`)
      setDeleteOpen(false)
      setDeleteTarget(null)
      await fetchEntries(currentPath)
    } catch {
      toast.error("Delete failed")
    } finally {
      setActionLoading(false)
    }
  }, [token, agentId, currentPath, deleteTarget, fetchEntries])

  // ── Rename ──

  const handleRename = useCallback(async () => {
    if (!token || !renameTarget || !renameName.trim()) return
    setActionLoading(true)

    try {
      const oldPath = joinPath(currentPath, renameTarget.name)
      const newPath = joinPath(currentPath, renameName.trim())
      const res = await fetch(`/api/v1/agents/${encodeURIComponent(agentId)}/files/rename`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ oldPath, newPath }),
      })

      if (!res.ok) {
        toast.error(res.status === 409 ? "A file with that name already exists" : "Rename failed")
        return
      }

      toast.success(`Renamed to ${renameName.trim()}`)
      setRenameOpen(false)
      setRenameTarget(null)
      setRenameName("")
      await fetchEntries(currentPath)
    } catch {
      toast.error("Rename failed")
    } finally {
      setActionLoading(false)
    }
  }, [token, agentId, currentPath, renameTarget, renameName, fetchEntries])

  // ── Mkdir ──

  const handleMkdir = useCallback(async () => {
    if (!token || !mkdirName.trim()) return
    setActionLoading(true)

    try {
      const dirPath = joinPath(currentPath, mkdirName.trim())
      const res = await fetch(`/api/v1/agents/${encodeURIComponent(agentId)}/files/mkdir`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ path: dirPath, parents: true }),
      })

      if (!res.ok) {
        toast.error(res.status === 409 ? "Directory already exists" : "Failed to create directory")
        return
      }

      toast.success(`Created ${mkdirName.trim()}`)
      setMkdirOpen(false)
      setMkdirName("")
      await fetchEntries(currentPath)
    } catch {
      toast.error("Failed to create directory")
    } finally {
      setActionLoading(false)
    }
  }, [token, agentId, currentPath, mkdirName, fetchEntries])

  // ── Preview ──

  const handlePreview = useCallback(
    async (entry: FileEntry) => {
      if (!token) return
      setPreviewLoading(true)
      setPreviewOpen(true)
      setPreview(null)

      try {
        const filePath = joinPath(currentPath, entry.name)
        const params = new URLSearchParams({ path: filePath })
        const res = await fetch(
          `/api/v1/agents/${encodeURIComponent(agentId)}/files/preview?${params.toString()}`,
          { headers: { Authorization: `Bearer ${token}` } },
        )

        if (!res.ok) {
          setPreviewOpen(false)
          toast.error("Preview failed")
          return
        }

        const data: FilePreview = await res.json()
        setPreview(data)
      } catch {
        setPreviewOpen(false)
        toast.error("Preview failed")
      } finally {
        setPreviewLoading(false)
      }
    },
    [token, agentId, currentPath],
  )

  // ── Sort entries: directories first, then alphabetical ──

  const sortedEntries = [...entries].sort((a, b) => {
    if (a.type === "directory" && b.type !== "directory") return -1
    if (a.type !== "directory" && b.type === "directory") return 1
    return a.name.localeCompare(b.name)
  })

  // ── Render ──

  const segments = pathSegments(currentPath)

  return (
    <div className="flex h-full flex-col" data-testid="file-browser">
      {/* Toolbar */}
      <div className="flex items-center justify-between border-b px-4 py-2">
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => navigateTo(parentPath(currentPath))}
            disabled={currentPath === "/"}
            aria-label="Go to parent directory"
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>

          <Breadcrumb>
            <BreadcrumbList>
              {segments.map((seg, i) => (
                <BreadcrumbItem key={seg.path}>
                  {i > 0 && (
                    <BreadcrumbSeparator>
                      <ChevronRight className="h-3 w-3" />
                    </BreadcrumbSeparator>
                  )}
                  <BreadcrumbLink
                    href="#"
                    onClick={(e) => {
                      e.preventDefault()
                      navigateTo(seg.path)
                    }}
                    className="text-sm"
                  >
                    {seg.name}
                  </BreadcrumbLink>
                </BreadcrumbItem>
              ))}
            </BreadcrumbList>
          </Breadcrumb>
        </div>

        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setShowHidden(!showHidden)}
            className="text-xs"
          >
            {showHidden ? "Hide hidden" : "Show hidden"}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => fetchEntries(currentPath)}
            aria-label="Refresh"
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => {
              setMkdirName("")
              setMkdirOpen(true)
            }}
            aria-label="Create directory"
          >
            <FolderPlus className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => uploadRef.current?.click()}
            disabled={actionLoading}
            aria-label="Upload file"
          >
            <Upload className="h-4 w-4" />
          </Button>
          <input
            ref={uploadRef}
            type="file"
            className="hidden"
            onChange={onFileSelected}
            data-testid="upload-input"
          />
        </div>
      </div>

      {/* Loading state */}
      {loading && (
        <div className="flex flex-1 items-center justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
        </div>
      )}

      {/* Error state */}
      {!loading && error && (
        <div className="flex flex-1 items-center justify-center p-4">
          <Alert variant="destructive" className="max-w-md">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>{error}</AlertTitle>
            <AlertDescription>
              Check the path and try again, or return to the parent directory.
            </AlertDescription>
          </Alert>
        </div>
      )}

      {/* Empty state */}
      {!loading && !error && sortedEntries.length === 0 && (
        <div className="flex flex-1 flex-col items-center justify-center text-center">
          <Folder className="h-12 w-12 text-muted-foreground/50" />
          <p className="mt-2 text-sm text-muted-foreground">This directory is empty</p>
        </div>
      )}

      {/* File table */}
      {!loading && !error && sortedEntries.length > 0 && (
        <ScrollArea className="flex-1">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-[50%]">Name</TableHead>
                <TableHead className="hidden md:table-cell">Size</TableHead>
                <TableHead className="hidden lg:table-cell">Permissions</TableHead>
                <TableHead className="hidden lg:table-cell">Owner</TableHead>
                <TableHead className="hidden md:table-cell">Modified</TableHead>
                <TableHead className="w-10" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {sortedEntries.map((entry) => (
                <TableRow
                  key={entry.name}
                  className={entry.type === "directory" ? "cursor-pointer" : undefined}
                  onClick={() => handleEntryClick(entry)}
                  data-testid={`entry-${entry.name}`}
                >
                  <TableCell>
                    <div className="flex items-center gap-2">
                      {fileIcon(entry)}
                      <span className="truncate text-sm">{entry.name}</span>
                    </div>
                  </TableCell>
                  <TableCell className="hidden md:table-cell text-sm text-muted-foreground">
                    {entry.type === "directory" ? "-" : formatFileSize(entry.size)}
                  </TableCell>
                  <TableCell className="hidden lg:table-cell font-mono text-xs text-muted-foreground">
                    {entry.permissions ?? "-"}
                  </TableCell>
                  <TableCell className="hidden lg:table-cell text-sm text-muted-foreground">
                    {entry.owner ?? "-"}
                  </TableCell>
                  <TableCell className="hidden md:table-cell text-sm text-muted-foreground">
                    {formatTime(entry.modifiedAt)}
                  </TableCell>
                  <TableCell>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8"
                          onClick={(e) => e.stopPropagation()}
                          aria-label={`Actions for ${entry.name}`}
                        >
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        {entry.type === "file" && (
                          <DropdownMenuItem
                            onClick={(e) => {
                              e.stopPropagation()
                              handlePreview(entry)
                            }}
                          >
                            <Eye className="mr-2 h-4 w-4" />
                            Preview
                          </DropdownMenuItem>
                        )}
                        {entry.type === "file" && (
                          <DropdownMenuItem
                            onClick={(e) => {
                              e.stopPropagation()
                              handleDownload(entry)
                            }}
                          >
                            <Download className="mr-2 h-4 w-4" />
                            Download
                          </DropdownMenuItem>
                        )}
                        <DropdownMenuItem
                          onClick={(e) => {
                            e.stopPropagation()
                            setRenameTarget(entry)
                            setRenameName(entry.name)
                            setRenameOpen(true)
                          }}
                        >
                          <Pencil className="mr-2 h-4 w-4" />
                          Rename
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem
                          onClick={(e) => {
                            e.stopPropagation()
                            setDeleteTarget(entry)
                            setDeleteOpen(true)
                          }}
                          className="text-destructive"
                        >
                          <Trash2 className="mr-2 h-4 w-4" />
                          Delete
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </ScrollArea>
      )}

      {/* ── Dialogs ── */}

      {/* Create Directory */}
      <Dialog open={mkdirOpen} onOpenChange={setMkdirOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Directory</DialogTitle>
            <DialogDescription>Create a new directory in {currentPath}</DialogDescription>
          </DialogHeader>
          <Input
            placeholder="Directory name"
            value={mkdirName}
            onChange={(e) => setMkdirName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleMkdir()
            }}
            data-testid="mkdir-input"
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setMkdirOpen(false)}>
              Cancel
            </Button>
            <Button onClick={handleMkdir} disabled={!mkdirName.trim() || actionLoading}>
              {actionLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Create
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Rename */}
      <Dialog open={renameOpen} onOpenChange={setRenameOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Rename</DialogTitle>
            <DialogDescription>Rename {renameTarget?.name}</DialogDescription>
          </DialogHeader>
          <Input
            placeholder="New name"
            value={renameName}
            onChange={(e) => setRenameName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleRename()
            }}
            data-testid="rename-input"
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setRenameOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={handleRename}
              disabled={!renameName.trim() || renameName === renameTarget?.name || actionLoading}
            >
              {actionLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Rename
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation */}
      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete {deleteTarget?.name}?</AlertDialogTitle>
            <AlertDialogDescription>
              {deleteTarget?.type === "directory"
                ? "This will permanently delete this directory and all its contents. This action cannot be undone."
                : "This will permanently delete this file. This action cannot be undone."}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={handleDelete}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {actionLoading ? <Loader2 className="mr-2 h-4 w-4 animate-spin" /> : null}
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* File Preview */}
      <Dialog open={previewOpen} onOpenChange={setPreviewOpen}>
        <DialogContent className="max-w-2xl max-h-[80vh]">
          <DialogHeader>
            <DialogTitle className="truncate">
              {preview?.path?.split("/").pop() ?? "Preview"}
            </DialogTitle>
            <DialogDescription>
              {preview
                ? `${preview.mimeType} — ${formatFileSize(preview.totalSize)}${preview.truncated ? " (truncated)" : ""}`
                : "Loading..."}
            </DialogDescription>
          </DialogHeader>
          {previewLoading && (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
            </div>
          )}
          {preview && !previewLoading && (
            <ScrollArea className="max-h-[60vh]">
              {preview.encoding === "utf-8" ? (
                <pre
                  className="whitespace-pre-wrap break-all rounded bg-muted p-4 font-mono text-xs"
                  data-testid="preview-content"
                >
                  {preview.content}
                </pre>
              ) : (
                <div
                  className="rounded bg-muted p-4 text-center text-sm text-muted-foreground"
                  data-testid="preview-content"
                >
                  Binary file ({preview.mimeType}). Download to view.
                </div>
              )}
            </ScrollArea>
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
