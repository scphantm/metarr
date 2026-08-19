// Shared explanation for pages under System whose fields save through the
// same queued/saved model — Configuration and Security both edit fields this
// way, so they share this copy rather than each carrying their own.
export function SavingInfoSidebar() {
  return (
    <section>
      <h2 className="mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        About saving
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        <p>
          Metarr is eventually consistent. Saving fires an event and returns
          immediately; a background listener writes it to the database a moment
          later.
        </p>
        <ul className="mt-2 flex flex-col gap-1">
          <li>
            <span className="text-yellow">◌ Queued</span> — accepted, not yet
            stored
          </li>
          <li>
            <span className="text-green">✓ Saved</span> — confirmed by the
            server
          </li>
          <li>
            <span className="text-orange">! Not confirmed</span> — no read-back
            after 20s
          </li>
        </ul>
      </div>

      <h2 className="mt-6 mb-2 text-xs font-semibold tracking-wide text-ink-muted uppercase">
        Editing
      </h2>
      <div className="rounded border border-edge bg-surface px-3 py-2.5 text-xs leading-relaxed text-ink-muted">
        Click any value to edit it. <kbd className="text-ink">Enter</kbd> saves,{' '}
        <kbd className="text-ink">Escape</kbd> cancels. Slugs and ids are fixed
        once created — the API keys records by them.
      </div>
    </section>
  )
}
