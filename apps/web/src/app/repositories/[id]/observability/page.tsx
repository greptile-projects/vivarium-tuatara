import type { Metadata } from "next";
import { ObservabilityGapsWorkspace } from "@/components/observability-gaps-workspace";
import { TelemetryContractsWorkspace } from "@/components/telemetry-contracts-workspace";
import { SignalRolloutsWorkspace } from "@/components/signal-rollouts-workspace";
import { SignalEvaluationsWorkspace } from "@/components/signal-evaluations-workspace";
export const metadata: Metadata = { title: "Observability gaps" };
export default async function ObservabilityPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return (
    <div className="space-y-10">
      <ObservabilityGapsWorkspace repositoryID={id} />
      <TelemetryContractsWorkspace repositoryID={id} />
      <SignalRolloutsWorkspace repositoryID={id} />
      <SignalEvaluationsWorkspace repositoryID={id} />
    </div>
  );
}
