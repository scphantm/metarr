import type { Workflow } from '../../api/types'

export function VersionHistory({
  versions,
  viewingVersion,
  onSelect,
}: {
  versions: Workflow[]
  viewingVersion: number | null
  onSelect: (version: number) => void
}) {
  if (versions.length === 0) return null

  return (
    <div className="flex flex-col gap-1 border-l border-edge px-3 py-2">
      <h3 className="px-1 text-[11px] tracking-wide text-ink-muted uppercase">History</h3>
      {versions.map((version) => (
        <button
          key={version.version}
          type="button"
          onClick={() => onSelect(version.version)}
          className={`rounded px-2 py-1 text-left text-xs transition-colors ${
            viewingVersion === version.version
              ? 'bg-surface-hover font-medium text-blue'
              : 'text-ink hover:bg-surface-hover hover:text-ink-strong'
          }`}
        >
          v{version.version}
          <span className="ml-1.5 text-ink-muted">
            {new Date(version.created_at).toLocaleString()}
          </span>
        </button>
      ))}
    </div>
  )
}
