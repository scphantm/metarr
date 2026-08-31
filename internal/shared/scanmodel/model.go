package scanmodel

// This is the result of one directory scan. Its purpose is to record the top
// level metadata structure which is stored in the database and used to populate
// the nfo files on write. It is also used to identify all the sidecar files in
// the directory.
//
// The record types are aliases to their generated metarr.v1 messages: proto is
// the single definition for a model that crosses a language boundary, and this
// one is stored in the local_directory collection and served by the
// LocalDirectoryService. See docs/adr/0005. Persistence goes through
// MarshalStored / UnmarshalStored (protojson) rather than bson struct tags, so
// the stored field names stay snake_case and the document is still readable
// directly in the collection.

import (
	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// The scan record model. Every type here is an alias to the generated message
// that defines it — see the package doc.
type (
	TVSeries       = metarrv1.TVSeries
	TVSeason       = metarrv1.TVSeason
	MediaFile      = metarrv1.MediaFile
	SidecarFile    = metarrv1.SidecarFile
	TrickplayInfo  = metarrv1.TrickplayInfo
	FileInfo       = metarrv1.FileInfo
	StreamDetails  = metarrv1.StreamDetails
	VideoStream    = metarrv1.VideoStream
	AudioStream    = metarrv1.AudioStream
	SubtitleStream = metarrv1.SubtitleStream
	FileStat       = metarrv1.FileStat
	ImageInfo      = metarrv1.ImageInfo
)

// ScanResult is everything one directory scan produced. It is the transient
// container the scan pipeline assembles and the ingestion path works in — it is
// never stored or transmitted field-for-field, so it stays a plain struct
// rather than a generated message.
type ScanResult struct {
	Directory  *TVSeries    `json:"directory"`
	MediaFiles []*MediaFile `json:"media_files"`
}
