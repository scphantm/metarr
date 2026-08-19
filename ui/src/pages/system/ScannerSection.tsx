import { queryKeys, useUpdateDirectoryScanner } from '../../api/queries'
import { Card, Row } from '../../components/Card'
import { EditableNumber } from '../../components/Editable'

export function ScannerSection({ parallelCount }: { parallelCount: number }) {
  const updateScanner = useUpdateDirectoryScanner()

  return (
    <Card
      title="Directory scanner"
      description="How the background scanner walks the configured libraries."
    >
      <Row
        label="Parallel count"
        hint="Directories scanned at once. The server rejects anything below 1."
      >
        <EditableNumber
          label="Parallel count"
          queryKey={queryKeys.directoryScanner}
          value={parallelCount}
          min={1}
          onSave={(parallel_count) =>
            updateScanner.mutateAsync({ parallel_count })
          }
        />
      </Row>
    </Card>
  )
}
