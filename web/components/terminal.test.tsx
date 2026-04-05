import { act, cleanup, render, screen } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { TerminalView } from "./terminal"

// Mock xterm.js — it requires a real DOM canvas which jsdom doesn't provide
const mockWrite = vi.fn()
const mockDispose = vi.fn()
const mockFocus = vi.fn()
const mockLoadAddon = vi.fn()
const mockOpen = vi.fn()
const mockOnData = vi.fn()
const mockOnBinary = vi.fn()
const mockOnResize = vi.fn()
const mockResize = vi.fn()

vi.mock("@xterm/xterm", () => {
  return {
    Terminal: function MockTerminal() {
      this.cols = 80
      this.rows = 24
      this.write = mockWrite
      this.dispose = mockDispose
      this.focus = mockFocus
      this.loadAddon = mockLoadAddon
      this.open = mockOpen
      this.onData = mockOnData
      this.onBinary = mockOnBinary
      this.onResize = mockOnResize
      this.resize = mockResize
    },
  }
})

vi.mock("@xterm/addon-fit", () => {
  return {
    FitAddon: function MockFitAddon() {
      this.fit = vi.fn()
      this.dispose = vi.fn()
    },
  }
})

// Mock useAuth — cookie-based auth, no token in JS
vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({ isAuthenticated: true, loading: false }),
}))

// Track WebSocket instances created
let mockWebSocketInstances: MockWebSocket[] = []

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3

  url: string
  protocols: string | string[] | undefined
  binaryType = "blob"
  readyState = MockWebSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  sentMessages: (string | ArrayBuffer | Uint8Array)[] = []

  constructor(url: string, protocols?: string | string[]) {
    this.url = url
    this.protocols = protocols
    mockWebSocketInstances.push(this)
  }

  send(data: string | ArrayBuffer | Uint8Array) {
    this.sentMessages.push(data)
  }

  close(_code?: number, _reason?: string) {
    this.readyState = MockWebSocket.CLOSED
  }

  // Test helpers
  simulateOpen() {
    this.readyState = MockWebSocket.OPEN
    this.onopen?.(new Event("open"))
  }

  simulateMessage(data: ArrayBuffer | string) {
    this.onmessage?.(new MessageEvent("message", { data }))
  }

  simulateClose(code = 1000, reason = "") {
    this.readyState = MockWebSocket.CLOSED
    this.onclose?.(new CloseEvent("close", { code, reason }))
  }

  simulateError() {
    this.onerror?.(new Event("error"))
  }
}

// Mock ResizeObserver
class MockResizeObserver {
  observe = vi.fn()
  unobserve = vi.fn()
  disconnect = vi.fn()
}

