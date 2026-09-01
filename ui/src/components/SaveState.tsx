import { Space, Spin, Typography } from "antd";
import {
  CheckCircleFilled,
  CloseCircleFilled,
  ClockCircleOutlined,
  ExclamationCircleFilled,
} from "@ant-design/icons";

import type { SaveState } from "./useSaveState";

/*
 * The visual vocabulary for the save lifecycle. It is deliberately small and
 * always in the same place, so a user learns it once: a spinner means in
 * flight, a clock means accepted but not yet stored, a tick means the server
 * has confirmed it, and anything red needs reading.
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
          Sending…
        </Typography.Text>
      </Space>
    );
  }

  if (state === "pending") {
    return (
      <Space
        size={4}
        align="center"
        title="The API accepted this write and queued it. It is stored once the background listener has processed the event."
      >
        <ClockCircleOutlined style={{ color: "var(--color-yellow)" }} />
        <Typography.Text style={{ fontSize: 12, color: "var(--color-yellow)" }}>
          Queued
        </Typography.Text>
      </Space>
    );
  }

  if (state === "unconfirmed") {
    return (
      <Space
        size={4}
        align="center"
        title="The write was accepted but the server has not reported the new value yet. It may still land; reload to check."
      >
        <ExclamationCircleFilled style={{ color: "var(--color-orange)" }} />
        <Typography.Text style={{ fontSize: 12, color: "var(--color-orange)" }}>
          Not confirmed
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
