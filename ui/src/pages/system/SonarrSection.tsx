import { useState } from "react";
import type { MessageInitShape } from "@bufbuild/protobuf";
import { Input, Space, Typography } from "antd";

import {
  queryKeys,
  useDeleteSonarrInstance,
  useUpsertSonarrInstance,
} from "../../api/queries";
import { storageModes } from "../../api/vocab";
import {
  SonarrInstanceSchema,
  type SonarrInstance,
} from "../../gen/metarr/v1/sonarr_interfaces_pb";
import { Button, Card, EmptyState, Row } from "../../components/Card";
import {
  EditableNumber,
  EditableSelect,
  EditableText,
} from "../../components/Editable";
import "./SonarrSection.css";

// onSave/onCreate below hand off to useUpsertSonarrInstance, whose
// parameter is MessageInitShape<typeof SonarrInstanceSchema> (a plain
// object, not a branded Message) — but every *read* here always comes back
// as a real SonarrInstance from the List RPC, and typing props/state as the
// branded message keeps each nested field a single concrete type rather
// than the (Message | plain-init-shape) union MessageInitShape produces,
// which otherwise stops TS from simplifying `{...instance, field}` spreads.
type SonarrInstanceInit = MessageInitShape<typeof SonarrInstanceSchema>;

/*
 * Sonarr instances. Like scan directories these are keyed by a slug the upsert
 * matches on, so the slug is fixed once created.
 *
 * Root directory mappings translate a path as Sonarr reports it into one on
 * this machine — the pair only means something together, so they are edited as
 * rows rather than as two independent lists.
 */
export function SonarrSection({ instances }: { instances: SonarrInstance[] }) {
  const upsert = useUpsertSonarrInstance();
  const remove = useDeleteSonarrInstance();
  const [adding, setAdding] = useState(false);

  return (
    <Card
      title="Sonarr interfaces"
      description="Sonarr instances Metarr caches series data from."
      actions={
        <Button variant="primary" onClick={() => setAdding(true)}>
          Add instance
        </Button>
      }
    >
      {instances.length === 0 && !adding ? (
        <EmptyState>No Sonarr instances configured</EmptyState>
      ) : null}

      <Space direction="vertical" size={16} style={{ width: "100%" }}>
        {instances.map((instance) => (
          <InstanceCard
            key={instance.instanceSlug}
            instance={instance}
            onSave={(next) => upsert.mutateAsync(next)}
            onRemove={() => {
              if (
                window.confirm(
                  `Remove the Sonarr instance "${instance.instanceName || instance.instanceSlug}"?`,
                )
              ) {
                void remove.mutateAsync(instance.instanceSlug);
              }
            }}
          />
        ))}
      </Space>

      {adding ? (
        <NewInstance
          existingSlugs={instances.map((entry) => entry.instanceSlug)}
          onCancel={() => setAdding(false)}
          onCreate={async (entry) => {
            await upsert.mutateAsync(entry);
            setAdding(false);
          }}
        />
      ) : null}
    </Card>
  );
}

