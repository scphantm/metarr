import type { ReactNode } from "react";
import { Button, Segmented, Typography } from "antd";
import { PushpinFilled, PushpinOutlined, LockOutlined } from "@ant-design/icons";
import { useNavigate } from "react-router-dom";

import { useAuth } from "../auth/AuthContext";
import { useTheme } from "../theme/ThemeContext";
import { useAuthScheme } from "../api/queries";
import { AuthenticationScheme } from "../gen/metarr/v1/admin_pb";
import "./Sidebar.css";

/*
 * The right column. Pages fill it through the SidebarContent slot; what
 * lives here permanently is the pin control, the session, and the theme
 * switch.
 */
export function Sidebar({
  children,
  pinned,
  onTogglePin,
}: {
  children?: ReactNode;
  pinned?: boolean;
  onTogglePin?: () => void;
}) {
  const { theme, toggleTheme } = useTheme();
  const { username, expiresAt, logout } = useAuth();
  const authScheme = useAuthScheme();
  const navigate = useNavigate();

  const schemeIsNone = authScheme.data === AuthenticationScheme.NONE;

  return (
    <aside className="sidebar" aria-label="Sidebar">
      <div className="sidebar-header">
        <Typography.Text type="secondary" className="sidebar-header-label">
          Panel
        </Typography.Text>
        {onTogglePin ? (
          <Button
            type="text"
            size="small"
            icon={pinned ? <PushpinFilled /> : <PushpinOutlined />}
            onClick={onTogglePin}
            aria-label={pinned ? "Unpin panel" : "Pin panel open"}
            title={pinned ? "Unpin" : "Pin open"}
          />
        ) : null}
      </div>

      {schemeIsNone ? (
        <section className="sidebar-section">
          <Typography.Text type="secondary" className="sidebar-section-title">
            Security
          </Typography.Text>
          <Button
            type="text"
            danger
            icon={<LockOutlined />}
            onClick={() => navigate("/system/security")}
            className="sidebar-auth-disabled"
            title="Authentication is disabled. Click to enable it."
          >
            Authentication disabled
          </Button>
        </section>
      ) : (
        <section className="sidebar-section">
          <Typography.Text type="secondary" className="sidebar-section-title">
            Session
          </Typography.Text>
          <div className="sidebar-session-card">
            <div className="sidebar-session-name">{username ?? "Signed in"}</div>
            {expiresAt ? (
              <Typography.Text
                type="secondary"
                className="sidebar-session-expiry"
              >
                Expires {new Date(expiresAt).toLocaleTimeString()}
              </Typography.Text>
            ) : null}
            <Button
              type="link"
              size="small"
              danger
              onClick={() => void logout()}
              className="sidebar-signout"
            >
              Sign out
            </Button>
          </div>
        </section>
      )}

      <section className="sidebar-section">
        <Typography.Text type="secondary" className="sidebar-section-title">
          Appearance
        </Typography.Text>
        <Segmented
          block
          value={theme}
          onChange={(value) => value !== theme && toggleTheme()}
          options={[
            { label: "Dark", value: "dark" },
            { label: "Light", value: "light" },
          ]}
        />
        <Typography.Text type="secondary" className="sidebar-appearance-note">
          Solarized
        </Typography.Text>
      </section>

      {children}
    </aside>
  );
}
