import { useEffect, useState, type ReactNode } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { Button, Menu, Tag, Typography, type MenuProps } from "antd";
import { PushpinFilled, PushpinOutlined } from "@ant-design/icons";

import "./NavColumn.css";

/*
 * The left column. Declared as antd Menu items so the Searches, Workflows
 * and Automations areas the project is heading for slot in beside System
 * without touching the rendering. Group keys (group-*) are purely for
 * expand/collapse state; only leaf items carry a real route as their key,
 * since those double as both the Menu selection key and the navigation
 * target.
 */

function comingSoonLabel(text: string): ReactNode {
  return (
    <span className="nav-column-coming-soon">
      {text}
      <Tag className="nav-column-soon-tag">soon</Tag>
    </span>
  );
}

const items: MenuProps["items"] = [
  {
    key: "group-workflows",
    label: "Workflows",
    children: [
      { key: "/workflows", label: "All Workflows" },
      { key: "/workflows/add", label: "Add Workflow" },
    ],
  },
  { key: "group-searches", label: comingSoonLabel("Searches"), disabled: true },
  {
    key: "group-automations",
    label: comingSoonLabel("Automations"),
    disabled: true,
  },
  { key: "/tasks", label: "Tasks" },
  {
    key: "group-system",
    label: "System",
    children: [
      { key: "/system", label: "Overview" },
      {
        key: "group-system-configuration",
        label: "Configuration",
        children: [
          { key: "/system/directory-scanner", label: "Directory Scanner" },
          { key: "/system/sidecars", label: "Sidecars" },
          { key: "/system/interfaces", label: "Interfaces" },
          { key: "/system/event-bus", label: "Event Bus" },
          { key: "/system/security", label: "Security" },
        ],
      },
      { key: "/system/agents", label: "Agents" },
      { key: "/system/logging", label: "Logging" },
      { key: "/system/external-tools", label: "External Tools" },
    ],
  },
];

// Walks the item tree to find every group whose descendants include
// pathname, so the tree auto-expands to reveal whichever page is active.
function ancestorGroupKeys(
  nodes: MenuProps["items"],
  pathname: string,
  trail: string[] = [],
): string[] {
  for (const node of nodes ?? []) {
    if (!node || !("key" in node) || node.key == null) continue;
    const key = String(node.key);
    const children = "children" in node ? node.children : undefined;
    if (children && children.length > 0) {
      const found = ancestorGroupKeys(children, pathname, [...trail, key]);
      if (found.length > 0) return found;
    } else if (key === pathname) {
      return trail;
    }
  }
  return [];
}

export function NavColumn({
  pinned,
  onTogglePin,
}: {
  // Only present on the hover/pin variant (AppShell) — a plain embed with no
  // pin control just omits it.
  pinned?: boolean;
  onTogglePin?: () => void;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const [openKeys, setOpenKeys] = useState<string[]>(() =>
    ancestorGroupKeys(items, location.pathname),
  );

  // Navigating into a nested route additively opens its ancestor groups
  // without collapsing whatever the user opened by hand — a merge of route
  // state into UI state that has to happen after the location changes.
  useEffect(() => {
    // eslint-disable-next-line @eslint-react/set-state-in-effect
    setOpenKeys((current) =>
      Array.from(
        new Set([...current, ...ancestorGroupKeys(items, location.pathname)]),
      ),
    );
  }, [location.pathname]);

  return (
    <nav className="nav-column" aria-label="Main">
      <div className="nav-column-header">
        <div className="nav-column-title">
          <Typography.Text strong className="nav-column-brand">
            Metarr
          </Typography.Text>
          <Typography.Text type="secondary" className="nav-column-version">
            v{__APP_VERSION__}
          </Typography.Text>
        </div>
        {onTogglePin ? (
          <Button
            type="text"
            size="small"
            icon={pinned ? <PushpinFilled /> : <PushpinOutlined />}
            onClick={onTogglePin}
            aria-label={pinned ? "Unpin navigation" : "Pin navigation open"}
            title={pinned ? "Unpin" : "Pin open"}
          />
        ) : null}
      </div>

      <Menu
        mode="inline"
        items={items}
        selectedKeys={[location.pathname]}
        openKeys={openKeys}
        onOpenChange={setOpenKeys}
        onClick={({ key }) => {
          void navigate(key);
        }}
        className="nav-column-menu"
      />
    </nav>
  );
}
