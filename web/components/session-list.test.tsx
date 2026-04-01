import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { SessionList } from "./session-list"

// Mock useAuth — cookie-based auth, no token in JS
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ isAuthenticated: true, loading: false }),
}))

// Mock Radix Tooltip — jsdom doesn't support pointer events or portals that Radix requires
vi.mock("@/components/ui/tooltip", () => {
  return {
    TooltipProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
    Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
    TooltipTrigger: ({
      children,
      asChild,
    }: {
      children: ReactNode
      asChild?: boolean
    }) => {
      if (asChild) return children
      return <span>{children}</span>
    },
    TooltipContent: ({ children }: { children: ReactNode }) => (
      <span role="tooltip">{children}</span>
    ),
  }
})

// ── Test data ──

function makeSession(overrides: Record<string, unknown> = {}) {
  return {
    id: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
    agentId: "agent-001",
    userId: "user-001",
    status: "active" as const,
    pinned: 0,
    idleTimeout: 3600,
    cols: 80,
    rows: 24,
    createdAt: new Date().toISOString(),
    detachedAt: null,
    closedAt: null,
    ...overrides,
  }
}

const activeSession = makeSession({
  id: "11111111-aaaa-bbbb-cccc-dddddddddddd",
  status: "active",
  pinned: 0,
})

const detachedSession = makeSession({
  id: "22222222-aaaa-bbbb-cccc-dddddddddddd",
  agentId: "agent-002",
  status: "detached",
  pinned: 0,
  detachedAt: new Date().toISOString(),
})

const pinnedSession = makeSession({
  id: "33333333-aaaa-bbbb-cccc-dddddddddddd",
  status: "active",
  pinned: 1,
})

// ── Fetch mock helpers ──

