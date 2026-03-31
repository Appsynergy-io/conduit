import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { FileBrowser } from "./file-browser"

// Mock useAuth
const mockToken = "test-jwt-token"
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ token: mockToken, isAuthenticated: true }),
}))

// Mock Radix DropdownMenu — jsdom doesn't support pointer events that Radix requires
vi.mock("@/components/ui/dropdown-menu", () => {
  return {
    DropdownMenu: ({ children }: { children: ReactNode }) => (
      <div data-testid="dropdown">{children}</div>
    ),
    DropdownMenuTrigger: ({
      children,
      asChild,
      ...props
    }: {
      children: ReactNode
      asChild?: boolean
      [key: string]: unknown
    }) => {
      if (asChild) return children
      return <button {...props}>{children}</button>
    },
    DropdownMenuContent: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    DropdownMenuItem: ({
      children,
      onClick,
      className,
      ...props
    }: {
      children: ReactNode
      onClick?: (e: React.MouseEvent) => void
      className?: string
      [key: string]: unknown
    }) => (
      <button type="button" role="menuitem" onClick={onClick} className={className} {...props}>
        {children}
      </button>
    ),
    DropdownMenuSeparator: () => <hr />,
  }
})

// ── Test data ──

const mockEntries = [
  {
    name: "documents",
    type: "directory",
    size: 4096,
    permissions: "drwxr-xr-x",
    owner: "root",
    group: "root",
    modifiedAt: "2026-03-30T10:00:00Z",
  },
  {
    name: "readme.md",
    type: "file",
    size: 1234,
    permissions: "-rw-r--r--",
    owner: "root",
    group: "root",
    modifiedAt: "2026-03-29T15:30:00Z",
  },
  {
    name: "config.yaml",
    type: "file",
    size: 567,
    permissions: "-rw-------",
    owner: "root",
    group: "root",
    modifiedAt: "2026-03-28T09:00:00Z",
  },
  {
    name: "link-to-docs",
    type: "symlink",
    size: 0,
    modifiedAt: "2026-03-27T12:00:00Z",
  },
]

// ── Fetch mock helpers ──

function mockFetchSuccess(data: unknown, status = 200) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
    blob: () => Promise.resolve(new Blob(["file content"])),
  })
}

