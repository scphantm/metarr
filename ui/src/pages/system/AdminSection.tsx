import { useState } from "react";
import { Input, Space, Typography } from "antd";

import { queryKeys, useUpdateAdmin } from "../../api/queries";
import type { AdminUser } from "../../gen/metarr/v1/admin_pb";
import { Button, Card, Row } from "../../components/Card";
import { EditableText } from "../../components/Editable";
import { SaveIndicator } from "../../components/SaveState";
import "./AdminSection.css";

/*
 * The admin account. Username and email edit in place; the password does not —
 * a credential you cannot read back is a bad fit for click-to-edit, and it
 * needs confirming against a second field before it is sent.
 */
export function AdminSection({ admin }: { admin: AdminUser }) {
  const updateAdmin = useUpdateAdmin();

  return (
    <Card
      title="Administrator"
      description="The single administrative account. These credentials are what the sign-in form checks against."
    >
      <Row label="Username">
        <EditableText
          label="Username"
          queryKey={queryKeys.config}
          value={admin.username}
          validate={(next) => (next ? null : "Username cannot be empty")}
          onSave={(username) => updateAdmin.mutateAsync({ username })}
        />
      </Row>

      <Row label="Email">
        <EditableText
          label="Email"
          queryKey={queryKeys.config}
          value={admin.email}
          validate={(next) =>
            next.includes("@") ? null : "Must be an email address"
          }
          onSave={(email) => updateAdmin.mutateAsync({ email })}
        />
      </Row>

      <Row
        label="Password"
        hint="Never displayed; the server stores only a salted hash"
      >
        <PasswordChanger />
      </Row>
    </Card>
  );
}

function PasswordChanger() {
  const updateAdmin = useUpdateAdmin();

  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [done, setDone] = useState(false);

  async function submit() {
    if (password.length < 8) {
      setError("Use at least 8 characters");
      return;
    }
    if (password !== confirmation) {
      setError("The two entries do not match");
      return;
    }

    setError(null);
    try {
      await updateAdmin.mutateAsync({ password });
      setDone(true);
      setOpen(false);
      setPassword("");
      setConfirmation("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  }

  if (!open) {
    return (
      <Space align="center">
        <Button onClick={() => setOpen(true)}>Change password</Button>
        {done ? (
          <SaveIndicator state="pending" />
        ) : (
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            ••••••••
          </Typography.Text>
        )}
      </Space>
    );
  }

  return (
    <div className="admin-password-form">
      <Input.Password
        autoFocus
        value={password}
        placeholder="New password"
        autoComplete="new-password"
        onChange={(event) => setPassword(event.target.value)}
      />
      <Input.Password
        value={confirmation}
        placeholder="Confirm password"
        autoComplete="new-password"
        onChange={(event) => setConfirmation(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") void submit();
          if (event.key === "Escape") setOpen(false);
        }}
      />
      {error ? (
        <Typography.Text type="danger" style={{ fontSize: 12 }}>
          {error}
        </Typography.Text>
      ) : null}
      <Space>
        <Button variant="primary" onClick={() => void submit()}>
          Save password
        </Button>
        <Button variant="ghost" onClick={() => setOpen(false)}>
          Cancel
        </Button>
      </Space>
    </div>
  );
}