function mockFetchSuccess(data: unknown, status = 200) {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(data),
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
  vi.useFakeTimers({ shouldAdvanceTime: true })
  vi.clearAllMocks()
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

describe("SessionList", () => {
  it("shows loading state initially", () => {
    // Fetch never resolves so we stay in loading
    vi.stubGlobal(
      "fetch",
      vi.fn().mockReturnValue(new Promise(() => {})),
    )

    render(<SessionList agentId="agent-001" />)

    expect(screen.getByText("Loading sessions...")).toBeInTheDocument()
  })

  it("shows empty state when no sessions returned", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ data: [] }))

    render(<SessionList agentId="agent-001" />)

    await waitFor(() => {
      expect(screen.getByText("No active sessions")).toBeInTheDocument()
    })
  })

  it("shows empty state when data field is null", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ data: null }))

    render(<SessionList agentId="agent-001" />)

    await waitFor(() => {
      expect(screen.getByText("No active sessions")).toBeInTheDocument()
    })
  })

  it("renders sessions with truncated IDs", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ data: [activeSession] }))

    render(<SessionList agentId="agent-001" />)

    await waitFor(() => {
      // ID is truncated to first 8 chars
      expect(screen.getByText(activeSession.id.slice(0, 8))).toBeInTheDocument()
    })
  })

  it("renders active status badge", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ data: [activeSession] }))

    render(<SessionList agentId="agent-001" />)

    await waitFor(() => {
      expect(screen.getByText("active")).toBeInTheDocument()
    })
  })

  it("renders detached status badge", async () => {
    vi.stubGlobal("fetch", mockFetchSuccess({ data: [detachedSession] }))

    render(<SessionList agentId="agent-002" />)

    await waitFor(() => {
      expect(screen.getByText("detached")).toBeInTheDocument()
    })
  })

  it("shows session count in header", async () => {
    vi.stubGlobal(
      "fetch",
      mockFetchSuccess({ data: [activeSession, detachedSession] }),
    )

    render(<SessionList />)

    await waitFor(() => {
      expect(screen.getByText("Sessions (2)")).toBeInTheDocument()
    })
  })

  it("fetches from agent-specific URL when agentId is provided", async () => {
    const fetchMock = mockFetchSuccess({ data: [] })
    vi.stubGlobal("fetch", fetchMock)

    render(<SessionList agentId="agent-xyz" />)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })

    const url = fetchMock.mock.calls[0][0] as string
    expect(url).toBe("/api/v1/agents/agent-xyz/shell/sessions")
  })

  it("fetches from global URL when agentId is not provided", async () => {
    const fetchMock = mockFetchSuccess({ data: [] })
    vi.stubGlobal("fetch", fetchMock)

    render(<SessionList />)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })

    const url = fetchMock.mock.calls[0][0] as string
    expect(url).toBe("/api/v1/shell/sessions")
  })

  it("encodes agentId in URL to prevent path traversal", async () => {
    const fetchMock = mockFetchSuccess({ data: [] })
    vi.stubGlobal("fetch", fetchMock)

    render(<SessionList agentId="../../etc/passwd" />)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })

    const url = fetchMock.mock.calls[0][0] as string
    expect(url).toContain(encodeURIComponent("../../etc/passwd"))
    expect(url).not.toContain("../../etc/passwd/shell")
  })

  it("terminate button calls DELETE and removes session from list", async () => {
    const onTerminate = vi.fn()
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ data: [activeSession, detachedSession] }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 204,
        json: () => Promise.resolve({}),
      })

    vi.stubGlobal("fetch", fetchMock)

    render(<SessionList onTerminate={onTerminate} />)

    await waitFor(() => {
      expect(screen.getByText(activeSession.id.slice(0, 8))).toBeInTheDocument()
      expect(screen.getByText(detachedSession.id.slice(0, 8))).toBeInTheDocument()
    })

    // Find terminate buttons (the Square icon buttons) — there should be one per session
    const terminateButtons = screen.getAllByRole("tooltip", { name: "Terminate" })
    // Each Terminate tooltip is next to a button — get the buttons
    // With our mock, TooltipContent renders as <span role="tooltip">Terminate</span>
    // The button is a sibling. Let's find buttons by looking at the structure.
    // The terminate button is inside each session item. Find all buttons with destructive class.
    const allButtons = screen.getAllByRole("button")
    // Filter to the terminate buttons — they have text-destructive class
    const terminateBtns = allButtons.filter((btn) =>
      btn.className.includes("text-destructive"),
    )
    expect(terminateBtns.length).toBe(2)

    // Click the first terminate button (for activeSession)
    await act(async () => {
      fireEvent.click(terminateBtns[0])
    })

    // Verify DELETE was called with correct URL
    await waitFor(() => {
      const deleteCall = fetchMock.mock.calls.find(
        (call: [string, RequestInit?]) =>
          typeof call[1] === "object" && call[1]?.method === "DELETE",
      )
      expect(deleteCall).toBeDefined()
      expect(deleteCall[0]).toContain(
        `/api/v1/agents/${encodeURIComponent(activeSession.agentId)}/shell/sessions/${encodeURIComponent(activeSession.id)}`,
      )
    })

    // The session should be removed from the list
    await waitFor(() => {
      expect(screen.queryByText(activeSession.id.slice(0, 8))).not.toBeInTheDocument()
    })

    // onTerminate callback should have been called
    expect(onTerminate).toHaveBeenCalledWith(activeSession.id)

    // The other session should still be visible
    expect(screen.getByText(detachedSession.id.slice(0, 8))).toBeInTheDocument()
  })

  it("clicking a session row calls onAttach", async () => {
    const onAttach = vi.fn()
    vi.stubGlobal(
      "fetch",
      mockFetchSuccess({ data: [detachedSession] }),
    )

    render(<SessionList onAttach={onAttach} />)

    await waitFor(() => {
      expect(screen.getByText(detachedSession.id.slice(0, 8))).toBeInTheDocument()
    })

    // Click the session row — the row div containing the session ID
    const sessionRow = screen.getByText(detachedSession.id.slice(0, 8)).closest("[class*='rounded-md']")
    expect(sessionRow).toBeTruthy()

    await act(async () => {
      fireEvent.click(sessionRow!)
    })

    expect(onAttach).toHaveBeenCalledTimes(1)
    expect(onAttach).toHaveBeenCalledWith(
      expect.objectContaining({
        id: detachedSession.id,
        status: "detached",
      }),
    )
  })

  it("clicking the active session row does not call onAttach", async () => {
    const onAttach = vi.fn()
    vi.stubGlobal(
      "fetch",
      mockFetchSuccess({ data: [activeSession] }),
    )

    render(<SessionList activeSessionId={activeSession.id} onAttach={onAttach} />)

    await waitFor(() => {
      expect(screen.getByText(activeSession.id.slice(0, 8))).toBeInTheDocument()
    })

    // Click the active session row — should be a no-op
    const sessionRow = screen.getByText(activeSession.id.slice(0, 8)).closest("[class*='rounded-md']")
    expect(sessionRow).toBeTruthy()

    await act(async () => {
      fireEvent.click(sessionRow!)
    })

    expect(onAttach).not.toHaveBeenCalled()
  })

  it("renders pin icon for pinned sessions", async () => {
    vi.stubGlobal(
      "fetch",
      mockFetchSuccess({ data: [pinnedSession, activeSession] }),
    )

    const { container } = render(<SessionList />)

    await waitFor(() => {
      expect(screen.getByText(pinnedSession.id.slice(0, 8))).toBeInTheDocument()
    })

    // The Pin icon from lucide renders as an SVG. Pinned session should have
    // the pin icon SVG. We verify by counting SVGs with the lucide-pin class or
    // by checking that the pinned session's row has a Pin icon.
    // lucide-react renders SVGs with class "lucide". The Pin icon is only rendered
    // for pinned sessions.
    // With pinnedSession (pinned=1) and activeSession (pinned=0), there should be
    // exactly one Pin icon SVG element.
    const pinIcons = container.querySelectorAll(".lucide-pin")
    expect(pinIcons.length).toBe(1)
  })

  it("refresh button triggers a new fetch", async () => {
    const fetchMock = mockFetchSuccess({ data: [activeSession] })
    vi.stubGlobal("fetch", fetchMock)

    render(<SessionList agentId="agent-001" />)

    await waitFor(() => {
      expect(screen.getByText(activeSession.id.slice(0, 8))).toBeInTheDocument()
    })

    const initialCallCount = fetchMock.mock.calls.length

    // The refresh button is inside the header, next to the "Sessions (N)" text.
    const headerArea = screen.getByText(/Sessions \(\d+\)/).closest("div")
    const refreshBtn = headerArea?.querySelector("button")
    expect(refreshBtn).toBeTruthy()

    await act(async () => {
      fireEvent.click(refreshBtn!)
    })

    expect(fetchMock.mock.calls.length).toBeGreaterThan(initialCallCount)
  })

  it("handles fetch error gracefully — shows empty state", async () => {
    vi.stubGlobal("fetch", mockFetchError(500))

    render(<SessionList agentId="agent-001" />)

    // On error, res.ok is false so sessions stays empty => shows "No active sessions"
    await waitFor(() => {
      expect(screen.getByText("No active sessions")).toBeInTheDocument()
    })
  })

  it("handles network error gracefully — shows empty state", async () => {
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("Network error")))

    render(<SessionList agentId="agent-001" />)

    await waitFor(() => {
      expect(screen.getByText("No active sessions")).toBeInTheDocument()
    })
  })

  it("polls every 5 seconds", async () => {
    const fetchMock = mockFetchSuccess({ data: [activeSession] })
    vi.stubGlobal("fetch", fetchMock)

    render(<SessionList agentId="agent-001" />)

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1)
    })

    // Advance timer by 5 seconds to trigger the interval
    await act(async () => {
      vi.advanceTimersByTime(5000)
    })

    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(2)
    })

    // Advance another 5 seconds
    await act(async () => {
      vi.advanceTimersByTime(5000)
    })

    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBeGreaterThanOrEqual(3)
    })
  })
})