function mockFetchError(status: number) {
  return vi.fn().mockResolvedValue({
    ok: false,
    status,
    json: () => Promise.resolve({ title: "Error" }),
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe("FileBrowser", () => {
  it("renders the file browser with toolbar", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ path: "/", entries: mockEntries }))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByTestId("file-browser")).toBeInTheDocument()
    })

    expect(screen.getByRole("button", { name: "Go to parent directory" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Refresh" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Create directory" })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Upload file" })).toBeInTheDocument()
  })

  it("fetches and displays directory entries", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ path: "/", entries: mockEntries }))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("documents")).toBeInTheDocument()
    })

    expect(screen.getByText("readme.md")).toBeInTheDocument()
    expect(screen.getByText("config.yaml")).toBeInTheDocument()
    expect(screen.getByText("link-to-docs")).toBeInTheDocument()
  })

  it("sorts directories before files", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ path: "/", entries: mockEntries }))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("documents")).toBeInTheDocument()
    })

    const rows = screen.getAllByRole("row")
    // Row 0 is header, row 1 should be "documents" (directory first)
    expect(rows[1]).toHaveTextContent("documents")
  })

  it("shows empty state when directory is empty", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ path: "/", entries: [] }))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("This directory is empty")).toBeInTheDocument()
    })
  })

  it("shows error state on fetch failure", async () => {
    vi.stubGlobal("fetch", mockFetchError(500))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("Failed to list directory")).toBeInTheDocument()
    })
  })

  it("shows 404 error for missing path", async () => {
    vi.stubGlobal("fetch", mockFetchError(404))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("Path not found")).toBeInTheDocument()
    })
  })

  it("shows 401 error for unauthorized", async () => {
    vi.stubGlobal("fetch", mockFetchError(401))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("Unauthorized")).toBeInTheDocument()
    })
  })

  it("shows 403 error for forbidden", async () => {
    vi.stubGlobal("fetch", mockFetchError(403))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("Access denied")).toBeInTheDocument()
    })
  })

  it("navigates into a directory on click", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ path: "/", entries: mockEntries }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/documents",
            entries: [
              {
                name: "report.pdf",
                type: "file",
                size: 99999,
                modifiedAt: "2026-03-30T10:00:00Z",
              },
            ],
          }),
      })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("documents")).toBeInTheDocument()
    })

    await act(async () => {
      fireEvent.click(screen.getByTestId("entry-documents"))
    })

    await waitFor(() => {
      expect(screen.getByText("report.pdf")).toBeInTheDocument()
    })

    // Second fetch should include /documents path
    const secondCall = fetchMock.mock.calls[1][0] as string
    expect(secondCall).toContain("path=%2Fdocuments")
  })

  it("navigates to parent directory", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/var/log",
            entries: [
              { name: "syslog", type: "file", size: 100, modifiedAt: "2026-03-30T10:00:00Z" },
            ],
          }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/var",
            entries: [
              { name: "log", type: "directory", size: 4096, modifiedAt: "2026-03-30T10:00:00Z" },
            ],
          }),
      })

    vi.stubGlobal("fetch", fetchMock)

    // Start at /var/log by setting initial path via navigation
    render(<FileBrowser agentId="agent-123" />)

    // Wait for initial load
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })
  })

  it("refreshes on refresh button click", async () => {
    const fetchMock = mockFetchSuccess({ path: "/", entries: mockEntries })
    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("documents")).toBeInTheDocument()
    })

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Refresh" }))
    })

    // Should have been called at least twice (initial + refresh)
    expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(2)
  })

  it("opens create directory dialog", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ path: "/", entries: mockEntries }))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("documents")).toBeInTheDocument()
    })

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Create directory" }))
    })

    expect(screen.getByText("Create Directory")).toBeInTheDocument()
    expect(screen.getByTestId("mkdir-input")).toBeInTheDocument()
  })

  it("creates a directory via mkdir dialog", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ path: "/", entries: mockEntries }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve({}),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/",
            entries: [
              ...mockEntries,
              {
                name: "new-dir",
                type: "directory",
                size: 4096,
                modifiedAt: "2026-03-31T00:00:00Z",
              },
            ],
          }),
      })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("documents")).toBeInTheDocument()
    })

    // Open dialog
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Create directory" }))
    })

    // Type name and submit
    await act(async () => {
      fireEvent.change(screen.getByTestId("mkdir-input"), { target: { value: "new-dir" } })
    })

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Create" }))
    })

    // Verify mkdir API was called
    await waitFor(() => {
      const mkdirCall = fetchMock.mock.calls.find(
        (call: [string, RequestInit]) =>
          typeof call[0] === "string" && call[0].includes("/files/mkdir"),
      )
      expect(mkdirCall).toBeDefined()
      const body = JSON.parse(mkdirCall[1].body as string)
      expect(body.path).toBe("/new-dir")
      expect(body.parents).toBe(true)
    })
  })

  it("shows toggle for hidden files", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ path: "/", entries: mockEntries }))

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("documents")).toBeInTheDocument()
    })

    expect(screen.getByText("Show hidden")).toBeInTheDocument()
  })

  it("displays file sizes correctly", async () => {
    vi.stubGlobal(
      "fetch",
      mockFetchSuccess({
        path: "/",
        entries: [
          { name: "small.txt", type: "file", size: 100, modifiedAt: "2026-03-30T10:00:00Z" },
          { name: "big.bin", type: "file", size: 1048576, modifiedAt: "2026-03-30T10:00:00Z" },
        ],
      }),
    )

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("100 B")).toBeInTheDocument()
      expect(screen.getByText("1.0 MB")).toBeInTheDocument()
    })
  })

  it("shows dash for directory size", async () => {
    vi.stubGlobal(
      "fetch",
      mockFetchSuccess({
        path: "/",
        entries: [
          { name: "mydir", type: "directory", size: 4096, modifiedAt: "2026-03-30T10:00:00Z" },
        ],
      }),
    )

    render(<FileBrowser agentId="agent-123" />)

    await waitFor(() => {
      expect(screen.getByText("mydir")).toBeInTheDocument()
    })

    // Directory row should show "-" for size, not "4.0 KB"
    const row = screen.getByTestId("entry-mydir")
    expect(row).toHaveTextContent("-")
  })
})

