import { useIsFetching, useQueryClient } from "@tanstack/react-query";

import { useSidecarTypes } from "../../api/queries";
import { Button } from "../../components/Card";
import { PageError, PageLoading } from "../../components/PageState";
import { PageHeader } from "../../layout/AppShell";
import { SavingInfoSidebar } from "./SavingInfoSidebar";
import { SidecarTypesSection } from "./SidecarTypesSection";

export function SidecarsPage() {
  const sidecarTypes = useSidecarTypes();

  const queryClient = useQueryClient();
  const fetching = useIsFetching({ queryKey: ["config"] });

  if (sidecarTypes.error) {
    return (
      <>
        <PageHeader title="Sidecars" />
        <PageError error={sidecarTypes.error} />
      </>
    );
  }

  if (sidecarTypes.isLoading) {
    return (
      <>
        <PageHeader title="Sidecars" />
        <PageLoading />
      </>
    );
  }

  return (
    <>
      <PageHeader
        title="Sidecars"
        description="How the scanner classifies non-media files found next to media, stored in the single application config document. Click a value to edit it."
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
        <SidecarTypesSection types={sidecarTypes.data ?? []} />
      </div>
    </>
  );
}

export function SidecarsSidebar() {
  return <SavingInfoSidebar />;
}
