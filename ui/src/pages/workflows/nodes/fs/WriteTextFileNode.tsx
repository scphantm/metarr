import type { Node, NodeProps } from "@xyflow/react";

import { NodeShell } from "../shared/NodeShell";
import type { CatalogNodeData } from "../../editorNodeData";

const TYPE_KEY = "fs/writeTextFile";

export function WriteTextFileNode({
  id,
  data,
}: NodeProps<Node<CatalogNodeData>>) {
  return <NodeShell id={id} data={data} typeKey={TYPE_KEY} />;
}