describe("SessionList — formatTimeAgo (indirect)", () => {
  it("shows 'just now' for recent sessions", async () => {
    const recentSession = makeSession({
      id: "44444444-aaaa-bbbb-cccc-dddddddddddd",
      createdAt: new Date().toISOString(),
    })

    vi.stubGlobal("fetch", mockFetchSuccess({ data: [recentSession] }))

    render(<SessionList />)

    await waitFor(() => {
      expect(screen.getByText("just now")).toBeInTheDocument()
    })
  })

  it("shows minutes ago for sessions created minutes ago", async () => {
    const fiveMinAgo = new Date(Date.now() - 5 * 60 * 1000).toISOString()
    const session = makeSession({
      id: "55555555-aaaa-bbbb-cccc-dddddddddddd",
      createdAt: fiveMinAgo,
    })

    vi.stubGlobal("fetch", mockFetchSuccess({ data: [session] }))

    render(<SessionList />)

    await waitFor(() => {
      expect(screen.getByText("5m ago")).toBeInTheDocument()
    })
  })

  it("shows hours ago for sessions created hours ago", async () => {
    const threeHoursAgo = new Date(Date.now() - 3 * 60 * 60 * 1000).toISOString()
    const session = makeSession({
      id: "66666666-aaaa-bbbb-cccc-dddddddddddd",
      createdAt: threeHoursAgo,
    })

    vi.stubGlobal("fetch", mockFetchSuccess({ data: [session] }))

    render(<SessionList />)

    await waitFor(() => {
      expect(screen.getByText("3h ago")).toBeInTheDocument()
    })
  })

  it("shows days ago for sessions created days ago", async () => {
    const twoDaysAgo = new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString()
    const session = makeSession({
      id: "77777777-aaaa-bbbb-cccc-dddddddddddd",
      createdAt: twoDaysAgo,
    })

    vi.stubGlobal("fetch", mockFetchSuccess({ data: [session] }))

    render(<SessionList />)

    await waitFor(() => {
      expect(screen.getByText("2d ago")).toBeInTheDocument()
    })
  })
})
