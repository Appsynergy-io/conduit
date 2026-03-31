"use client"

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">Server configuration and preferences.</p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Server Configuration</CardTitle>
          <CardDescription>Server settings will be available in a future release.</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Configuration is managed through{" "}
            <code className="rounded bg-muted px-1.5 py-0.5 text-xs">server.yaml</code> and CLI
            flags.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
