"use client";

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

function TerminalContent() {
  const searchParams = useSearchParams();
  const agentId = searchParams.get("agent");

  if (!agentId) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-12 text-center">
        <p className="text-lg font-medium">No agent selected</p>
        <p className="mt-1 text-sm text-muted-foreground">
          Select an agent from the dashboard to open a terminal session.
        </p>
      </div>
    );
  }

  return (
    <div className="flex h-[calc(100vh-7rem)] flex-col">
      <Card className="flex flex-1 flex-col">
        <CardHeader className="pb-3">
          <CardTitle className="text-lg">Terminal Session</CardTitle>
          <CardDescription>
            Agent: <code className="text-xs">{agentId}</code>
          </CardDescription>
        </CardHeader>
        <CardContent className="flex flex-1 items-center justify-center rounded-b-lg bg-black text-green-400 font-mono text-sm">
          <p>xterm.js integration pending</p>
        </CardContent>
      </Card>
    </div>
  );
}

export default function TerminalPage() {
  return (
    <Suspense>
      <TerminalContent />
    </Suspense>
  );
}
