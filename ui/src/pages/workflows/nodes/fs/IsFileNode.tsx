import type { Node, NodeProps } from "@xyflow/react";

import { NodeShell } from "../shared/NodeShell";
import type { CatalogNodeData } from "../../editorNodeData";

const TYPE_KEY = "fs/isfile";

export function IsFileNode({ id, data }: NodeProps<Node<CatalogNodeData>>) {
  return <NodeShell id={id} data={data} typeKey={TYPE_KEY} />;
}
