/*
 * Closed vocabularies the config screens draw their dropdowns from. Each one
 * is closed on the Go side; these lists let the editor offer exactly those
 * values and nothing else. They are option lists, not model types — the
 * models themselves are the generated messages under ../gen.
 *
 * The epic that generates the models will eventually make these proto enums
 * too (see docs/adr/0005); until then they live here rather than alongside
 * the retired hand-written model mirrors.
 */

export const storageModes = ["cache", "versioned"] as const;
export type StorageMode = (typeof storageModes)[number];

// The closed vocabulary from mediascan.ParseDirectoryType.
export const directoryTypes = ["movie", "tv", "music_video"] as const;
export type DirectoryType = (typeof directoryTypes)[number];

// Closed on the Go side (mediascan.ParseSidecarCategory).
export const sidecarCategories = [
  "image",
  "video_extra",
  "subtitle",
  "metadata",
  "audio",
  "disc_structure",
  "trickplay",
  "unknown",
] as const;
export type SidecarCategory = (typeof sidecarCategories)[number];

// The server's log-level floor (appconfig.LogLevelInfo / LogLevelDebug).
export const logLevels = ["info", "debug"] as const;
export type LogLevel = (typeof logLevels)[number];
