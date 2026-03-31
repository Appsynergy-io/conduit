"use client"

import { AppSidebar } from "@/components/app-sidebar"
import { AuthGuard } from "@/components/auth-guard"
import { Badge } from "@/components/ui/badge"
import { Separator } from "@/components/ui/separator"
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar"
import { useWebSocket } from "@/hooks/use-websocket"
import { WebSocketProvider } from "@/hooks/use-websocket-context"

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return (
    <AuthGuard>
      <DashboardShell>{children}</DashboardShell>
    </AuthGuard>
  )
}

function DashboardShell({ children }: { children: React.ReactNode }) {
  const ws = useWebSocket()

  return (
    <WebSocketProvider value={ws}>
      <SidebarProvider>
        <AppSidebar />
        <SidebarInset>
          <header className="flex h-12 items-center gap-2 border-b px-4">
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 h-4" />
            <div className="flex flex-1 items-center justify-end gap-2">
              <Badge variant={ws.connected ? "default" : "secondary"} className="text-xs">
                {ws.connected ? "Live" : "Connecting..."}
              </Badge>
            </div>
          </header>
          <main className="flex-1 overflow-auto p-6">{children}</main>
        </SidebarInset>
      </SidebarProvider>
    </WebSocketProvider>
  )
}
