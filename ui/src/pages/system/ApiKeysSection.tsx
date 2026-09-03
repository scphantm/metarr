import { useState } from "react";
import { Input, Space, Typography } from "antd";

import {
  useCreateApiKey,
  useDeleteApiKey,
  useUpdateApiKey,
} from "../../api/queries";
import {
  AccessLevel,
  type APIKeysConfig,
} from "../../gen/metarr/v1/api_keys_pb";
import type { Config } from "../../gen/metarr/v1/config_pb";
import { Button, Card, EmptyState } from "../../components/Card";
import { EditableText } from "../../components/Editable";
import "./ApiKeysSection.css";

// Not `keyof APIKeysConfig` — that also picks up $typeName/$unknown from the
// branded message type, neither of which is a real key group.
type APIKeyGroup = "admin" | "user" | "webhook" | "readOnly";

// ApiKeyService scopes Create / List by the AccessLevel enum, not by a
// string group name (docs/adr/0010). This maps the UI's own camelCase group
// names — which also index the aggregate Config.apiKeys read — to that enum.
const accessLevelOf: Record<APIKeyGroup, AccessLevel> = {
  admin: AccessLevel.ADMIN,
  user: AccessLevel.USER,
  webhook: AccessLevel.WEBHOOK,
  readOnly: AccessLevel.READ_ONLY,
};

const apiKeyGroups: { key: APIKeyGroup; label: string; hint: string }[] = [
  { key: "admin", label: "Admin", hint: "Full access to every endpoint" },
  { key: "user", label: "User", hint: "Tasks and library reads" },
  { key: "webhook", label: "Webhook", hint: "For inbound automation" },
  { key: "readOnly", label: "Read only", hint: "Library reads only" },
];

/*
 * Each key is addressed by its own minted id through a scoped upsert/delete
 * — see ADR 0001 — never by sending the whole configuration document back.
 *
 * The keys come back from the server in cleartext, so they are masked until
 * asked for. Editing one in place is genuinely useful: this is where a key is
 * pasted in after being generated elsewhere.
 */
export function ApiKeysSection({ config }: { config: Config }) {
  const createApiKey = useCreateApiKey();
  const updateApiKey = useUpdateApiKey();
  const deleteApiKey = useDeleteApiKey();
  const [addingTo, setAddingTo] = useState<APIKeyGroup | null>(null);
  const [draftName, setDraftName] = useState("");

  const apiKeys: APIKeysConfig = config.apiKeys ?? {
    $typeName: "metarr.v1.APIKeysConfig",
    admin: [],
    user: [],
    webhook: [],
    readOnly: [],
  };

  return (
    <Card
      title="API keys"
      description="Static keys, grouped by the access each grants. A key is shown only when you ask for it."
    >
      <Space direction="vertical" size={24} style={{ width: "100%" }}>
        {apiKeyGroups.map(({ key: group, label, hint }) => {
          const entries = apiKeys[group] ?? [];
          const accessLevel = accessLevelOf[group];

          return (
            <div key={group}>
              <div className="api-key-group-header">
                <div>
                  <Typography.Text className="api-key-group-label">
                    {label}
                  </Typography.Text>
                  <Typography.Text
                    type="secondary"
                    className="api-key-group-hint"
                  >
                    {hint}
                  </Typography.Text>
                </div>
                <Button
                  onClick={() => {
                    setAddingTo(group);
                    setDraftName("");
                  }}
                >
                  Add key
                </Button>
              </div>

              {entries.length === 0 && addingTo !== group ? (
                <EmptyState>No {label.toLowerCase()} keys</EmptyState>
              ) : (
                <Space direction="vertical" size={4} style={{ width: "100%" }}>
                  {entries.map((entry) => (
                    <div key={entry.id} className="api-key-row">
                      <div className="api-key-row-name">
                        <EditableText
                          label="Key name"
                          value={entry.name}
                          placeholder="Unnamed"
                          onSave={(name) =>
                            updateApiKey.mutateAsync({ id: entry.id, name })
                          }
                        />
                      </div>
                      <div className="api-key-row-value">
                        <EditableText
                          label="API key"
                          value={entry.apiKey}
                          placeholder="No key set"
                          monospace
                          secret
                          onSave={(apiKey) =>
                            updateApiKey.mutateAsync({ id: entry.id, apiKey })
                          }
                        />
                      </div>
                      <Button
                        variant="danger"
                        title={`Remove ${entry.name || "this key"}`}
                        onClick={() => void deleteApiKey.mutateAsync(entry.id)}
                      >
                        Remove
                      </Button>
                    </div>
                  ))}
                </Space>
              )}

              {addingTo === group ? (
                <div className="api-key-add-row">
                  <Input
                    autoFocus
                    value={draftName}
                    placeholder="Name for the new key"
                    onChange={(event) => setDraftName(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key === "Escape") setAddingTo(null);
                      if (event.key === "Enter" && draftName.trim()) {
                        void createApiKey.mutateAsync({
                          accessLevel,
                          name: draftName.trim(),
                        });
                        setAddingTo(null);
                      }
                    }}
                  />
                  <Button
                    variant="primary"
                    disabled={!draftName.trim()}
                    onClick={() => {
                      void createApiKey.mutateAsync({
                        accessLevel,
                        name: draftName.trim(),
                      });
                      setAddingTo(null);
                    }}
                  >
                    Add
                  </Button>
                  <Button variant="ghost" onClick={() => setAddingTo(null)}>
                    Cancel
                  </Button>
                </div>
              ) : null}
            </div>
          );
        })}
      </Space>
    </Card>
  );
}
