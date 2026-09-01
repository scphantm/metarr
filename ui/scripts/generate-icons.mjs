// One-off codegen: renders a handful of react-icons components to static
// SVG and writes them out as mask-image CSS classes. Run manually
// (`node scripts/generate-icons.mjs`) whenever an icon needs adding or
// swapping — never part of `npm run build`/`dev`/lint. react-icons is a
// devDependency used only here; nothing under src/ may import it (see
// eslint.config.js's no-restricted-imports rule).
//
// Class names are semantic (what the icon means), not icon-set-derived —
// swapping FaFolder for a different glyph later is a re-run + CSS diff,
// never a call-site change across the app.
import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import {
  FaList,
  FaRegThumbsUp,
  FaCopy,
  FaNoteSticky,
  FaFolderTree,
} from 'react-icons/fa6'
import { FaFileAlt } from 'react-icons/fa'
import { PiFolders, PiFiles, PiTreeView } from 'react-icons/pi'
import { GoIterations, GoFileDirectoryFill } from 'react-icons/go'
import {
  MdNotificationImportant,
  MdInput,
  MdRocketLaunch,
  MdSaveAlt,
  MdError,
  MdInventory,
  MdFormatColorFill,
  MdOutlineJoinFull,
  MdOutlineConfirmationNumber,
  MdNearbyError,
  MdOutput,
  MdCheckCircle,
  MdReport,
} from 'react-icons/md'
import {
  TbBinaryTreeFilled,
  TbArrowsJoin,
  TbDeviceImacQuestion,
  TbFileDatabase,
} from 'react-icons/tb'
import { BiSolidMessageSquareError } from 'react-icons/bi'
import { GiMagicSwirl, GiLightningBranches, GiMatchTip } from 'react-icons/gi'
import {
  RiLoopRightAiFill,
  RiDeleteBack2Fill,
  RiTranslate,
} from 'react-icons/ri'
import { HiCollection } from 'react-icons/hi'
import { FcParallelTasks } from 'react-icons/fc'
import {
  BsStoplights,
  BsListColumnsReverse,
  BsFillSignpostSplitFill,
  BsDatabaseFillUp,
  BsDatabaseFillCheck,
} from 'react-icons/bs'
import { ImMoveDown } from 'react-icons/im'
import { SiReadme } from 'react-icons/si'
import { TfiWrite } from 'react-icons/tfi'
import { LuGalleryThumbnails } from 'react-icons/lu'
import { CgCap } from 'react-icons/cg'

const icons = [
  { className: 'icon-directory', Component: GoFileDirectoryFill },
  { className: 'icon-file', Component: FaFileAlt },
  { className: 'icon-list', Component: FaList },
  { className: 'icon-list-directory', Component: PiFolders },
  { className: 'icon-list-file', Component: PiFiles },
  { className: 'icon-iterate', Component: GoIterations },
  { className: 'icon-tree', Component: PiTreeView },
  { className: 'icon-media', Component: TbFileDatabase },
  { className: 'icon-type-unsafe', Component: MdNotificationImportant },
  { className: 'icon-recursive', Component: FaFolderTree },
  // Control-flow port icons — keyed by port name (in/error/next/yes/no),
  // not by data Type, so these live alongside the data-type icons above but
  // are looked up differently — see lib/typeIcons.ts's
  // iconClassForControlPort.
  { className: 'icon-control-in', Component: MdInput },
  { className: 'icon-control-error', Component: MdNearbyError },
  { className: 'icon-control-next', Component: MdOutput },
  { className: 'icon-control-yes', Component: MdCheckCircle },
  { className: 'icon-control-no', Component: MdReport },
  // Node-shape icons (nodeVisual.ts's iconVisual) — same mask-image
  // mechanism as the data-type icons above, just filling a node's shape
  // box instead of a handle/edge endpoint. See index.css's .shape-icon.
  { className: 'shape-decision', Component: TbBinaryTreeFilled },
  { className: 'shape-input', Component: MdInput },
  { className: 'shape-start', Component: MdRocketLaunch },
  { className: 'shape-write-changes', Component: MdSaveAlt },
  { className: 'shape-error-output', Component: BiSolidMessageSquareError },
  { className: 'shape-trickplay', Component: GiMagicSwirl },
  { className: 'shape-for-each', Component: RiLoopRightAiFill },
  { className: 'shape-collect', Component: HiCollection },
  { className: 'shape-parallel', Component: FcParallelTasks },
  { className: 'shape-join', Component: TbArrowsJoin },
  { className: 'shape-break', Component: BsStoplights },
  { className: 'shape-end', Component: FaRegThumbsUp },
  { className: 'shape-fail', Component: MdError },
  { className: 'shape-list-directory', Component: BsListColumnsReverse },
  { className: 'shape-move-file', Component: ImMoveDown },
  { className: 'shape-copy-file', Component: FaCopy },
  { className: 'shape-delete-file', Component: RiDeleteBack2Fill },
  { className: 'shape-make-directory', Component: GiLightningBranches },
  { className: 'shape-file-size', Component: TbDeviceImacQuestion },
  { className: 'shape-read-text-file', Component: SiReadme },
  { className: 'shape-write-text-file', Component: TfiWrite },
  { className: 'shape-probe', Component: MdInventory },
  { className: 'shape-transcode', Component: RiTranslate },
  { className: 'shape-extract-stream', Component: BsFillSignpostSplitFill },
  { className: 'shape-generate-thumbnail', Component: LuGalleryThumbnails },
  { className: 'shape-nfo-read', Component: BsDatabaseFillUp },
  { className: 'shape-nfo-write', Component: BsDatabaseFillCheck },
  { className: 'shape-format', Component: MdFormatColorFill },
  { className: 'shape-regex-match', Component: GiMatchTip },
  { className: 'shape-concat', Component: MdOutlineJoinFull },
  { className: 'shape-parse-number', Component: MdOutlineConfirmationNumber },
  { className: 'shape-trim', Component: CgCap },
  { className: 'shape-note', Component: FaNoteSticky },
]

function toMaskDataURI(svgMarkup) {
  const encoded = encodeURIComponent(svgMarkup)
    .replace(/'/g, '%27')
    .replace(/"/g, '%22')
  return `url("data:image/svg+xml,${encoded}")`
}

let css = `/*
 * Generated by ui/scripts/generate-icons.mjs from react-icons (fa6, pi, go,
 * md, tb, bi, gi, ri, hi, fc, bs, im, si, tfi, lu, cg icon sets) — react-icons is a devDependency, never imported at runtime
 * (see eslint.config.js's no-restricted-imports rule). Each class masks a
 * background with the icon's silhouette so it recolors for free via
 * currentColor wherever it's placed — no per-color asset needed.
 *
 * Hand-editable after generation, but prefer re-running the script and
 * diff-reviewing the result over hand-writing a new data URI.
 */

`

for (const { className, Component } of icons) {
  const markup = renderToStaticMarkup(React.createElement(Component))
  const dataURI = toMaskDataURI(markup)
  css += `.${className} {
  mask-image: ${dataURI};
  mask-repeat: no-repeat;
  mask-position: center;
  mask-size: contain;
  background-color: currentcolor;
  display: inline-block;
}

`
}

const outPath = fileURLToPath(
  new URL('../src/lib/icons/typeIcons.css', import.meta.url),
)
writeFileSync(outPath, css)
console.log(`Wrote ${icons.length} icon classes to ${outPath}`)
