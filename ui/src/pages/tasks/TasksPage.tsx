import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Alert, Button, Select, Typography } from "antd";

import { useRunDirectoryScan, useScanDirectories } from "../../api/queries";
import type { ScanDirectory } from "../../gen/metarr/v1/directory_scanner_pb";
import { Card } from "../../components/Card";
import { PageError } from "../../components/PageState";
import { PageHeader } from "../../layout/AppShell";
import "./TasksPage.css";

/*
 * Tasks is a top-level area — a peer of Workflows and System — for
 * operator-triggered background jobs. Each control lives in its own Card so
 * more can be added as sibling cards without reworking the page. There is no
 * client-side permission gating here: the server enforces the `tasks` group
 * and every current session is an admin.
 */

type Feedback = { type: "success" | "error"; text: string };

// The success/error line is transient: it clears itself after a few seconds so
// the card settles back to a neutral state without the operator dismissing it.
const FEEDBACK_TIMEOUT_MS = 6000;

function DirectoryScanCard({
  scanDirectories,
  loading,
}: {
  scanDirectories: ScanDirectory[];
  loading: boolean;
}) {
  const runScan = useRunDirectoryScan();
  const [selectedSlug, setSelectedSlug] = useState<string | undefined>(
    undefined,
  );
  const [feedback, setFeedback] = useState<Feedback | null>(null);

  useEffect(() => {
    if (!feedback) return;
    const timer = window.setTimeout(() => setFeedback(null), FEEDBACK_TIMEOUT_MS);
    return () => window.clearTimeout(timer);
  }, [feedback]);

  const noDirectories = !loading && scanDirectories.length === 0;
  const controlsDisabled = noDirectories || runScan.isPending;

  async function kickOff() {
    if (!selectedSlug) return;
    try {
      const { scanId } = await runScan.mutateAsync(selectedSlug);
      setFeedback({
        type: "success",
        text: `Directory scan started — scan id ${scanId}.`,
      });
    } catch (cause) {
      setFeedback({
        type: "error",
        text: cause instanceof Error ? cause.message : String(cause),
      });
    }
  }

  return (
    <Card
      title="Directory scan"
      description="Rescan one configured library. The scan runs on the agent that owns it."
    >
      <div className="tasks-directory-scan">
        <Select
          className="tasks-directory-scan-select"
          placeholder="Select a scan directory"
          value={selectedSlug}
          onChange={setSelectedSlug}
          disabled={controlsDisabled}
          options={scanDirectories.map((directory) => ({
            value: directory.scannerSlug,
            label: `${directory.scannerSlug} — ${directory.directory}`,
          }))}
        />

        <Button
          type="primary"
          loading={runScan.isPending}
          disabled={controlsDisabled || !selectedSlug}
          onClick={() => void kickOff()}
        >
          Kick off directory scan
        </Button>

        {noDirectories ? (
          <Typography.Text type="secondary">
            No scan directories are configured yet. Add one on the{" "}
            <Link to="/system/directory-scanner">Directory Scanner</Link>{" "}
            screen.
          </Typography.Text>
        ) : null}

        {feedback ? (
          <Alert
            className="tasks-directory-scan-feedback"
            type={feedback.type}
            showIcon
            message={feedback.text}
          />
        ) : null}
      </div>
    </Card>
  );
}

export function TasksPage() {
  const directories = useScanDirectories();

  if (directories.error) {
    return (
      <>
        <PageHeader title="Tasks" />
        <PageError error={directories.error} />
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Tasks"
        description="Kick off background jobs by hand. Each runs on the agent that owns the affected library."
      />

      <div className="page-body">
        <DirectoryScanCard
          scanDirectories={directories.data ?? []}
          loading={directories.isLoading}
        />
      </div>
    </>
  );
}

export function TasksSidebar() {
  return (
    <div className="saving-info-sidebar">
      <Alert
        type="info"
        message="About tasks"
        description={
          <>
            <p>
              A directory scan is dispatched to the agent mapped to that
              library, not run here. If the agent is briefly offline the command
              waits on its durable queue and runs when it returns.
            </p>
            <p>
              Results arrive asynchronously — the scan id lets you tie later
              activity back to the run you started.
            </p>
          </>
        }
      />
    </div>
  );
}
