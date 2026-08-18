"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api } from "@/lib/api";
import { useAuth } from "./auth";
import { Badge, Button, Card } from "./ui";

type Definition = {
  name: string;
  description: string;
  usage: string;
  source_path: string;
  owner_ids: string[];
  constraints: { accessibility: string[]; localization: string[] };
  examples: {
    title: string;
    description: string;
    properties: Record<string, string>;
  }[];
};
type Revision = {
  version: number;
  title: string;
  summary: string;
  rationale: string;
  commit_id: string;
  release_id: string;
  release_version: string;
  owner_ids: string[];
  themes: string[];
  tokens: {
    name: string;
    category: string;
    value: string;
    theme: string;
    description: string;
    owner_ids: string[];
  }[];
  components: Definition[];
  interaction_patterns: Definition[];
  content_rules: Definition[];
  responsive_rules: {
    name: string;
    condition: string;
    behavior: string;
    owner_ids: string[];
  }[];
  adoption_policy: {
    level: string;
    supported_consumers: string[];
    exceptions: string[];
    migration_guidance: string;
  };
  implementations: {
    consumer: string;
    repository_id?: string;
    release_id?: string;
    commit_id: string;
    definition_name: string;
    status: string;
    notes?: string;
  }[];
  created_by: string;
  created_at: string;
};
type System = {
  id: string;
  current_version: number;
  revisions: Revision[];
  diagnostics: {
    kind: string;
    severity: string;
    message: string;
    definition?: string;
    consumer?: string;
  }[];
};
const text = (f: FormData, n: string) => String(f.get(n) ?? "").trim();
const list = (v: string) =>
  v
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
function Field({
  name,
  label,
  value = "",
  required = true,
}: {
  name: string;
  label: string;
  value?: string;
  required?: boolean;
}) {
  return (
    <label className="grid gap-1 text-xs font-semibold">
      {label}
      <input
        name={name}
        defaultValue={value}
        required={required}
        className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm font-normal outline-none focus:border-[var(--brand)]"
      />
    </label>
  );
}
function Area({
  name,
  label,
  value = "",
}: {
  name: string;
  label: string;
  value?: string;
}) {
  return (
    <label className="grid gap-1 text-xs font-semibold">
      {label}
      <textarea
        name={name}
        defaultValue={value}
        required
        rows={2}
        className="rounded-lg border border-[var(--line-strong)] bg-white px-3 py-2 text-sm font-normal outline-none focus:border-[var(--brand)]"
      />
    </label>
  );
}
function definition(
  f: FormData,
  prefix: string,
  current?: Definition,
): Definition {
  return {
    name: text(f, `${prefix}_name`),
    description: text(f, `${prefix}_description`),
    usage: text(f, `${prefix}_usage`),
    source_path: text(f, `${prefix}_path`),
    owner_ids: current?.owner_ids ?? list(text(f, "owners")),
    constraints: {
      accessibility: list(text(f, `${prefix}_a11y`)),
      localization: list(text(f, `${prefix}_l10n`)),
    },
    examples: [
      {
        title: text(f, `${prefix}_example`),
        description: text(f, `${prefix}_example_description`),
        properties: {
          ...(current?.examples[0]?.properties ?? {}),
          state: text(f, `${prefix}_state`),
        },
      },
      ...(current?.examples.slice(1) ?? []),
    ],
  };
}

