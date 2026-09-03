import { Space, Spin, Typography } from "antd";
import { CheckCircleFilled, CloseCircleFilled } from "@ant-design/icons";

import type { SaveState } from "./useSaveState";

/*
 * The visual vocabulary for the save lifecycle. It is deliberately small and
 * always in the same place, so a user learns it once: a spinner means the
 * write is in flight, a tick means the server has stored it, and anything red
 * needs reading. Writes are synchronous, so there is no "queued" state.
 */

export function SaveIndicator({
  state,
  error,
  onDismissError,
}: {
  state: SaveState;
  error?: string | null;
  onDismissError?: () => void;
}) {
  if (state === "idle") {
    return null;
  }

  if (state === "error") {
    return (
      <Space size={4} align="center">
        <CloseCircleFilled style={{ color: "var(--color-red)" }} />
        <Typography.Text type="danger" style={{ fontSize: 12 }}>
          {error ?? "Save failed"}
        </Typography.Text>
        {onDismissError ? (
          <Typography.Link onClick={onDismissError} style={{ fontSize: 12 }}>
            dismiss
          </Typography.Link>
        ) : null}
      </Space>
    );
  }

  if (state === "saving") {
    return (
      <Space size={4} align="center">
        <Spin size="small" />
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          Saving…
        </Typography.Text>
      </Space>
    );
  }

  return (
    <Space size={4} align="center">
      <CheckCircleFilled style={{ color: "var(--color-green)" }} />
      <Typography.Text style={{ fontSize: 12, color: "var(--color-green)" }}>
        Saved
      </Typography.Text>
    </Space>
  );
}