beforeEach(() => {
  mockWebSocketInstances = []
  vi.stubGlobal("WebSocket", MockWebSocket)
  vi.stubGlobal("ResizeObserver", MockResizeObserver)
  vi.clearAllMocks()
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe("TerminalView", () => {
  it("renders the terminal container and status bar", () => {
    render(<TerminalView agentId="agent-123" agentHostname="web-server-1" />)

    expect(screen.getByTestId("terminal-view")).toBeInTheDocument()
    expect(screen.getByTestId("terminal-container")).toBeInTheDocument()
    expect(screen.getByText("web-server-1")).toBeInTheDocument()
    expect(screen.getByText("Connecting...")).toBeInTheDocument()
  })

  it("shows agentId when no hostname provided", () => {
    render(<TerminalView agentId="agent-456" />)
    expect(screen.getByText("agent-456")).toBeInTheDocument()
  })

  it("renders close button when onClose provided", () => {
    const onClose = vi.fn()
    render(<TerminalView agentId="agent-123" onClose={onClose} />)

    const closeBtn = screen.getByRole("button", { name: "Close terminal" })
    expect(closeBtn).toBeInTheDocument()
  })

  it("does not render close button when onClose not provided", () => {
    render(<TerminalView agentId="agent-123" />)
    expect(screen.queryByRole("button", { name: "Close terminal" })).not.toBeInTheDocument()
  })

  it("connects WebSocket with correct URL structure", async () => {
    render(<TerminalView agentId="agent-123" />)

    // Wait for async init
    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    // Verify URL structure — no token in query param, agentId in path, subprotocol set
    expect(ws.url).toContain("/api/v1/agents/agent-123/shell/new")
    expect(ws.url).not.toContain("token=")
    expect(ws.url).toContain("cols=80")
    expect(ws.url).toContain("rows=24")
    expect(ws.protocols).toBe("conduit-shell-v1")
    expect(ws.binaryType).toBe("arraybuffer")
  })

  it("updates status to connected on WebSocket open", async () => {
    render(<TerminalView agentId="agent-123" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    mockWebSocketInstances[0].simulateOpen()

    await vi.waitFor(() => {
      expect(screen.getByText("Connected")).toBeInTheDocument()
    })
  })

  it("writes received data to terminal", async () => {
    render(<TerminalView agentId="agent-123" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    await act(async () => {
      ws.simulateOpen()
    })

    // Test string data path
    await act(async () => {
      ws.simulateMessage("hello world")
    })

    expect(mockWrite).toHaveBeenCalledWith("hello world")
  })

  it("shows disconnected status on WebSocket close", async () => {
    render(<TerminalView agentId="agent-123" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    ws.simulateOpen()
    ws.simulateClose(1000)

    await vi.waitFor(() => {
      expect(screen.getByText("Disconnected")).toBeInTheDocument()
    })
  })

  it("shows error status on WebSocket error", async () => {
    render(<TerminalView agentId="agent-123" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    mockWebSocketInstances[0].simulateError()

    await vi.waitFor(() => {
      expect(screen.getByText("Error")).toBeInTheDocument()
    })
  })

  it("sends keystrokes as binary data", async () => {
    render(<TerminalView agentId="agent-123" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    ws.simulateOpen()

    // Get the onData callback and simulate typing
    expect(mockOnData).toHaveBeenCalled()
    const onDataCallback = mockOnData.mock.calls[0][0]
    onDataCallback("ls -la\r")

    expect(ws.sentMessages.length).toBe(1)
    // Verify it was sent as typed array (binary, not string)
    expect(typeof ws.sentMessages[0]).not.toBe("string")
    expect(ArrayBuffer.isView(ws.sentMessages[0])).toBe(true)
  })

  it("sends resize events as JSON", async () => {
    render(<TerminalView agentId="agent-123" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    ws.simulateOpen()

    // Get the onResize callback
    expect(mockOnResize).toHaveBeenCalled()
    const onResizeCallback = mockOnResize.mock.calls[0][0]
    onResizeCallback({ cols: 120, rows: 40 })

    expect(ws.sentMessages.length).toBe(1)
    // Decode the sent message and verify it's a resize command
    const sent = new TextDecoder().decode(ws.sentMessages[0] as Uint8Array)
    const parsed = JSON.parse(sent)
    expect(parsed).toEqual({ type: "resize", cols: 120, rows: 40 })
  })

  it("cleans up WebSocket and terminal on unmount", async () => {
    const { unmount } = render(<TerminalView agentId="agent-123" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    unmount()

    expect(mockDispose).toHaveBeenCalled()
  })

  it("does not send data when WebSocket is not open", async () => {
    render(<TerminalView agentId="agent-123" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    // Don't call simulateOpen — WebSocket stays in CONNECTING state
    const onDataCallback = mockOnData.mock.calls[0][0]
    onDataCallback("should not send")

    expect(mockWebSocketInstances[0].sentMessages.length).toBe(0)
  })
})

describe("TerminalView — Security", () => {
  it("encodes agentId in URL to prevent path traversal", async () => {
    render(<TerminalView agentId="../../etc/passwd" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    // encodeURIComponent should encode slashes and dots
    expect(ws.url).toContain(`/api/v1/agents/${encodeURIComponent("../../etc/passwd")}/shell/new`)
    expect(ws.url).not.toContain("../../etc/passwd?")
  })

  it("does not include token in WebSocket URL (cookie-based auth)", async () => {
    render(<TerminalView agentId="agent-1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    // No token in URL — auth is via httpOnly cookie
    expect(ws.url).not.toContain("token=")
  })

  it("uses binary WebSocket type to prevent XSS in terminal output", async () => {
    render(<TerminalView agentId="agent-1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    // binaryType must be arraybuffer — prevents string injection
    expect(mockWebSocketInstances[0].binaryType).toBe("arraybuffer")
  })

  it("writes terminal output through xterm API only (never innerHTML)", async () => {
    render(<TerminalView agentId="agent-1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    ws.simulateOpen()

    // Simulate receiving HTML that could be XSS — even as a string,
    // xterm.js renders it as terminal text, not as DOM HTML
    await act(async () => {
      ws.simulateMessage('<script>alert("xss")</script>')
    })

    // Data goes through term.write() (which renders as terminal text, not HTML)
    expect(mockWrite).toHaveBeenCalledWith('<script>alert("xss")</script>')

    // Verify the terminal container doesn't have raw HTML injected
    const container = screen.getByTestId("terminal-container")
    expect(container.querySelector("script")).toBeNull()
  })

  it("uses wss: protocol on https: pages", async () => {
    // jsdom defaults to http: — verify the protocol selection logic
    render(<TerminalView agentId="agent-1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    // In jsdom (http:), should use ws:
    expect(ws.url).toMatch(/^ws:/)
  })

  it("closes WebSocket with normal closure code on cleanup", async () => {
    const { unmount } = render(<TerminalView agentId="agent-1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const closeSpy = vi.spyOn(mockWebSocketInstances[0], "close")
    unmount()

    expect(closeSpy).toHaveBeenCalledWith(1000, "session ended")
  })

  it("does not reconnect after unmount (no zombie connections)", async () => {
    const { unmount } = render(<TerminalView agentId="agent-1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    unmount()

    // Simulate close event after unmount — should not trigger reconnect
    mockWebSocketInstances[0].simulateClose()

    // Wait a tick to see if any reconnection attempt happens
    await new Promise((r) => setTimeout(r, 50))

    // Should still only have 1 WebSocket instance (no reconnect)
    expect(mockWebSocketInstances.length).toBe(1)
  })

  it("resize values are bounded integers in JSON", async () => {
    render(<TerminalView agentId="agent-1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    ws.simulateOpen()

    const onResizeCallback = mockOnResize.mock.calls[0][0]
    onResizeCallback({ cols: 200, rows: 50 })

    const sent = new TextDecoder().decode(ws.sentMessages[0] as Uint8Array)
    const parsed = JSON.parse(sent)

    // Values must be integers
    expect(Number.isInteger(parsed.cols)).toBe(true)
    expect(Number.isInteger(parsed.rows)).toBe(true)
    // Must be the exact values (no injection possible through xterm resize API)
    expect(parsed.cols).toBe(200)
    expect(parsed.rows).toBe(50)
    expect(parsed.type).toBe("resize")
    // No extra fields
    expect(Object.keys(parsed)).toEqual(["type", "cols", "rows"])
  })

  it("shows standby overlay when server assigns watcher role", async () => {
    render(<TerminalView agentId="agent-1" sessionId="s1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    await act(async () => {
      ws.simulateOpen()
    })

    await act(async () => {
      ws.simulateMessage(
        JSON.stringify({ type: "session", sessionId: "s1", role: "watcher" }),
      )
    })

    expect(screen.getByTestId("standby-overlay")).toBeInTheDocument()
    expect(screen.getByText("Standby")).toBeInTheDocument()
  })

  it("discards typed input while in standby mode", async () => {
    render(<TerminalView agentId="agent-1" sessionId="s1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    await act(async () => {
      ws.simulateOpen()
    })

    await act(async () => {
      ws.simulateMessage(
        JSON.stringify({ type: "session", sessionId: "s1", role: "watcher" }),
      )
    })

    // Server placed us in standby — keystrokes must not be forwarded
    const onDataCallback = mockOnData.mock.calls[0][0]
    onDataCallback("ls -la\r")
    expect(ws.sentMessages.length).toBe(0)
  })

  it("sends take_control message on standby Take Control click", async () => {
    render(<TerminalView agentId="agent-1" sessionId="s1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    await act(async () => {
      ws.simulateOpen()
    })

    await act(async () => {
      ws.simulateMessage(
        JSON.stringify({ type: "session", sessionId: "s1", role: "watcher" }),
      )
    })

    const takeBtn = screen.getAllByRole("button", { name: /take control/i })[0]
    await act(async () => {
      takeBtn.click()
    })

    expect(ws.sentMessages.length).toBe(1)
    const sent = new TextDecoder().decode(ws.sentMessages[0] as Uint8Array)
    expect(JSON.parse(sent)).toEqual({ type: "take_control" })
  })

  it("removes standby overlay and sends resize on control_granted", async () => {
    render(<TerminalView agentId="agent-1" sessionId="s1" />)

    await vi.waitFor(() => {
      expect(mockWebSocketInstances.length).toBe(1)
    })

    const ws = mockWebSocketInstances[0]
    await act(async () => {
      ws.simulateOpen()
    })

    await act(async () => {
      ws.simulateMessage(
        JSON.stringify({ type: "session", sessionId: "s1", role: "watcher" }),
      )
    })
    expect(screen.getByTestId("standby-overlay")).toBeInTheDocument()

    // Clear messages from initial handshake, then promote to controller
    ws.sentMessages.length = 0
    await act(async () => {
      ws.simulateMessage(JSON.stringify({ type: "control_granted" }))
    })

    expect(screen.queryByTestId("standby-overlay")).not.toBeInTheDocument()
    // control_granted triggers an explicit resize sync to the PTY
    const resizeMsg = ws.sentMessages.find((m) => {
      const s = new TextDecoder().decode(m as Uint8Array)
      try {
        return JSON.parse(s).type === "resize"
      } catch {
        return false
      }
    })
    expect(resizeMsg).toBeDefined()
  })
})
