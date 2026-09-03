import { Alert } from "antd";

// Shared editing hint for the System pages whose fields save in place —
// Configuration and Security both edit fields this way. Config writes are
// synchronous now (docs/adr/0002): a save persists and returns the stored
// value before the request resolves, so there is no queued/confirmed state
// to explain.
export function SavingInfoSidebar() {
  return (
    <div className="saving-info-sidebar">
      <Alert
        type="info"
        message="Editing"
        description={
          <>
            Edit a field directly. <kbd>Enter</kbd> or clicking away saves,{" "}
            <kbd>Escape</kbd> reverts. A save persists immediately; a green tick
            confirms it. Slugs and ids are fixed once created — the API keys
            records by them.
          </>
        }
      />
    </div>
  );
}
