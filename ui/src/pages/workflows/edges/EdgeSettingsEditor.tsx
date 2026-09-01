import { useState } from "react";
import { Checkbox, Modal, Typography } from "antd";

/*
 * The edit form for one data edge's settings, opened by double-clicking a
 * path-typed edge (see DataEdge.tsx). Same antd Modal as
 * NodeSettingsEditor.tsx, which floats above React Flow's zoomed/panned
 * canvas transform regardless of where in the DOM it renders.
 *
 * Unlike a node's settings, an edge has no catalog-declared Setting[] to
 * drive a generic form from — there is no per-type edge schema
 * (workflow.Edge.Settings has no catalog counterpart). Recursive is
 * currently the only edge setting that exists, hardcoded here rather than
 * built from a list of one; generalize this into a catalog-driven form only
 * once a second edge setting actually needs one.
 */
export function EdgeSettingsEditor({
  recursive,
  onSave,
  onCancel,
}: {
  recursive: boolean;
  onSave: (next: { recursive: boolean }) => void;
  onCancel: () => void;
}) {
  const [recursiveDraft, setRecursiveDraft] = useState(recursive);

  return (
    <Modal
      open
      title="Path connection settings"
      onCancel={onCancel}
      onOk={() => onSave({ recursive: recursiveDraft })}
      okText="Save"
    >
      <Checkbox
        checked={recursiveDraft}
        onChange={(event) => setRecursiveDraft(event.target.checked)}
      >
        Recursive
      </Checkbox>
      <Typography.Text
        type="secondary"
        style={{ display: "block", marginTop: 4, fontSize: 11 }}
      >
        Include subdirectories when this connection&rsquo;s path destination is
        used.
      </Typography.Text>
    </Modal>
  );
}
