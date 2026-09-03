import { Spin, Typography } from "antd";

import "./StartupGate.css";

/*
 * The deterministic loading state shown while the app resolves the
 * authentication scheme on a cold load (docs/adr/0012). It stands in for
 * both the app shell and the login screen until App.tsx knows which to
 * render, so startup never flashes one on the way to the other.
 */
export function StartupGate() {
  return (
    <div className="startup-gate" role="status" aria-live="polite">
      <div className="startup-gate-panel">
        <Typography.Title level={2} className="startup-gate-title">
          Metarr
        </Typography.Title>
        <Spin />
        <Typography.Text type="secondary">Starting…</Typography.Text>
      </div>
    </div>
  );
}
