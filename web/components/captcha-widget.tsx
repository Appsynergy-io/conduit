"use client"

import { AlertCircle, CheckCircle2, Loader2 } from "lucide-react"
import { useEffect, useRef, useState } from "react"
import { type CaptchaSolution, solveCaptcha } from "@/lib/captcha"

// Expected iterations at the server's default difficulty (18 leading zero
// bits → 2^18 ≈ 262144). Used only to render a bounded progress bar.
const EXPECTED_ITERATIONS = 1 << 18

type Status = "idle" | "solving" | "solved" | "error"

export interface CaptchaWidgetProps {
  /** Server endpoint the challenge is bound to. */
  endpoint: string
  /** Called once a valid solution is produced. */
  onSolved: (solution: CaptchaSolution) => void
}

/**
 * Visible proof-of-work CAPTCHA widget, modeled on Cloudflare Turnstile's
 * click-to-verify checkbox. The user clicks the checkbox; a Web Worker fetches
 * a challenge from the server and solves it locally. Progress is shown while
 * the solver runs and a green checkmark is shown when a valid nonce is found.
 * The surrounding form waits for the onSolved callback before submitting
 * (NIST SC-5 DoS/bot mitigation, OWASP API4/API6).
 */
export function CaptchaWidget({ endpoint, onSolved }: CaptchaWidgetProps) {
  const [status, setStatus] = useState<Status>("idle")
  const [progress, setProgress] = useState(0)
  const [error, setError] = useState<string | null>(null)
  // Ref guard: prevents double-solve if the checkbox is clicked rapidly.
  const solving = useRef(false)
  // Track whether the component is mounted so the async solve doesn't call
  // setState after unmount.
  const mounted = useRef(true)

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
    }
  }, [])

  function startSolve() {
    if (solving.current) return
    if (status === "solving" || status === "solved") return
    solving.current = true
    setStatus("solving")
    setError(null)
    setProgress(0)

    solveCaptcha(endpoint, (p) => {
      if (!mounted.current) return
      const pct = Math.min(99, (p.iterations / EXPECTED_ITERATIONS) * 100)
      setProgress(pct)
    })
      .then((sol) => {
        if (!mounted.current) return
        setProgress(100)
        setStatus("solved")
        onSolved(sol)
      })
      .catch(() => {
        if (!mounted.current) return
        setStatus("error")
        setError("Verification failed. Click to try again.")
        solving.current = false
      })
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === " " || e.key === "Enter") {
      e.preventDefault()
      startSolve()
    }
  }

  const clickable = status === "idle" || status === "error"
  const label =
    status === "idle"
      ? "I'm human"
      : status === "solving"
        ? "Verifying..."
        : status === "solved"
          ? "Verified"
          : "Try again"

  return (
    <button
      type="button"
      onClick={clickable ? startSolve : undefined}
      onKeyDown={handleKeyDown}
      disabled={!clickable}
      aria-pressed={status === "solved"}
      aria-busy={status === "solving"}
      aria-label="Verify you're human"
      className="flex w-full items-center gap-3 rounded-md border bg-muted/30 p-3 text-left transition-colors enabled:hover:bg-muted/50 disabled:cursor-not-allowed"
    >
      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded border-2 border-muted-foreground/40 bg-background">
        {status === "idle" && null}
        {status === "solving" && <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />}
        {status === "solved" && (
          <CheckCircle2 className="h-4 w-4 text-green-600 dark:text-green-400" />
        )}
        {status === "error" && <AlertCircle className="h-4 w-4 text-destructive" />}
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium">{label}</p>
        {status === "solving" && (
          <div className="mt-1.5 h-1 w-full overflow-hidden rounded-full bg-muted">
            <div
              className="h-full bg-primary transition-all duration-150"
              style={{ width: `${progress}%` }}
            />
          </div>
        )}
        {status === "error" && error && <p className="mt-0.5 text-xs text-destructive">{error}</p>}
      </div>
    </button>
  )
}
