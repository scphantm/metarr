import { Alert } from 'antd'

// Shared explanation for pages under System whose fields save through the
// same queued/saved model — Configuration and Security both edit fields this
// way, so they share this copy rather than each carrying their own.
export function SavingInfoSidebar() {
  return (
    <div className="saving-info-sidebar">
      <Alert
        type="info"
        message="About saving"
        description={
          <>
            <p>
              Metarr is eventually consistent. Saving fires an event and returns immediately; a
              background listener writes it to the database a moment later.
            </p>
            <ul>
              <li>
                <span style={{ color: 'var(--color-yellow)' }}>◌ Queued</span> — accepted, not yet
                stored
              </li>
              <li>
                <span style={{ color: 'var(--color-green)' }}>✓ Saved</span> — confirmed by the
                server
              </li>
              <li>
                <span style={{ color: 'var(--color-orange)' }}>! Not confirmed</span> — no
                read-back after 20s
              </li>
            </ul>
          </>
        }
      />

      <Alert
        type="info"
        message="Editing"
        description={
          <>
            Edit a field directly. <kbd>Enter</kbd> or clicking away saves, <kbd>Escape</kbd>{' '}
            reverts. Slugs and ids are fixed once created — the API keys records by them.
          </>
        }
      />
    </div>
  )
}
