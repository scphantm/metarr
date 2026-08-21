import type { ComponentType } from 'react'
import type { Node, NodeProps } from '@xyflow/react'

import type { CatalogNodeData, UnknownNodeData } from '../catalogTypes'
import { BreakNode } from './core/BreakNode'
import { CheckFlowVariableNode } from './core/CheckFlowVariableNode'
import { CollectNode } from './core/CollectNode'
import { EndNode } from './core/EndNode'
import { ErrorOutputNode } from './core/ErrorOutputNode'
import { FailNode } from './core/FailNode'
import { ForEachNode } from './core/ForEachNode'
import { InputPathNode } from './core/InputPathNode'
import { JoinNode } from './core/JoinNode'
import { NotesNode } from './core/NotesNode'
import { ParallelNode } from './core/ParallelNode'
import { StartNode } from './core/StartNode'
import { TrickplayNode } from './core/TrickplayNode'
import { WriteChangesNode } from './core/WriteChangesNode'
import { CopyFileNode } from './fs/CopyFileNode'
import { DeleteFileNode } from './fs/DeleteFileNode'
import { ExistsNode } from './fs/ExistsNode'
import { FileSizeNode } from './fs/FileSizeNode'
import { ListDirectoryNode } from './fs/ListDirectoryNode'
import { MakeDirectoryNode } from './fs/MakeDirectoryNode'
import { MoveFileNode } from './fs/MoveFileNode'
import { ReadTextFileNode } from './fs/ReadTextFileNode'
import { WriteTextFileNode } from './fs/WriteTextFileNode'
import { ExtractStreamNode } from './media/ExtractStreamNode'
import { GenerateThumbnailNode } from './media/GenerateThumbnailNode'
import { ProbeNode } from './media/ProbeNode'
import { TranscodeNode } from './media/TranscodeNode'
import { ReadNode as NfoReadNode } from './nfo/ReadNode'
import { WriteNode as NfoWriteNode } from './nfo/WriteNode'
import { UnknownNode } from './shared/UnknownNode'
import { ConcatNode } from './string/ConcatNode'
import { FormatNode } from './string/FormatNode'
import { ParseNumberNode } from './string/ParseNumberNode'
import { RegexMatchNode } from './string/RegexMatchNode'
import { TrimNode } from './string/TrimNode'
import {IsDirNode} from "@/pages/workflows/nodes/fs/IsDirNode.tsx";
import {IsFileNode} from "@/pages/workflows/nodes/fs/IsFileNode.tsx";

/*
 * Every catalog entry gets its own registered React Flow node component —
 * see design.md §9 and CLAUDE.md's node design pattern. This map is the one
 * place that has to be kept in sync with catalog.json by hand (a mismatch
 * either way is harmless: an extra registry entry is simply never used, a
 * missing one falls back to UnknownNode — see graphAdapter.toRFNode). Keys
 * are written as literal strings rather than importing each file's own
 * TYPE_KEY constant, so those constants can stay unexported — keeping every
 * node file down to a single component export.
 */
export const nodeTypes: Record<string, ComponentType<NodeProps<Node<CatalogNodeData>>>> = {
  'core/start': StartNode,
  'core/inputPath': InputPathNode,
  'core/writeChanges': WriteChangesNode,
  'core/errorOutput': ErrorOutputNode,
  'core/note': NotesNode,
  'core/checkFlowVariable': CheckFlowVariableNode,
  'core/trickplay': TrickplayNode,
  'core/forEach': ForEachNode,
  'core/collect': CollectNode,
  'core/parallel': ParallelNode,
  'core/join': JoinNode,
  'core/break': BreakNode,
  'core/end': EndNode,
  'core/fail': FailNode,

  'fs/listDirectory': ListDirectoryNode,
  'fs/moveFile': MoveFileNode,
  'fs/copyFile': CopyFileNode,
  'fs/deleteFile': DeleteFileNode,
  'fs/exists': ExistsNode,
  'fs/isdir': IsDirNode,
  'fs/isfile': IsFileNode,
  'fs/makeDirectory': MakeDirectoryNode,
  'fs/fileSize': FileSizeNode,
  'fs/readTextFile': ReadTextFileNode,
  'fs/writeTextFile': WriteTextFileNode,

  'media/probe': ProbeNode,
  'media/transcode': TranscodeNode,
  'media/extractStream': ExtractStreamNode,
  'media/generateThumbnail': GenerateThumbnailNode,

  'nfo/read': NfoReadNode,
  'nfo/write': NfoWriteNode,

  'string/format': FormatNode,
  'string/regexMatch': RegexMatchNode,
  'string/concat': ConcatNode,
  'string/parseNumber': ParseNumberNode,
  'string/trim': TrimNode,
}

// unknownNode is the catalog-drift fallback (graphAdapter.UNKNOWN_NODE_TYPE)
// — kept out of `nodeTypes` above since it takes UnknownNodeData, not
// CatalogNodeData, and registered separately wherever `nodeTypes` is spread
// into React Flow's own `nodeTypes` prop.
export const unknownNodeType: Record<string, ComponentType<NodeProps<Node<UnknownNodeData>>>> = {
  unknownNode: UnknownNode,
}

export const registeredTypes: ReadonlySet<string> = new Set(Object.keys(nodeTypes))
