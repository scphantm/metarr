/*
 * A hand-rolled inline sparkline — a single <polyline> in a fixed viewBox,
 * no charting library. It draws the shape of a rolling metric series so an
 * operator can read a trend (backing up, draining, flat) without watching
 * the number live.
 *
 * Geometry is computed here because it is data, not style; stroke colour and
 * weight live in SystemDashboardPage.css. The line is scaled to fill the box
 * (preserveAspectRatio="none") and kept crisp with a non-scaling stroke.
 */

const VIEW_W = 100
const VIEW_H = 24

type SparklineProps = {
  values: ReadonlyArray<bigint | number> | undefined
  className?: string
  title?: string
}

export function Sparkline({ values, className, title }: SparklineProps) {
  const points = (values ?? []).map(Number).filter((n) => Number.isFinite(n))

  // One point is not a trend; nothing to draw.
  if (points.length < 2) return null

  const min = Math.min(...points)
  const max = Math.max(...points)
  const span = max - min

  const stepX = VIEW_W / (points.length - 1)
  const path = points
    .map((value, index) => {
      const x = index * stepX
      // A flat series sits on the mid-line rather than pinning to an edge.
      const y = span === 0 ? VIEW_H / 2 : VIEW_H - ((value - min) / span) * VIEW_H
      return `${x.toFixed(2)},${y.toFixed(2)}`
    })
    .join(' ')

  return (
    <svg
      className={['system-dashboard-sparkline', className].filter(Boolean).join(' ')}
      viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
      preserveAspectRatio="none"
      role="img"
      aria-label={title ?? 'trend'}
    >
      {title ? <title>{title}</title> : null}
      <polyline points={path} />
    </svg>
  )
}