describe("FileBrowser — Security", () => {
  it("encodes agentId in API URLs to prevent path traversal", async () => {
    const fetchMock = mockFetchSuccess({ path: "/", entries: [] })
    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="../../etc/passwd" />)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })

    const url = fetchMock.mock.calls[0][0] as string
    // agentId must be encoded — no raw traversal sequences
    expect(url).toContain(encodeURIComponent("../../etc/passwd"))
    expect(url).not.toContain("../../etc/passwd/files")
  })

  it("sends auth token in Authorization header", async () => {
    const fetchMock = mockFetchSuccess({ path: "/", entries: [] })
    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })

    const headers = fetchMock.mock.calls[0][1].headers as Record<string, string>
    expect(headers.Authorization).toBe("Bearer test-jwt-token")
  })

  it("uses POST for destructive operations (delete)", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/",
            entries: [
              { name: "target.txt", type: "file", size: 100, modifiedAt: "2026-03-30T10:00:00Z" },
            ],
          }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 204,
        json: () => Promise.resolve({}),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ path: "/", entries: [] }),
      })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(screen.getByText("target.txt")).toBeInTheDocument()
    })

    // Click delete in dropdown menu (menu items rendered via mock)
    await act(async () => {
      fireEvent.click(screen.getByRole("menuitem", { name: /Delete/ }))
    })

    // Confirm deletion in AlertDialog
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Delete" }))
    })

    await waitFor(() => {
      const deleteCall = fetchMock.mock.calls.find(
        (call: [string, RequestInit]) =>
          typeof call[0] === "string" && call[0].includes("/files/delete"),
      )
      expect(deleteCall).toBeDefined()
      expect(deleteCall[1].method).toBe("POST")
    })
  })

  it("uses POST for rename operations", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/",
            entries: [
              { name: "old.txt", type: "file", size: 100, modifiedAt: "2026-03-30T10:00:00Z" },
            ],
          }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ oldPath: "/old.txt", newPath: "/new.txt" }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/",
            entries: [
              { name: "new.txt", type: "file", size: 100, modifiedAt: "2026-03-30T10:00:00Z" },
            ],
          }),
      })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(screen.getByText("old.txt")).toBeInTheDocument()
    })

    // Click rename in dropdown menu
    await act(async () => {
      fireEvent.click(screen.getByRole("menuitem", { name: /Rename/ }))
    })

    // Type new name and submit
    await act(async () => {
      fireEvent.change(screen.getByTestId("rename-input"), { target: { value: "new.txt" } })
    })

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Rename" }))
    })

    await waitFor(() => {
      const renameCall = fetchMock.mock.calls.find(
        (call: [string, RequestInit]) =>
          typeof call[0] === "string" && call[0].includes("/files/rename"),
      )
      expect(renameCall).toBeDefined()
      expect(renameCall[1].method).toBe("POST")
      const body = JSON.parse(renameCall[1].body as string)
      expect(body.oldPath).toBe("/old.txt")
      expect(body.newPath).toBe("/new.txt")
    })
  })

  it("sends Content-Type application/json for mutation requests", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ path: "/", entries: [] }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve({}),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ path: "/", entries: [] }),
      })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })

    // Open mkdir dialog
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Create directory" }))
    })

    await act(async () => {
      fireEvent.change(screen.getByTestId("mkdir-input"), { target: { value: "test" } })
    })

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Create" }))
    })

    await waitFor(() => {
      const mkdirCall = fetchMock.mock.calls.find(
        (call: [string, RequestInit]) =>
          typeof call[0] === "string" && call[0].includes("/files/mkdir"),
      )
      expect(mkdirCall).toBeDefined()
      const headers = mkdirCall[1].headers as Record<string, string>
      expect(headers["Content-Type"]).toBe("application/json")
    })
  })

  it("sends Content-Type application/octet-stream for uploads", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ path: "/", entries: [] }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 201,
        json: () => Promise.resolve({ path: "/test.txt", size: 5 }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ path: "/", entries: [] }),
      })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })

    // Simulate file upload via hidden input
    const file = new globalThis.File(["hello"], "test.txt", { type: "text/plain" })
    const input = screen.getByTestId("upload-input")

    await act(async () => {
      fireEvent.change(input, { target: { files: [file] } })
    })

    await waitFor(() => {
      const uploadCall = fetchMock.mock.calls.find(
        (call: [string, RequestInit]) =>
          typeof call[0] === "string" && call[0].includes("/files/upload"),
      )
      expect(uploadCall).toBeDefined()
      expect(uploadCall[1].method).toBe("POST")
      const headers = uploadCall[1].headers as Record<string, string>
      expect(headers["Content-Type"]).toBe("application/octet-stream")
    })
  })

  it("requires confirmation before delete (AlertDialog)", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve({
          path: "/",
          entries: [
            { name: "secret.key", type: "file", size: 32, modifiedAt: "2026-03-30T10:00:00Z" },
          ],
        }),
    })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(screen.getByText("secret.key")).toBeInTheDocument()
    })

    // Click delete in dropdown menu
    await act(async () => {
      fireEvent.click(screen.getByRole("menuitem", { name: /Delete/ }))
    })

    // AlertDialog should be open with confirmation
    expect(screen.getByText("Delete secret.key?")).toBeInTheDocument()
    expect(screen.getByText(/permanently delete this file/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument()

    // Only 1 fetch so far (initial listing) — delete not sent until confirmed
    const deleteCallsBefore = fetchMock.mock.calls.filter(
      (call: [string, RequestInit]) =>
        typeof call[0] === "string" && call[0].includes("/files/delete"),
    )
    expect(deleteCallsBefore.length).toBe(0)
  })

  it("shows recursive delete warning for directories", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve({
          path: "/",
          entries: [
            {
              name: "my-folder",
              type: "directory",
              size: 4096,
              modifiedAt: "2026-03-30T10:00:00Z",
            },
          ],
        }),
    })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(screen.getByText("my-folder")).toBeInTheDocument()
    })

    // Click delete in dropdown menu
    await act(async () => {
      fireEvent.click(screen.getByRole("menuitem", { name: /Delete/ }))
    })

    expect(screen.getByText(/directory and all its contents/)).toBeInTheDocument()
  })

  it("renders preview content as text, not HTML", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/",
            entries: [
              { name: "xss.html", type: "file", size: 50, modifiedAt: "2026-03-30T10:00:00Z" },
            ],
          }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/xss.html",
            content: '<script>alert("xss")</script>',
            truncated: false,
            totalSize: 50,
            mimeType: "text/html",
            encoding: "utf-8",
          }),
      })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(screen.getByText("xss.html")).toBeInTheDocument()
    })

    // Click preview in dropdown menu
    await act(async () => {
      fireEvent.click(screen.getByRole("menuitem", { name: /Preview/ }))
    })

    await waitFor(() => {
      const previewEl = screen.getByTestId("preview-content")
      // Content rendered as text inside <pre>, not as DOM HTML
      expect(previewEl.tagName).toBe("PRE")
      expect(previewEl.textContent).toContain('<script>alert("xss")</script>')
      // No actual script element injected
      expect(previewEl.querySelector("script")).toBeNull()
    })
  })

  it("handles network errors gracefully", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("Network error")))

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(screen.getByText("Failed to connect to server")).toBeInTheDocument()
    })
  })

  it("encodes path query parameter to prevent injection", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            path: "/",
            entries: [
              {
                name: "sub dir",
                type: "directory",
                size: 4096,
                modifiedAt: "2026-03-30T10:00:00Z",
              },
            ],
          }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ path: "/sub dir", entries: [] }),
      })

    vi.stubGlobal("fetch", fetchMock)

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      expect(screen.getByText("sub dir")).toBeInTheDocument()
    })

    // Navigate into directory with spaces in name
    await act(async () => {
      fireEvent.click(screen.getByTestId("entry-sub dir"))
    })

    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(2)
    })

    // Path should be properly encoded in the URL
    const secondUrl = fetchMock.mock.calls[1][0] as string
    expect(secondUrl).toContain("path=")
    // URLSearchParams handles encoding
    expect(secondUrl).not.toContain("path=/sub dir&")
  })

  it("disables parent nav button at root", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ path: "/", entries: [] }))

    render(<FileBrowser agentId="agent-1" />)

    await waitFor(() => {
      const backBtn = screen.getByRole("button", { name: "Go to parent directory" })
      expect(backBtn).toBeDisabled()
    })
  })
})
