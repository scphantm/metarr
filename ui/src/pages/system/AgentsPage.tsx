import { useState } from "react";
import { Alert, Badge, Progress, Space, Typography } from "antd";

import { timestampDate } from "@bufbuild/protobuf/wkt";

import {
  useAgents,
  useAgentsPresenceStreamStatus,
  useScanDirectories,
} from "../../api/queries";
import type { AgentTelemetry } from "../../gen/metarr/bus/v1/agent_contract_pb";
import type { Agent } from "../../gen/metarr/v1/agents_pb";
import { Button, Card, EmptyState } from "../../components/Card";
import { PageError, PageLoading } from "../../components/PageState";
import { PageHeader } from "../../layout/AppShell";
import { AgentConfigureForm } from "./AgentConfigureForm";
import "./AgentsPage.css";

/*
 * System > Agents.
 *
 * An agent announces itself simply by connecting to Redis, so this screen shows
 * the union of two different things: agents someone has configured, and agents
 * that are currently there. Either can exist without the other, and the
 * difference matters — a configured agent that has gone quiet is a machine to go
 * and check on, while an unconfigured one that has appeared is a machine waiting
 * to be set up.
 */
export function AgentsPage() {
  const agents = useAgents();
  const directories = useScanDirectories();
  const socketStatus = useAgentsPresenceStreamStatus();

  const [configuring, setConfiguring] = useState<string | null>(null);

  if (agents.error && !agents.data) {
    return (
      <>
        <PageHeader title="Agents" />
        <PageError error={agents.error} />
      </>
    );
  }

  if (!agents.data) {
    return (
      <>
        <PageHeader title="Agents" />
        <PageLoading>Looking for agents…</PageLoading>
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Agents"
        description="Agents run next to your media and do every filesystem operation. They connect to Redis with nothing but a name; everything else is configured here."
        actions={<ConnectionIndicator status={socketStatus} />}
      />

      <div className="page-body">
        {agents.data.length === 0 ? (
          <EmptyState>
            No agents yet. Start a metarr-agent pointed at this Redis and it
            will appear here within a few seconds.
          </EmptyState>
        ) : null}

        {agents.data.map((agent) => (
          <AgentCard
            key={agent.slug}
            agent={agent}
            scanDirectories={directories.data ?? []}
            configuring={configuring === agent.slug}
            onConfigure={() => setConfiguring(agent.slug)}
            onClose={() => setConfiguring(null)}
          />
        ))}
      </div>
    </>
  );
}

function ConnectionIndicator({ status }: { status: string }) {
  const label =
    status === "open"
      ? "Live"
      : status === "connecting"
        ? "Connecting"
        : "Stale";
  const badgeStatus =
    status === "open"
      ? "success"
      : status === "connecting"
        ? "processing"
        : "warning";

  return <Badge status={badgeStatus} text={label} />;
}

function AgentCard({
  agent,
  scanDirectories,
  configuring,
  onConfigure,
  onClose,
}: {
  agent: Agent;
  scanDirectories: { scannerSlug: string; directory: string }[];
  configuring: boolean;
  onConfigure: () => void;
  onClose: () => void;
}) {
  const needsSetup = agent.online && !agent.configured;

  return (
    <Card
      title={agent.displayName || agent.slug}
      description={agent.displayName ? agent.slug : undefined}
      actions={
        <Space align="center">
          <AgentStatus agent={agent} />
          {configuring ? null : (
            <Button
              variant={needsSetup ? "primary" : "default"}
              onClick={onConfigure}
            >
              {agent.configured ? "Edit" : "Configure this agent"}
            </Button>
          )}
        </Space>
      }
    >
      {needsSetup ? (
        <Alert
          type="info"
          showIcon
          className="agent-setup-notice"
          message="This agent has connected but has not been set up yet. Map the libraries it can reach to start using it."
        />
      ) : null}

      {agent.identity ? <AgentIdentityGrid agent={agent} /> : null}

      {agent.telemetry ? <TelemetryMeters telemetry={agent.telemetry} /> : null}

      {configuring ? (
        <AgentConfigureForm
          agent={agent}
          scanDirectories={scanDirectories}
          onDone={onClose}
        />
      ) : (
        <MappingList agent={agent} scanDirectories={scanDirectories} />
      )}
    </Card>
  );
}

// Status is a word first and a colour second: an operator scanning a column of
// cards reads the word, and colour alone would carry the whole meaning for
// nobody who cannot separate the hues.
function AgentStatus({ agent }: { agent: Agent }) {
  if (!agent.online) {
    return <Badge status="default" text="Offline" />;
  }
  if (!agent.configured) {
    return <Badge status="warning" text="Needs setup" />;
  }
  return <Badge status="success" text="Online" />;
}

function AgentIdentityGrid({ agent }: { agent: Agent }) {
  const identity = agent.identity;
  if (!identity) return null;

  const facts: [string, string][] = [
    ["Host", identity.hostname || "—"],
    ["Address", identity.ip || "—"],
    ["Running as", `${identity.username || "unknown"} (uid ${identity.uid})`],
    ["Platform", `${identity.os}/${identity.arch}`],
    ["Version", identity.version],
    [
      "Up since",
      identity.started ? timestampDate(identity.started).toLocaleString() : "—",
    ],
  ];

  return (
    <div className="agent-identity-grid">
      {facts.map(([label, value]) => (
        <div key={label}>
          <div className="agent-identity-label">{label}</div>
          <div className="agent-identity-value" title={value}>
            {value}
          </div>
        </div>
      ))}
    </div>
  );
}

// CPU and memory are each one value against a known limit, so they are meters —
// a filled track against the same-hue background. A chart of two numbers would
// say less than the numbers do.
function TelemetryMeters({ telemetry }: { telemetry: AgentTelemetry }) {
  const memoryUsed = Number(telemetry.memoryUsedBytes);
  const memoryTotal = Number(telemetry.memoryTotalBytes);
  const memoryPercent = memoryTotal ? (memoryUsed / memoryTotal) * 100 : 0;

  return (
    <Space direction="vertical" size={12} className="agent-telemetry-meters">
      <Meter
        label="CPU"
        percent={telemetry.cpuPercent}
        detail={`${telemetry.cpuPercent.toFixed(1)}%`}
      />
      <Meter
        label="Memory"
        percent={memoryPercent}
        detail={`${formatBytes(memoryUsed)} of ${formatBytes(memoryTotal)}`}
      />
      {telemetry.gpus.map((gpu, index) => (
        <Meter
          // Fixed hardware list, never reordered; identical GPUs share a name
          // so the index disambiguates.
          // eslint-disable-next-line @eslint-react/no-array-index-key
          key={`${gpu.name}-${index}`}
          label={gpu.name || "GPU"}
          percent={gpu.utilizationPercent}
          detail={`${gpu.utilizationPercent.toFixed(0)}% · ${formatBytes(
            Number(gpu.memoryUsedBytes),
          )} of ${formatBytes(Number(gpu.memoryTotalBytes))}`}
        />
      ))}
    </Space>
  );
}

function Meter({
  label,
  percent,
  detail,
}: {
  label: string;
  percent: number;
  detail: string;
}) {
  const clamped = Math.max(0, Math.min(100, percent));

  return (
    <div>
      <div className="agent-meter-header">
        <Typography.Text style={{ fontSize: 12 }}>{label}</Typography.Text>
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {detail}
        </Typography.Text>
      </div>
      <Progress
        percent={clamped}
        showInfo={false}
        size="small"
        aria-label={label}
      />
    </div>
  );
}

function MappingList({
  agent,
  scanDirectories,
}: {
  agent: Agent;
  scanDirectories: { scannerSlug: string; directory: string }[];
}) {
  // The server sends raw mappings (scanner_slug + agent_path); the library's
  // path on this server is joined in here from the scan directories the page
  // already loaded.
  const serverPathFor = (scannerSlug: string) =>
    scanDirectories.find((directory) => directory.scannerSlug === scannerSlug)
      ?.directory ?? "";

  if (agent.mappings.length === 0) {
    return (
      <Typography.Text type="secondary" style={{ fontSize: 14 }}>
        No libraries mapped, so this agent has nothing to scan.
      </Typography.Text>
    );
  }

  return (
    <Space direction="vertical" size={4} style={{ width: "100%" }}>
      <Typography.Text type="secondary" className="agent-mapping-heading">
        Mapped libraries
      </Typography.Text>
      {agent.mappings.map((mapping) => (
        <div key={mapping.scannerSlug} className="agent-mapping-row">
          <span className="agent-mapping-slug">{mapping.scannerSlug}</span>
          <span className="agent-mapping-path">
            {serverPathFor(mapping.scannerSlug) || "—"}
          </span>
          <Typography.Text type="secondary" aria-hidden="true">
            →
          </Typography.Text>
          <span className="agent-mapping-path is-local">
            {mapping.agentPath}
          </span>
        </div>
      ))}
    </Space>
  );
}

function formatBytes(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const exponent = Math.min(
    units.length - 1,
    Math.floor(Math.log(bytes) / Math.log(1024)),
  );
  const value = bytes / 1024 ** exponent;
  return `${value.toFixed(value < 10 && exponent > 0 ? 1 : 0)} ${units[exponent]}`;
}

export function AgentsSidebar() {
  return (
    <div className="saving-info-sidebar">
      <Alert
        type="info"
        message="How agents work"
        description={
          <>
            <p>
              An agent is configured locally with only two things: how to reach
              Redis, and its own name. Everything else — which libraries it can
              see and where they live on its machine — is published to it from
              here.
            </p>
            <p>
              It never connects to the database. Scan results travel back over
              the event bus and are stored under this server&apos;s paths, so
              the library reads the same however many agents produced it.
            </p>
          </>
        }
      />

      <Alert
        type="info"
        message="Mapping libraries"
        description={
          <>
            <p>
              A mapping says what this machine calls a library you have
              configured under Directory Scanner. Leave one blank when the agent
              cannot reach it — agents sit on different machines and are not
              expected to see everything.
            </p>
            <p>
              Each library belongs to one agent. Two agents scanning the same
              files would each overwrite the other&apos;s records.
            </p>
          </>
        }
      />
    </div>
  );
}