function InstanceCard({
  instance,
  onSave,
  onRemove,
}: {
  instance: SonarrInstance;
  onSave: (next: SonarrInstanceInit) => Promise<unknown>;
  onRemove: () => void;
}) {
  const key = queryKeys.sonarr;

  // A plain object built field-by-field, never {...instance, field}: once a
  // spread carries instance's own literal $typeName, TS resolves
  // MessageInit<SonarrInstance>'s union to the strict branded branch even
  // for THIS object, which then demands nested fields (storage, rootDirMap)
  // be full branded messages too, not plain field bags. Omitting $typeName
  // entirely keeps the whole object in the loose branch, where nested
  // fields stay plain. Not a runtime concern either way — create() accepts
  // a plain field-only object fine.
  function nextInstance(
    fields: Partial<{
      instanceName: string;
      sonarrUrl: string;
      sonarrApiKey: string;
      storage: { mode: string; ttl: string; maxCount: number };
      rootDirMap: { sonarrPath: string; localPath: string }[];
    }>,
  ): SonarrInstanceInit {
    return {
      instanceSlug: instance.instanceSlug,
      instanceName: instance.instanceName ?? "",
      sonarrUrl: instance.sonarrUrl ?? "",
      sonarrApiKey: instance.sonarrApiKey ?? "",
      rootDirMap: instance.rootDirMap ?? [],
      storage: {
        mode: instance.storage?.mode ?? "cache",
        ttl: instance.storage?.ttl ?? "",
        maxCount: instance.storage?.maxCount ?? 0,
      },
      ...fields,
    };
  }

  return (
    <div className="sonarr-instance-card">
      <div className="sonarr-instance-card-header">
        <Typography.Text className="sonarr-instance-slug">
          {instance.instanceSlug}
        </Typography.Text>
        <Button variant="danger" onClick={onRemove}>
          Remove
        </Button>
      </div>

      <Row label="Name">
        <EditableText
          label="Instance name"
          queryKey={key}
          value={instance.instanceName ?? ""}
          placeholder="Unnamed instance"
          onSave={(instanceName) => onSave(nextInstance({ instanceName }))}
        />
      </Row>

      <Row label="URL">
        <EditableText
          label="Sonarr URL"
          queryKey={key}
          value={instance.sonarrUrl ?? ""}
          monospace
          placeholder="http://localhost:8989"
          validate={(next) =>
            next.startsWith("http://") || next.startsWith("https://")
              ? null
              : "Must start with http:// or https://"
          }
          onSave={(sonarrUrl) => onSave(nextInstance({ sonarrUrl }))}
        />
      </Row>

      <Row label="API key">
        <EditableText
          label="Sonarr API key"
          queryKey={key}
          value={instance.sonarrApiKey ?? ""}
          monospace
          secret
          placeholder="No key set"
          onSave={(sonarrApiKey) => onSave(nextInstance({ sonarrApiKey }))}
        />
      </Row>

      <Row
        label="Storage mode"
        hint="cache expires after a TTL; versioned keeps revisions"
      >
        <EditableSelect
          label="Storage mode"
          queryKey={key}
          value={instance.storage?.mode ?? "cache"}
          options={storageModes}
          onSave={(mode) =>
            onSave(
              nextInstance({
                storage: {
                  mode,
                  ttl: instance.storage?.ttl ?? "",
                  maxCount: instance.storage?.maxCount ?? 0,
                },
              }),
            )
          }
        />
      </Row>

      {/* Only the field belonging to the active mode is meaningful, so only
          that one is offered — showing both invites setting a value that is
          silently ignored. */}
      {instance.storage?.mode === "versioned" ? (
        <Row label="Max revisions">
          <EditableNumber
            label="Max count"
            queryKey={key}
            value={instance.storage?.maxCount ?? 0}
            min={1}
            onSave={(maxCount) =>
              onSave(
                nextInstance({
                  storage: {
                    mode: instance.storage?.mode ?? "cache",
                    ttl: instance.storage?.ttl ?? "",
                    maxCount,
                  },
                }),
              )
            }
          />
        </Row>
      ) : (
        // The server stores this string without parsing it today, so the editor
        // does not enforce a format — rejecting a value the API accepts would
        // make an existing entry uneditable.
        <Row label="TTL" hint="How long cached data lives, e.g. 24h or 90m">
          <EditableText
            label="TTL"
            queryKey={key}
            value={instance.storage?.ttl ?? ""}
            monospace
            placeholder="24h"
            onSave={(ttl) =>
              onSave(
                nextInstance({
                  storage: {
                    mode: instance.storage?.mode ?? "cache",
                    ttl,
                    maxCount: instance.storage?.maxCount ?? 0,
                  },
                }),
              )
            }
          />
        </Row>
      )}

      <Row
        label="Root directory map"
        hint="Sonarr's path on the left, this machine's on the right"
      >
        <RootDirMap instance={instance} onSave={onSave} />
      </Row>
    </div>
  );
}

