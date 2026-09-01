import { useIsFetching, useQueryClient } from "@tanstack/react-query";

import { useConfig } from "../../api/queries";
import { Button } from "../../components/Card";
import { PageError, PageLoading } from "../../components/PageState";
import { PageHeader } from "../../layout/AppShell";
import { AdminSection } from "./AdminSection";
import { ApiKeysSection } from "./ApiKeysSection";
import { SavingInfoSidebar } from "./SavingInfoSidebar";

export function SecurityPage() {
  const config = useConfig();

  const queryClient = useQueryClient();
  const fetching = useIsFetching({ queryKey: ["config"] });

  if (config.error) {
    return (
      <>
        <PageHeader title="Security" />
        <PageError error={config.error} />
      </>
    );
  }

  if (config.isLoading || !config.data || !config.data.admin) {
    return (
      <>
        <PageHeader title="Security" />
        <PageLoading />
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Security"
        description="The administrator account and API keys, both stored in the single application config document. Click a value to edit it."
        actions={
          <Button
            onClick={() =>
              void queryClient.invalidateQueries({ queryKey: ["config"] })
            }
            title="Re-read every configuration section from the server"
          >
            {fetching ? "Refreshing…" : "Refresh"}
          </Button>
        }
      />

      <div className="page-body">
        <AdminSection admin={config.data.admin} />

        <ApiKeysSection config={config.data} />
      </div>
    </>
  );
}

export function SecuritySidebar() {
  return <SavingInfoSidebar />;
}
