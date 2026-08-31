import { Alert, Typography } from 'antd'

import { queryKeys, useEventBusConfig, useUpdateEventBusConfig } from '../../api/queries'
import type { EventBusConfig } from '../../gen/metarr/v1/event_bus_pb'
import { Card, Row } from '../../components/Card'
import { EditableNumber } from '../../components/Editable'
import { PageError, PageLoading } from '../../components/PageState'
import { PageHeader } from '../../layout/AppShell'

/*
 * System > Event Bus.
 *
 * The knobs behind the Redis event bus: how large a stream is allowed to
 * grow, how far back history is kept, and how hard the Router retries a
 * failing handler before its message is logged and dropped.
 *
 * Unlike the log level, these are read once when the server starts, so an
 * edit here takes effect on the next restart rather than live. Every field
 * saves the whole section, so the server can reject a contradictory
 * combination (a max backoff below the base, a non-positive stream cap).
 */
export function EventBusPage() {
  const eventBus = useEventBusConfig()

  if (eventBus.error && !eventBus.data) {
    return (
      <>
        <PageHeader title="Event Bus" />
        <PageError error={eventBus.error} />
      </>
    )
  }

  if (!eventBus.data) {
    return (
      <>
        <PageHeader title="Event Bus" />
        <PageLoading />
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Event Bus"
        description="The stream size cap, the retention window, and the Router's retry policy. Read at server startup — changes apply on the next restart."
      />

      <div className="page-body">
        <EventBusFields config={eventBus.data} />
      </div>
    </>
  )
}

// One field is a Row + EditableNumber that saves the whole section with this
// one value replaced; the server validates the combination.
type SaveField = (patch: Partial<EventBusConfig>) => Promise<unknown>

function NumberField({
  label,
  hint,
  value,
  min,
  field,
  save,
}: {
  label: string
  hint: string
  value: number
  min: number
  field: keyof EventBusConfig
  save: SaveField
}) {
  return (
    <Row label={label} hint={hint}>
      <EditableNumber
        label={label}
        queryKey={queryKeys.eventBus}
        value={value}
        min={min}
        onSave={(next) => save({ [field]: next })}
      />
    </Row>
  )
}

function EventBusFields({ config }: { config: EventBusConfig }) {
  const update = useUpdateEventBusConfig()
  const save: SaveField = (patch) => update.mutateAsync({ ...config, ...patch })

  return (
    <>
      <Card
        title="Retention"
        description="Every publish sets one approximate MAXLEN for every stream; a periodic sweep also trims each stream by age."
      >
        <NumberField
          label="Stream cap"
          hint="Approximate entry cap applied to every stream. Must be greater than zero."
          value={config.maxLen}
          min={1}
          field="maxLen"
          save={save}
        />
        <NumberField
          label="Retention window (hours)"
          hint="How far back the age sweep keeps entries. The documented floor for external subscribers is 48; going lower narrows that guarantee."
          value={config.retentionHours}
          min={1}
          field="retentionHours"
          save={save}
        />
      </Card>

      <Card
        title="Retry"
        description="A handler error is retried with exponential backoff; a message that fails past the cap is logged at error level and acked (dropped)."
      >
        <NumberField
          label="Retry attempts"
          hint="Retries after the first attempt before a message is parked."
          value={config.retryAttempts}
          min={0}
          field="retryAttempts"
          save={save}
        />
        <NumberField
          label="Backoff base (ms)"
          hint="Wait before the first retry."
          value={config.retryBackoffBaseMs}
          min={1}
          field="retryBackoffBaseMs"
          save={save}
        />
        <NumberField
          label="Backoff max (ms)"
          hint="Ceiling for the exponential backoff between retries."
          value={config.retryBackoffMaxMs}
          min={1}
          field="retryBackoffMaxMs"
          save={save}
        />
      </Card>
    </>
  )
}

export function EventBusSidebar() {
  return (
    <div className="saving-info-sidebar">
      <Alert
        type="info"
        message="Applied at startup"
        description={
          <>
            <p>
              The server reads this section once, when it starts. Saving a change here does not
              reconfigure the running process — restart the server for it to take effect.
            </p>
            <p>
              The agent runs on its own built-in defaults for these values; it does not read this
              section.
            </p>
          </>
        }
      />
      <Alert
        type="info"
        message="Reading the numbers"
        description={
          <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 0 }}>
            A stream is bounded by whichever is larger — its MAXLEN in entries, or the retention
            window in time. A message whose handler keeps failing past the retry cap is logged at
            error level with its identifier and then dropped, not parked.
          </Typography.Paragraph>
        }
      />
    </div>
  )
}
