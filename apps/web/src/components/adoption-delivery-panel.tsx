"use client";
import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";
type Delivery = {
  id: string;
  provider_revision: string;
  release_revision: string;
  state: string;
  rollout: string[];
  health: string[];
  support_readiness: string;
  user_acceptance: string;
  cost_cents: number;
  currency: string;
  check_run_ids: string[];
  approval_ids: string[];
  pause_reasons: string[];
  authority: string;
};
type Workspace = {
  id: string;
  version: number;
  title: string;
  currency: string;
  plans: { id: string; selected_version: string }[] | null;
  deliveries: Delivery[] | null;
};
const lines = (value: FormDataEntryValue | null) =>
  String(value ?? "")
    .split("\n")
    .map((item) => item.trim())
    .filter(Boolean);
export function AdoptionDeliveryPanel() {
  const { token } = useAuth();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]),
    [selected, setSelected] = useState(""),
    [pending, setPending] = useState(false),
    [error, setError] = useState("");
  const load = useCallback(async () => {
    if (!token) return;
    const result = await api<{ adoption_workspaces: Workspace[] }>(
      "/adoption-workspaces",
      {},
      token,
    );
    setWorkspaces(result.adoption_workspaces);
    setSelected(
      (current) =>
        current ||
        result.adoption_workspaces.find((item) => (item.plans?.length ?? 0) > 0)
          ?.id ||
        "",
    );
  }, [token]);
  useEffect(() => {
    void Promise.resolve().then(load).catch((reason) =>
      setError(
        reason instanceof Error
          ? reason.message
          : "Unable to load governed deliveries.",
      ),
    );
  }, [load]);
  const workspace = workspaces.find((item) => item.id === selected),
    deliveries = workspace?.deliveries ?? [];
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!token || !workspace) return;
    setPending(true);
    setError("");
    const form = new FormData(event.currentTarget);
    try {
      const attestations = lines(form.get("attestations")).map((value) => {
        const [kind, statement, attested_by, satisfied] = value
          .split("|")
          .map((item) => item.trim());
        return {
          kind,
          statement,
          attested_by,
          satisfied: satisfied !== "unmet",
        };
      });
      await api(
        `/adoption-workspaces/${workspace.id}/deliveries`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: workspace.version,
            plan_id: form.get("plan"),
            consumer_repository_id: form.get("repository"),
            pull_request_id: form.get("pull"),
            release_id: form.get("release"),
            deployment_id: form.get("deployment"),
            restores_delivery_id: form.get("restores") || undefined,
            cost_cents: Number(form.get("cost")),
            currency: form.get("currency"),
            support_readiness: form.get("support"),
            user_acceptance: form.get("acceptance"),
            attestations,
          }),
        },
        token,
      );
      event.currentTarget.reset();
      await load();
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Unable to retain governed delivery.",
      );
    } finally {
      setPending(false);
    }
  }
  if (!workspace) return null;
  return (
    <Card className="mt-6 p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-semibold">Governed delivery</h2>
          <p className="mt-1 max-w-3xl text-sm text-[var(--muted)]">
            Exact provider and consumer revisions stay connected to ordinary
            review, checks, release, and staged deployment. This evidence grants
            no merge or environment authority.
          </p>
        </div>
        <select
          aria-label="Adoption workspace"
          value={selected}
          onChange={(event) => setSelected(event.target.value)}
          className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm"
        >
          {workspaces
            .filter((item) => (item.plans?.length ?? 0) > 0)
            .map((item) => (
              <option key={item.id} value={item.id}>
                {item.title}
              </option>
            ))}
        </select>
      </div>
      {error && (
        <p
          role="alert"
          className="mt-4 rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      <div className="mt-4 space-y-3">
        {deliveries.map((delivery) => (
          <article
            key={delivery.id}
            className="rounded-lg border border-[var(--line)] p-4"
          >
            <div className="flex justify-between gap-2">
              <strong>{delivery.state.replaceAll("_", " ")}</strong>
              <Badge
                tone={
                  delivery.state === "operating" ||
                  delivery.state === "restored"
                    ? "success"
                    : delivery.state === "paused"
                      ? "danger"
                      : "warning"
                }
              >
                {delivery.state.replaceAll("_", " ")}
              </Badge>
            </div>
            <p className="mt-2 break-all font-mono text-xs text-[var(--muted)]">
              provider {delivery.provider_revision || "restricted"}
              <br />
              consumer {delivery.release_revision || "restricted"}
            </p>
            <p className="mt-3 text-sm">
              {[
                ...delivery.rollout,
                ...delivery.health,
                delivery.support_readiness,
                delivery.user_acceptance,
              ]
                .filter(Boolean)
                .join(" · ")}
            </p>
            <p className="mt-2 text-xs text-[var(--muted)]">
              {delivery.check_run_ids?.length ?? 0} checks ·{" "}
              {delivery.approval_ids?.length ?? 0} approvals ·{" "}
              {delivery.currency} {delivery.cost_cents} ·{" "}
              {delivery.authority.replaceAll("_", " ")}
            </p>
            {delivery.pause_reasons?.length > 0 && (
              <p className="mt-3 rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]">
                Paused safely: {delivery.pause_reasons.join(" · ")}
              </p>
            )}
          </article>
        ))}
      </div>
      <details className="mt-4 rounded-lg border border-[var(--line)] p-3">
        <summary className="cursor-pointer text-sm font-semibold">
          Bind ordinary delivery evidence
        </summary>
        <form onSubmit={submit} className="mt-4 grid gap-3 md:grid-cols-2">
          {[
            ["repository", "Consumer repository ID"],
            ["pull", "Merged pull request ID"],
            ["release", "Consumer release ID"],
            ["deployment", "Staged deployment ID"],
            ["restores", "Paused delivery restored (optional)"],
            ["cost", "Observed cost (minor units)"],
            ["currency", "Currency"],
          ].map(([name, label]) => (
            <label key={name} className="grid gap-1.5 text-sm font-medium">
              {label}
              <input
                name={name}
                required={name !== "restores"}
                type={name === "cost" ? "number" : "text"}
                defaultValue={
                  name === "cost"
                    ? "0"
                    : name === "currency"
                      ? workspace.currency
                      : undefined
                }
                className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal"
              />
            </label>
          ))}
          <label className="grid gap-1.5 text-sm font-medium">
            Adoption agreement
            <select
              name="plan"
              className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 font-normal"
            >
              {workspace.plans?.map((plan) => (
                <option key={plan.id} value={plan.id}>
                  {plan.selected_version}
                </option>
              ))}
            </select>
          </label>
          {[
            ["support", "Support readiness"],
            ["acceptance", "User acceptance"],
            ["attestations", "Human attestations"],
          ].map(([name, label]) => (
            <label key={name} className="grid gap-1.5 text-sm font-medium">
              {label}
              <textarea
                name={name}
                required
                rows={3}
                placeholder={
                  name === "attestations"
                    ? "policy | Policy passed | human ID | met\nrehearsal | Rollback passed | human ID | met\nsupport | On-call ready | human ID | met\nuser_acceptance | Users accepted | human ID | met\ncost | Within budget | human ID | met"
                    : undefined
                }
                className="rounded-lg border border-[var(--line-strong)] bg-white p-3 font-normal"
              />
            </label>
          ))}
          <div className="md:col-span-2">
            <Button type="submit" disabled={pending}>
              {pending
                ? "Resolving exact evidence…"
                : "Retain delivery evidence"}
            </Button>
          </div>
        </form>
      </details>
    </Card>
  );
}