export function InterfaceSystemWorkspace({
  repositoryID,
}: {
  repositoryID: string;
}) {
  const { token, user } = useAuth(),
    [systems, setSystems] = useState<System[]>([]),
    [selected, setSelected] = useState<System>(),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  const load = useCallback(async () => {
    try {
      const out = await api<{ interface_systems: System[] }>(
        `/repositories/${repositoryID}/interface-systems`,
        {},
        token,
      );
      setSystems(out.interface_systems);
      setSelected(
        (old) =>
          out.interface_systems.find((x) => x.id === old?.id) ??
          out.interface_systems[0],
      );
      setError("");
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "Interface systems could not be loaded.",
      );
    }
  }, [repositoryID, token]);
  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);
  const current = selected?.revisions.at(-1),
    component = current?.components[0],
    interaction = current?.interaction_patterns[0],
    content = current?.content_rules[0],
    responsive = current?.responsive_rules[0],
    implementation = current?.implementations[0],
    tokenItem = current?.tokens[0];
  async function publish(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const f = new FormData(event.currentTarget),
      owners = list(text(f, "owners"));
    const revision = {
      title: text(f, "title"),
      summary: text(f, "summary"),
      rationale: text(f, "rationale"),
      commit_id: text(f, "commit"),
      release_id: text(f, "release"),
      owner_ids: owners,
      themes: list(text(f, "themes")),
      tokens: [
        {
          name: text(f, "token_name"),
          category: text(f, "token_category"),
          value: text(f, "token_value"),
          theme: text(f, "token_theme"),
          description: text(f, "token_description"),
          owner_ids: owners,
        },
        ...(current?.tokens.slice(1) ?? []),
      ],
      components: [
        definition(f, "component", component),
        ...(current?.components.slice(1) ?? []),
      ],
      interaction_patterns: [
        definition(f, "interaction", interaction),
        ...(current?.interaction_patterns.slice(1) ?? []),
      ],
      content_rules: [
        definition(f, "content", content),
        ...(current?.content_rules.slice(1) ?? []),
      ],
      responsive_rules: [
        {
          name: text(f, "responsive_name"),
          condition: text(f, "responsive_condition"),
          behavior: text(f, "responsive_behavior"),
          owner_ids: owners,
        },
        ...(current?.responsive_rules.slice(1) ?? []),
      ],
      adoption_policy: {
        level: text(f, "policy"),
        supported_consumers: list(text(f, "consumers")),
        exceptions: list(text(f, "exceptions")),
        migration_guidance: text(f, "migration"),
      },
      implementations: [
        {
          consumer: text(f, "consumer"),
          repository_id: repositoryID,
          release_id: text(f, "release"),
          commit_id: text(f, "commit"),
          definition_name: text(f, "component_name"),
          status: text(f, "status"),
          notes: text(f, "implementation_notes"),
        },
        ...(current?.implementations.slice(1) ?? []),
      ],
    };
    try {
      await api(
        selected
          ? `/repositories/${repositoryID}/interface-systems/${selected.id}/revisions`
          : `/repositories/${repositoryID}/interface-systems`,
        {
          method: "POST",
          body: JSON.stringify({
            expected_version: selected?.current_version ?? 0,
            revision,
          }),
        },
        token,
      );
      await load();
    } catch (e) {
      setError(
        e instanceof Error
          ? e.message
          : "The interface system could not be published.",
      );
    } finally {
      setBusy(false);
    }
  }
  const defs = [
    ...[...(current?.components ?? [])].map((x) => ["Component", x] as const),
    ...[...(current?.interaction_patterns ?? [])].map(
      (x) => ["Interaction", x] as const,
    ),
    ...[...(current?.content_rules ?? [])].map((x) => ["Content", x] as const),
  ];
  const definitionForms: [string, Definition | undefined][] = [
    ["component", component],
    ["interaction", interaction],
    ["content", content],
  ];
  return (
    <div className="space-y-6">
      <header>
        <div className="flex flex-wrap items-center gap-2">
          <Badge tone="info">Interface system</Badge>
          {current && <Badge>v{current.version}</Badge>}
        </div>
        <h1 className="mt-3 text-2xl font-semibold">Shared product language</h1>
        <p className="mt-2 max-w-3xl text-sm leading-6 text-[var(--muted)]">
          Discover the reviewed visual, interaction, content, responsive,
          accessibility, and localization decisions that make this product
          coherent.
        </p>
      </header>
      {error && (
        <p
          role="alert"
          className="rounded-lg bg-[var(--danger-soft)] p-3 text-sm text-[var(--danger)]"
        >
          {error}
        </p>
      )}
      {systems.length > 1 && (
        <nav aria-label="Interface systems" className="flex flex-wrap gap-2">
          {systems.map((x) => (
            <Button
              key={x.id}
              variant={x.id === selected?.id ? "primary" : "secondary"}
              onClick={() => setSelected(x)}
            >
              {x.revisions.at(-1)?.title}
            </Button>
          ))}
        </nav>
      )}
      {current && (
        <>
          <Card className="p-5">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold">{current.title}</h2>
                <p className="mt-1 text-sm text-[var(--muted)]">
                  {current.summary}
                </p>
              </div>
              <Badge
                tone={
                  current.adoption_policy.level === "required"
                    ? "warning"
                    : "success"
                }
              >
                {current.adoption_policy.level}
              </Badge>
            </div>
            <p className="mt-4 text-sm">
              <b>Why:</b> {current.rationale}
            </p>
            <p className="mt-3 text-xs text-[var(--muted)]">
              Release {current.release_version} ·{" "}
              <code>{current.commit_id.slice(0, 12)}</code> · published by{" "}
              {current.created_by} on{" "}
              {new Date(current.created_at).toLocaleDateString()}
            </p>
          </Card>
          {selected!.diagnostics.length > 0 && (
            <Card className="border-[var(--warning)] p-5">
              <h2 className="font-semibold">Explicit gaps and conflicts</h2>
              <div className="mt-3 grid gap-2">
                {selected!.diagnostics.map((x, i) => (
                  <div
                    key={`${x.kind}-${i}`}
                    className="flex items-start gap-2 text-sm"
                  >
                    <Badge
                      tone={x.severity === "blocking" ? "danger" : "warning"}
                    >
                      {x.kind.replaceAll("_", " ")}
                    </Badge>
                    <span>
                      {x.message}{" "}
                      {[x.definition, x.consumer].filter(Boolean).join(" · ")}
                    </span>
                  </div>
                ))}
              </div>
            </Card>
          )}
          <section className="grid gap-4 lg:grid-cols-2">
            <Card className="p-5">
              <h2 className="font-semibold">Tokens and themes</h2>
              <p className="mt-1 text-xs text-[var(--muted)]">
                Supported: {current.themes.join(" · ")}
              </p>
              <div className="mt-4 grid gap-3">
                {current.tokens.map((x) => (
                  <article
                    key={`${x.name}-${x.theme}`}
                    className="flex items-center gap-3 rounded-lg bg-[var(--surface-soft)] p-3"
                  >
                    <span
                      aria-hidden
                      className="size-10 rounded-lg border border-[var(--line)]"
                      style={{ backgroundColor: x.value }}
                    />
                    <div>
                      <code className="text-xs font-semibold">{x.name}</code>
                      <p className="text-sm">{x.description}</p>
                      <p className="text-xs text-[var(--muted)]">
                        {x.theme} · {x.category} · {x.value}
                      </p>
                    </div>
                  </article>
                ))}
              </div>
            </Card>
            <Card className="p-5">
              <h2 className="font-semibold">Responsive behavior</h2>
              {current.responsive_rules.map((x) => (
                <article
                  key={x.name}
                  className="mt-3 rounded-lg bg-[var(--surface-soft)] p-3"
                >
                  <b className="text-sm">{x.name}</b>
                  <p className="mt-1 text-sm">{x.behavior}</p>
                  <code className="mt-2 block text-xs text-[var(--muted)]">
                    {x.condition}
                  </code>
                </article>
              ))}
            </Card>
          </section>
          <section>
            <h2 className="text-lg font-semibold">
              Rendered examples and usage
            </h2>
            <div className="mt-3 grid gap-4 lg:grid-cols-3">
              {defs.map(([kind, x]) => (
                <Card className="overflow-hidden" key={`${kind}-${x.name}`}>
                  <div className="border-b border-[var(--line)] bg-[var(--surface-soft)] p-5">
                    <Badge>{kind}</Badge>
                    <div className="mt-4 rounded-xl border border-[var(--line)] bg-white p-5 text-center shadow-sm">
                      <b>{x.examples[0]?.title}</b>
                      <p className="mt-1 text-sm text-[var(--muted)]">
                        {x.examples[0]?.description}
                      </p>
                    </div>
                  </div>
                  <div className="p-5">
                    <h3 className="font-semibold">{x.name}</h3>
                    <p className="mt-2 text-sm">{x.description}</p>
                    <p className="mt-3 text-sm">
                      <b>Use:</b> {x.usage}
                    </p>
                    <p className="mt-3 text-xs">
                      <b>Accessibility:</b>{" "}
                      {x.constraints.accessibility.join(" · ") ||
                        "Not declared"}
                    </p>
                    <p className="mt-1 text-xs">
                      <b>Localization:</b>{" "}
                      {x.constraints.localization.join(" · ") || "Not declared"}
                    </p>
                    <code className="mt-3 block text-xs text-[var(--muted)]">
                      {x.source_path}
                    </code>
                  </div>
                </Card>
              ))}
            </div>
          </section>
          <Card className="p-5">
            <h2 className="font-semibold">Adoption and implementation</h2>
            <p className="mt-2 text-sm">
              {current.adoption_policy.migration_guidance}
            </p>
            <p className="mt-2 text-xs text-[var(--muted)]">
              Supported consumers:{" "}
              {current.adoption_policy.supported_consumers.join(" · ")}
            </p>
            {current.implementations.map((x, i) => (
              <div
                key={`${x.consumer}-${i}`}
                className="mt-3 flex flex-wrap items-center gap-2 rounded-lg bg-[var(--surface-soft)] p-3 text-sm"
              >
                <Badge
                  tone={
                    x.status === "current"
                      ? "success"
                      : x.status === "stale"
                        ? "warning"
                        : "danger"
                  }
                >
                  {x.status}
                </Badge>
                <b>{x.consumer}</b>
                <span>{x.definition_name}</span>
                <code className="text-xs">{x.commit_id.slice(0, 12)}</code>
              </div>
            ))}
          </Card>
          <details>
            <summary className="cursor-pointer text-sm font-semibold">
              Revision history ({selected!.revisions.length})
            </summary>
            <ol className="mt-3 space-y-2">
              {selected!.revisions.toReversed().map((x) => (
                <li key={x.version} className="text-sm">
                  <b>v{x.version}</b> · {x.release_version} ·{" "}
                  {new Date(x.created_at).toLocaleString()} · {x.rationale}
                </li>
              ))}
            </ol>
          </details>
        </>
      )}
      {token && (
        <Card className="p-5">
          <h2 className="font-semibold">
            {selected
              ? "Publish a successor revision"
              : "Define an interface system"}
          </h2>
          <form onSubmit={publish} className="mt-4 grid gap-4 lg:grid-cols-3">
            <Field name="title" label="System title" value={current?.title} />
            <Field
              name="owners"
              label="Owner IDs (comma separated)"
              value={current?.owner_ids.join(",") ?? user?.id}
            />
            <Field
              name="themes"
              label="Supported themes"
              value={current?.themes.join(",") ?? "light,dark"}
            />
            <Field
              name="release"
              label="Exact release ID"
              value={current?.release_id}
            />
            <Field
              name="commit"
              label="Exact 40-character release commit"
              value={current?.commit_id}
            />
            <label className="grid gap-1 text-xs font-semibold">
              Adoption level
              <select
                name="policy"
                defaultValue={current?.adoption_policy.level ?? "recommended"}
                className="min-h-10 rounded-lg border border-[var(--line-strong)] bg-white px-3 text-sm font-normal"
              >
                <option>experimental</option>
                <option>recommended</option>
                <option>required</option>
              </select>
            </label>
            <Area name="summary" label="Summary" value={current?.summary} />
            <Area
              name="rationale"
              label="Rationale"
              value={current?.rationale}
            />
            <Area
              name="migration"
              label="Adoption and migration guidance"
              value={current?.adoption_policy.migration_guidance}
            />
            <Field
              name="consumers"
              label="Supported consumers"
              value={current?.adoption_policy.supported_consumers.join(",")}
            />
            <Field
              name="exceptions"
              label="Adoption exceptions"
              value={current?.adoption_policy.exceptions.join(",")}
            />
            <Field
              name="consumer"
              label="Observed consumer"
              value={implementation?.consumer}
            />
            <Field
              name="status"
              label="Implementation status"
              value={implementation?.status ?? "current"}
            />
            <Field
              name="implementation_notes"
              label="Implementation evidence"
              value={implementation?.notes}
            />
            <Field
              name="token_name"
              label="Token name"
              value={tokenItem?.name ?? "color.action.primary"}
            />
            <Field
              name="token_category"
              label="Token category"
              value={tokenItem?.category ?? "color"}
            />
            <Field
              name="token_value"
              label="Token value"
              value={tokenItem?.value ?? "#176b4d"}
            />
            <Field
              name="token_theme"
              label="Token theme"
              value={tokenItem?.theme ?? "light"}
            />
            <Field
              name="token_description"
              label="Token purpose"
              value={tokenItem?.description ?? "Primary action emphasis"}
            />
            {definitionForms.map(([prefix, x]) => (
              <fieldset
                key={prefix}
                className="grid gap-3 rounded-lg border border-[var(--line)] p-3 lg:col-span-3 lg:grid-cols-3"
              >
                <legend className="px-2 text-sm font-semibold capitalize">
                  {prefix} definition
                </legend>
                <Field name={`${prefix}_name`} label="Name" value={x?.name} />
                <Field
                  name={`${prefix}_path`}
                  label="Reviewed source path"
                  value={x?.source_path}
                />
                <Field
                  name={`${prefix}_usage`}
                  label="When to use"
                  value={x?.usage}
                />
                <Field
                  name={`${prefix}_description`}
                  label="Decision"
                  value={x?.description}
                />
                <Field
                  name={`${prefix}_a11y`}
                  label="Accessibility constraints"
                  value={x?.constraints.accessibility.join(",")}
                />
                <Field
                  name={`${prefix}_l10n`}
                  label="Localization constraints"
                  value={x?.constraints.localization.join(",")}
                />
                <Field
                  name={`${prefix}_example`}
                  label="Example title"
                  value={x?.examples[0]?.title}
                />
                <Field
                  name={`${prefix}_example_description`}
                  label="Rendered example description"
                  value={x?.examples[0]?.description}
                />
                <Field
                  name={`${prefix}_state`}
                  label="Example state"
                  value={x?.examples[0]?.properties.state ?? "default"}
                />
              </fieldset>
            ))}
            <Field
              name="responsive_name"
              label="Responsive rule"
              value={responsive?.name ?? "Compact navigation"}
            />
            <Field
              name="responsive_condition"
              label="Viewport/content condition"
              value={responsive?.condition ?? "width < 48rem"}
            />
            <Field
              name="responsive_behavior"
              label="Required behavior"
              value={
                responsive?.behavior ??
                "Collapse labels without hiding primary actions."
              }
            />
            <div className="lg:col-span-3">
              <Button disabled={busy}>
                {busy
                  ? "Publishing…"
                  : selected
                    ? "Publish successor"
                    : "Publish system"}
              </Button>
            </div>
          </form>
        </Card>
      )}
    </div>
  );
}
