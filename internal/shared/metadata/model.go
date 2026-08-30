// Package metadata models the parsed contents of a Kodi NFO sidecar and the
// attributes read from a media file's name.
//
// The model types are aliases to their generated metarr.v1 messages: proto
// is the single definition for a model that crosses a language boundary, and
// this one is stored as part of a scan record. See docs/adr/0005. The nfo
// package keeps its own XML-tagged structs for the on-disk format and
// translates to and from these; XML round-tripping is a serialization
// concern these messages do not carry.
package metadata

import (
	metarrv1 "Metarr/internal/genproto/metarr/v1"
)

// DocumentKind names the kind of media a metadata record describes. For NFO
// files this is the root element the file turned out to contain. It is a
// free string on the message; these are the values it takes.
const (
	KindMovie      = "movie"
	KindTVShow     = "tvshow"
	KindEpisode    = "episodedetails"
	KindMusicVideo = "musicvideo"
	// KindURL is a sidecar holding a bare scraper URL rather than structured
	// data. There is nothing to parse, but the file is still recognized.
	KindURL = "url"
	// KindUnknown is well-formed content whose kind isn't one of the
	// recognized media types.
	KindUnknown = "unknown"
)

// Scopes describing which level of the tree a sidecar applies to.
const (
	ScopeDirectory = "directory"
	ScopeSeason    = "season"
	ScopeVideo     = "video"
)

// Extra sources, recording how a video was identified as an extra.
const (
	ExtraSourceFolder = "folder"
	ExtraSourceSuffix = "suffix"
)

// The metadata model. Every type here is an alias to the generated message
// that defines it — see the package doc.
type (
	Metadata         = metarrv1.Metadata
	VideoAttributes  = metarrv1.VideoAttributes
	SeasonSummary    = metarrv1.SeasonSummary
	CastCrew         = metarrv1.CastCrew
	MovieFields      = metarrv1.MovieFields
	TVShowFields     = metarrv1.TVShowFields
	EpisodeFields    = metarrv1.EpisodeFields
	MusicVideoFields = metarrv1.MusicVideoFields
	Link             = metarrv1.Link
	UniqueID         = metarrv1.UniqueId
	Ratings          = metarrv1.Ratings
	Rating           = metarrv1.Rating
	Thumb            = metarrv1.Thumb
	Fanart           = metarrv1.Fanart
	Actor            = metarrv1.Actor
	Resume           = metarrv1.Resume
	EpisodeBookmark  = metarrv1.EpisodeBookmark
	MovieSet         = metarrv1.MovieSet
	UnknownElement   = metarrv1.UnknownElement
	XMLAttr          = metarrv1.XmlAttr
)
