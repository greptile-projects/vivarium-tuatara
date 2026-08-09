"use client";

import { useState } from "react";
import { useAuth } from "./auth";
import { Button } from "./ui";

export function PackageArtifactDownload({ name, version }: { name: string; version: string }) {
  const { token } = useAuth();
  const [downloading, setDownloading] = useState(false);
  const [error, setError] = useState("");

  async function download() {
    setDownloading(true);
    setError("");
    try {
      const headers = new Headers();
      if (token) headers.set("Authorization", `Bearer ${token}`);
      const response = await fetch(`/api/packages/${name}/versions/${encodeURIComponent(version)}/artifact`, { headers, cache: "no-store" });
      if (!response.ok) throw new Error(response.status === 404 ? "This package artifact is no longer available to this account." : "The package artifact could not be downloaded.");
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      const disposition = response.headers.get("Content-Disposition") ?? "";
      link.download = disposition.match(/filename="([^"]+)"/)?.[1] ?? `${name}-${version}`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 0);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : "The package artifact could not be downloaded.");
    } finally {
      setDownloading(false);
    }
  }

  return <span className="inline-flex flex-col items-start gap-1"><Button type="button" variant="quiet" className="px-0 text-[var(--brand)] hover:bg-transparent hover:underline" onClick={() => void download()} disabled={downloading}>{downloading ? "Downloading…" : "Download artifact"}</Button>{error && <span role="alert" className="text-xs text-[var(--danger)]">{error}</span>}</span>;
}
