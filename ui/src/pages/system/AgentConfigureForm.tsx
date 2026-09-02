import { useState } from "react";
import { Input, Space, Typography } from "antd";

import {
  useCreateAgent,
  useDeleteAgent,
  useUpdateAgent,
} from "../../api/queries";
import type { Agent } from "../../gen/metarr/v1/agents_pb";
import { Button } from "../../components/Card";
import "./AgentConfigureForm.css";

/*
 * Configuring an agent is one question asked once per library: what does this
 * machine call it?
 *
 * Every configured scan directory is listed, blank by default. A blank row is a
 * deliberate, meaningful answer — this agent cannot reach that library — rather
 * than an unfinished one, which is why nothing here requires filling them all
 * in.
 */
export function AgentConfigureForm({
  agent,
  scanDirectories,
  onDone,
}: {
  agent: Agent;
  scanDirectories: { scannerSlug: string; directory: string }[];
  onDone: () => void;
}) {
  const create = useCreateAgent();
  const update = useUpdateAgent();
  const remove = useDeleteAgent();

  const [displayName, setDisplayName] = useState(agent.displayName);
  const [paths, setPaths] = useState<Record<string, string>>(() =>
    Object.fromEntries(
      agent.mappings.map((mapping) => [mapping.scannerSlug, mapping.agentPath]),
    ),
  );
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  async function save() {
    setSaving(true);
    setError(null);
    try {
      // A configured agent is edited through Update; one that has only
      // announced itself is set up through Create.
      const write = agent.configured ? update : create;
      await write.mutateAsync({
        slug: agent.slug,
        displayName: displayName.trim(),
        mappings: Object.entries(paths)
          .filter(([, path]) => path.trim() !== "")
          .map(([scannerSlug, path]) => ({
            scannerSlug,
            agentPath: path.trim(),
          })),
      });
      onDone();
    } catch (cause) {
      // The server explains rejections properly — an already-claimed library
      // names the agent holding it — so its message is shown verbatim.
      setError(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setSaving(false);
    }
  }

  async function forget() {
    if (
      !window.confirm(
        `Remove the configuration for "${agent.slug}"? The agent keeps running and will reappear here as unconfigured.`,
      )
    ) {
      return;
    }
    setSaving(true);
    try {
      await remove.mutateAsync(agent.slug);
      onDone();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
      setSaving(false);
    }
  }

  return (
    <div className="agent-configure-form">
      <div className="agent-configure-name">
        <label
          className="agent-configure-name-label"
          htmlFor={`name-${agent.slug}`}
        >
          Display name
        </label>
        <Input
          id={`name-${agent.slug}`}
          value={displayName}
          placeholder={agent.slug}
          onChange={(event) => setDisplayName(event.target.value)}
        />
      </div>

      <div>
        <Typography.Text
          type="secondary"
          className="agent-configure-section-heading"
        >
          Libraries
        </Typography.Text>
        <Typography.Text
          type="secondary"
          className="agent-configure-section-hint"
        >
          Enter the path each library has on this agent&apos;s machine. Leave
          one blank if the agent cannot reach it.
        </Typography.Text>

        {scanDirectories.length === 0 ? (
          <Typography.Text type="secondary" italic style={{ fontSize: 14 }}>
            No scan directories configured yet — add one under Directory Scanner
            first.
          </Typography.Text>
        ) : (
          <Space direction="vertical" size={8} style={{ width: "100%" }}>
            {scanDirectories.map((directory) => (
              <div key={directory.scannerSlug} className="agent-configure-row">
                <div className="agent-configure-row-label">
                  <div className="agent-configure-row-slug">
                    {directory.scannerSlug}
                  </div>
                  <div
                    className="agent-configure-row-path"
                    title={directory.directory}
                  >
                    {directory.directory}
                  </div>
                </div>
                <Typography.Text type="secondary" aria-hidden="true">
                  →
                </Typography.Text>
                <Input
                  value={paths[directory.scannerSlug] ?? ""}
                  placeholder="not reachable from this agent"
                  aria-label={`Path for ${directory.scannerSlug} on ${agent.slug}`}
                  className="editable-field-mono agent-configure-row-input"
                  onChange={(event) =>
                    setPaths((current) => ({
                      ...current,
                      [directory.scannerSlug]: event.target.value,
                    }))
                  }
                />
              </div>
            ))}
          </Space>
        )}
      </div>

      {error ? (
        <Typography.Text type="danger" style={{ fontSize: 12 }}>
          {error}
        </Typography.Text>
      ) : null}

      <Space wrap>
        <Button variant="primary" disabled={saving} onClick={() => void save()}>
          {saving ? "Saving…" : "Save"}
        </Button>
        <Button variant="ghost" onClick={onDone}>
          Cancel
        </Button>
        {agent.configured ? (
          <Button
            variant="danger"
            disabled={saving}
            onClick={() => void forget()}
          >
            Remove agent
          </Button>
        ) : null}
      </Space>
    </div>
  );
}