function RootDirMap({
  instance,
  onSave,
}: {
  instance: SonarrInstance;
  onSave: (next: SonarrInstanceInit) => Promise<unknown>;
}) {
  const mappings = instance.rootDirMap ?? [];
  const [adding, setAdding] = useState(false);
  const [sonarrPath, setSonarrPath] = useState("");
  const [localPath, setLocalPath] = useState("");

  // Built field-by-field rather than {...instance, rootDirMap} — see
  // InstanceCard's nextInstance for why spreading the branded instance
  // defeats the loose-init-shape typing for nested fields.
  function write(
    rootDirMap: { sonarrPath: string; localPath: string }[],
  ): Promise<unknown> {
    return onSave({
      instanceSlug: instance.instanceSlug,
      instanceName: instance.instanceName ?? "",
      sonarrUrl: instance.sonarrUrl ?? "",
      sonarrApiKey: instance.sonarrApiKey ?? "",
      storage: {
        mode: instance.storage?.mode ?? "cache",
        ttl: instance.storage?.ttl ?? "",
        maxCount: instance.storage?.maxCount ?? 0,
      },
      rootDirMap,
    });
  }

  return (
    <Space direction="vertical" size={8} style={{ width: "100%" }}>
      {mappings.length === 0 && !adding ? (
        <Typography.Text type="secondary" italic style={{ fontSize: 14 }}>
          No mappings
        </Typography.Text>
      ) : null}

      {mappings.map((mapping, index) => (
        // Mapping rows have no id and a row's paths can be blank mid-add, so
        // index is the only stable key here.
        // eslint-disable-next-line @eslint-react/no-array-index-key
        <div key={index} className="sonarr-root-dir-row">
          <div className="sonarr-root-dir-field">
            <EditableText
              label="Sonarr path"
              queryKey={queryKeys.sonarr}
              value={mapping.sonarrPath ?? ""}
              monospace
              onSave={(sonarrPath) => {
                const next = [...mappings];
                next[index] = { ...mapping, sonarrPath };
                return write(next);
              }}
            />
          </div>
          <Typography.Text type="secondary" aria-hidden="true">
            →
          </Typography.Text>
          <div className="sonarr-root-dir-field">
            <EditableText
              label="Local path"
              queryKey={queryKeys.sonarr}
              value={mapping.localPath ?? ""}
              monospace
              onSave={(localPath) => {
                const next = [...mappings];
                next[index] = { ...mapping, localPath };
                return write(next);
              }}
            />
          </div>
          <Button
            variant="danger"
            onClick={() => void write(mappings.filter((_, i) => i !== index))}
          >
            ×
          </Button>
        </div>
      ))}

      {adding ? (
        <div className="sonarr-root-dir-row">
          <Input
            autoFocus
            value={sonarrPath}
            placeholder="/tv"
            className="editable-field-mono sonarr-root-dir-field"
            onChange={(event) => setSonarrPath(event.target.value)}
          />
          <Typography.Text type="secondary" aria-hidden="true">
            →
          </Typography.Text>
          <Input
            value={localPath}
            placeholder="/media/tv"
            className="editable-field-mono sonarr-root-dir-field"
            onChange={(event) => setLocalPath(event.target.value)}
          />
          <Button
            variant="primary"
            disabled={!sonarrPath.trim() || !localPath.trim()}
            onClick={() => {
              void write([
                ...mappings,
                {
                  sonarrPath: sonarrPath.trim(),
                  localPath: localPath.trim(),
                },
              ]);
              setSonarrPath("");
              setLocalPath("");
              setAdding(false);
            }}
          >
            Add
          </Button>
          <Button variant="ghost" onClick={() => setAdding(false)}>
            Cancel
          </Button>
        </div>
      ) : (
        <div>
          <Button onClick={() => setAdding(true)}>Add mapping</Button>
        </div>
      )}
    </Space>
  );
}

function NewInstance({
  existingSlugs,
  onCreate,
  onCancel,
}: {
  existingSlugs: string[];
  onCreate: (entry: SonarrInstanceInit) => Promise<void>;
  onCancel: () => void;
}) {
  const [slug, setSlug] = useState("");
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function submit() {
    if (!slug.trim()) {
      setError(
        "A slug is required — it is how the API addresses this instance",
      );
      return;
    }
    if (existingSlugs.includes(slug.trim())) {
      setError(
        "That slug is already in use; it would replace the existing instance",
      );
      return;
    }
    setError(null);
    await onCreate({
      instanceSlug: slug.trim(),
      instanceName: name.trim() || slug.trim(),
      sonarrUrl: url.trim(),
      sonarrApiKey: apiKey.trim(),
      rootDirMap: [],
      storage: { mode: "cache", ttl: "24h" },
    });
  }

  return (
    <div className="new-sonarr-instance">
      <Input
        autoFocus
        value={slug}
        placeholder="Slug, e.g. sonarr-main"
        className="editable-field-mono"
        onChange={(event) => setSlug(event.target.value)}
      />
      <Input
        value={name}
        placeholder="Display name"
        onChange={(event) => setName(event.target.value)}
      />
      <Input
        value={url}
        placeholder="http://localhost:8989"
        className="editable-field-mono"
        onChange={(event) => setUrl(event.target.value)}
      />
      <Input
        value={apiKey}
        placeholder="Sonarr API key"
        className="editable-field-mono"
        onChange={(event) => setApiKey(event.target.value)}
      />

      {error ? (
        <Typography.Text type="danger" style={{ fontSize: 12 }}>
          {error}
        </Typography.Text>
      ) : null}

      <Space>
        <Button variant="primary" onClick={() => void submit()}>
          Add
        </Button>
        <Button variant="ghost" onClick={onCancel}>
          Cancel
        </Button>
      </Space>
    </div>
  );
}
