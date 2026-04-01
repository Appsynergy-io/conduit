import { AuthGuard } from "@/components/auth-guard"

/**
 * Pop-out terminal layout — minimal chrome, no sidebar, no header.
 * Auth guard validates JWT before rendering terminal.
 */
export default function PopOutLayout({ children }: { children: React.ReactNode }) {
  return <AuthGuard>{children}</AuthGuard>
}
