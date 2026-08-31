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
 * The knobs behind the Redis event bus: how large each stream is allowed to
 * grow, how far back history is kept, and how hard the Router retries a
 * failing handler before parking its message on events.dead_letter.
 *
 * Unlike the log level, these are read once when the server starts, so an
 * edit here takes effect on the next restart rather than live. Every field
 * saves independently against the whole section, so the server can reject a
 * contradictory combination (a max backoff below the base, a high cap below
 * the default one).
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
        description="Stream size caps, the retention window, and the Router's retry-then-dead-letter policy. Read at server startup — changes apply on the next restart."
      />

      <div className="page-body">
        <RetentionCard config={eventBus.data} />
        <RetryCard config={eventBus.data} />
      </div>
    </>
  )
}

function RetentionCard({ config }: { config: EventBusConfig }) {
  const update = useUpdateEventBusConfig()
  const save = (patch: Partial<EventBusConfig>) => update.mutateAsync({ ...config, ...patch })

  return (
    <Card
      title="Retention"
      description="Every publish sets an approximate MAXLEN by stream tier; a periodic sweep also trims each stream by age."
    >
      <Row
        label="High-volume cap"
        hint="Approximate entry cap for the agent result streams (scan results, node results)."
      >
        <EditableNumber
          label="High-volume cap"
          queryKey={queryKeys.eventBus}
          value={config.maxLenHigh}
          min={1}
          onSave={(maxLenHigh) => save({ maxLenHigh })}
        />
      </Row>
      <Row
        label="Default cap"
        hint="Approximate entry cap for every other stream, including events.dead_letter."
      >
        <EditableNumber
          label="Default cap"
          queryKey={queryKeys.eventBus}
          value={config.maxLenDefault}
          min={1}
          onSave={(maxLenDefault) => save({ maxLenDefault })}
        />
      </Row>
      <Row
        label="Retention window (hours)"
        hint="How far back the age sweep keeps entries. The documented floor for external subscribers is 48; going lower narrows that guarantee."
      >
        <EditableNumber
          label="Retention window (hours)"
          queryKey={queryKeys.eventBus}
          value={config.retentionHours}
          min={1}
          onSave={(retentionHours) => save({ retentionHours })}
        />
      </Row>
    </Card>
  )
}

function RetryCard({ config }: { config: EventBusConfig }) {
  const update = useUpdateEventBusConfig()
  const save = (patch: Partial<EventBusConfig>) => update.mutateAsync({ ...config, ...patch })

  return (
    <Card
      title="Retry & dead-letter"
      description="A handler error is retried with exponential backoff; a message that fails past the cap is parked on events.dead_letter and acked."
    >
      <Row label="Retry attempts" hint="Retries after the first attempt before a message is parked.">
        <EditableNumber
          label="Retry attempts"
          queryKey={queryKeys.eventBus}
          value={config.retryAttempts}
          min={0}
          onSave={(retryAttempts) => save({ retryAttempts })}
        />
      </Row>
      <Row label="Backoff base (ms)" hint="Wait before the first retry.">
        <EditableNumber
          label="Backoff base (ms)"
          queryKey={queryKeys.eventBus}
          value={config.retryBackoffBaseMs}
          min={1}
          onSave={(retryBackoffBaseMs) => save({ retryBackoffBaseMs })}
        />
      </Row>
      <Row label="Backoff max (ms)" hint="Ceiling for the exponential backoff between retries.">
        <EditableNumber
          label="Backoff max (ms)"
          queryKey={queryKeys.eventBus}
          value={config.retryBackoffMaxMs}
          min={1}
          onSave={(retryBackoffMaxMs) => save({ retryBackoffMaxMs })}
        />
      </Row>
    </Card>
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
            window in time. The dead-letter stream has no consumer; a non-zero length there is
            parked work waiting to be inspected on the System dashboard.
          </Typography.Paragraph>
        }
      />
    </div>
  )
}
