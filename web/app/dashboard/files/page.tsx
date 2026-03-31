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

function FileBrowserContent() {
  const searchParams = useSearchParams();
  const agentId = searchParams.get("agent");

  if (!agentId) {
    return (
      <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-12 text-center">
        <p className="text-lg font-medium">No agent selected</p>
        <p className="mt-1 text-sm text-muted-foreground">
          Select an agent from the dashboard to browse files.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">File Browser</h1>
        <p className="text-sm text-muted-foreground">
          Agent: <code className="text-xs">{agentId}</code>
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Files</CardTitle>
          <CardDescription>
            Browse, upload, and download files on the remote machine.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            File browser implementation pending.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}

export default function FileBrowserPage() {
  return (
    <Suspense>
      <FileBrowserContent />
    </Suspense>
  );
}
