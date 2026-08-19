package metadata

import (
	"encoding/xml"

	"github.com/oapi-codegen/runtime/types"
)

// DocumentKind names the kind of media a metadata record describes. For NFO
// files this is the root element the file turned out to contain.
type DocumentKind string

const (
	KindMovie      DocumentKind = "movie"
	KindTVShow     DocumentKind = "tvshow"
	KindEpisode    DocumentKind = "episodedetails"
	KindMusicVideo DocumentKind = "musicvideo"
	// KindURL is a sidecar holding a bare scraper URL rather than structured
	// data, a form Kodi has historically accepted. There is nothing to parse,
	// but the file is still recognized rather than reported as broken.
	KindURL DocumentKind = "url"
	// KindUnknown is well-formed content whose kind isn't one of the recognized
	// media types.
	KindUnknown DocumentKind = "unknown"
)

// SeasonSummary is a queryable index over a series' media files, not a second
// copy of them.
type SeasonSummary struct {
	SeasonNumber int    `bson:"season_number" json:"season_number"`
	FolderName   string `bson:"folder_name" json:"folder_name"`
	NamedSeason  string `bson:"named_season" json:"named_season"`
	SeasonPlot   string `bson:"season_plot" json:"season_plot"`
}

// Non-media files are described by scanmodel.SidecarFile, which names them
// against the configurable sidecar classification table. The DirectoryFile and
// FileRole types that used to live here, along with the per-file subtitle and
// image attributes, were retired with it.

// VideoAttributes is everything parsed out of a video file's name.
type VideoAttributes struct {
	Title        string `bson:"title,omitempty" json:"title,omitempty"`
	EpisodeTitle string `bson:"episode_title,omitempty" json:"episode_title,omitempty"`
	Year         int    `bson:"year,omitempty" json:"year,omitempty"`

	// SeasonNumber is nil when it could not be resolved, which is different
	// from season zero (specials).
	SeasonNumber   *int       `bson:"season_number,omitempty" json:"season_number,omitempty"`
	EpisodeNumbers []int      `bson:"episode_numbers,omitempty" json:"episode_numbers,omitempty"`
	AirDate        types.Date `bson:"air_date,omitempty" json:"air_date,omitempty"`
	IsSpecial      bool       `bson:"is_special,omitempty" json:"is_special,omitempty"`

	Edition      string `bson:"edition,omitempty" json:"edition,omitempty"`
	VersionLabel string `bson:"version_label,omitempty" json:"version_label,omitempty"`
	StackType    string `bson:"stack_type,omitempty" json:"stack_type,omitempty"`
	StackNumber  int    `bson:"stack_number,omitempty" json:"stack_number,omitempty"`
	ThreeDFormat string `bson:"three_d_format,omitempty" json:"three_d_format,omitempty"`

	// ExtraType and ExtraSource are set only on extra videos.
	ExtraType   string `bson:"extra_type,omitempty" json:"extra_type,omitempty"`
	ExtraSource string `bson:"extra_source,omitempty" json:"extra_source,omitempty"`
}

// Extra sources, recording how a video was identified as an extra.
const (
	ExtraSourceFolder = "folder"
	ExtraSourceSuffix = "suffix"
)

// Metadata records where an NFO sidecar sits in the tree, plus its parsed
// contents. The contents are read during the scan so one pass makes the whole
// library's metadata queryable.
type Metadata struct {
	Scope          string       `bson:"scope" json:"scope"`
	TargetBaseName string       `bson:"target_base_name,omitempty" json:"target_base_name,omitempty"`
	ExternalLinks  []Link       `bson:"external_links" json:"external_links"`
	Kind           DocumentKind `bson:"kind,omitempty" json:"kind,omitempty"`
	Title          string       `bson:"title,omitempty" json:"title,omitempty"`
	OriginalTitle  string       `bson:"original_title,omitempty" json:"original_title,omitempty"`
	Ratings        *Ratings     `bson:"ratings,omitempty" json:"ratings,omitempty"`
	UserRating     string       `bson:"user_rating,omitempty" json:"user_rating,omitempty"`
	Top250         string       `bson:"top250,omitempty" json:"top250,omitempty"`
	Outline        string       `bson:"outline,omitempty" json:"outline,omitempty"`
	Plot           string       `bson:"plot,omitempty" json:"plot,omitempty"`
	Tagline        string       `bson:"tagline,omitempty" json:"tagline,omitempty"`
	Runtime        string       `bson:"runtime,omitempty" json:"runtime,omitempty"`
	Thumbs         []Thumb      `bson:"thumbs,omitempty" json:"thumbs,omitempty"`
	MPAA           string       `bson:"mpaa,omitempty" json:"mpaa,omitempty"`
	PlayCount      int          `bson:"play_count,omitempty" json:"play_count,omitempty"`
	LastPlayed     string       `bson:"last_played,omitempty" json:"last_played,omitempty"`
	ID             string       `bson:"id,omitempty" json:"id,omitempty"`
	Genres         []string     `bson:"genres,omitempty" json:"genres,omitempty"`
	Tags           []string     `bson:"tags,omitempty" json:"tags,omitempty"`
	Premiered      types.Date   `bson:"premiered,omitempty" json:"premiered,omitempty"`
	Year           int          `bson:"year,omitempty" json:"year,omitempty"`
	Status         string       `bson:"status,omitempty" json:"status,omitempty"`
	Code           string       `bson:"code,omitempty" json:"code,omitempty"`
	Aired          string       `bson:"aired,omitempty" json:"aired,omitempty"`
	Studios        []string     `bson:"studios,omitempty" json:"studios,omitempty"`
	Trailer        string       `bson:"trailer,omitempty" json:"trailer,omitempty"`

	Resume    *Resume          `bson:"resume,omitempty" json:"resume,omitempty"`
	DateAdded types.Date       `bson:"date_added,omitempty" json:"date_added,omitempty"`
	Extra     []UnknownElement `bson:"extra,omitempty" json:"extra,omitempty"`

	Movie      *MovieFields      `bson:"movie,omitempty" json:"movie,omitempty"`
	Episode    *EpisodeFields    `bson:"episode,omitempty" json:"episode,omitempty"`
	MusicVideo *MusicVideoFields `bson:"musicvideo,omitempty" json:"musicvideo,omitempty"`
	TVShow     *TVShowFields     `bson:"tvshow,omitempty" json:"tvshow,omitempty"`
	CastCrew   *CastCrew         `bson:"cast_crew,omitempty" json:"cast_crew,omitempty"`

	// ParseError records why an NFO could not be read. A malformed sidecar is
	// reported here rather than failing the scan.
	ParseError string `bson:"parse_error,omitempty" json:"parse_error,omitempty"`
}

type CastCrew struct {
	Directors []string `bson:"directors,omitempty" json:"directors,omitempty"`
	Actors    []Actor  `bson:"actors,omitempty" json:"actors,omitempty"`
	Credits   []string `bson:"credits,omitempty" json:"credits,omitempty"`
}

// Resume is playback position state.
type Resume struct {
	Position string `xml:"position,omitempty" bson:"position,omitempty" json:"position,omitempty"`
	Total    string `xml:"total,omitempty" bson:"total,omitempty" json:"total,omitempty"`
}

// UnknownElement captures a tag this package doesn't model, so it survives a
// read/write round trip instead of being silently dropped. Kodi and other
// tools write fields Metarr has no opinion about, and NFO files are the
// system of record — discarding them would destroy user metadata.
//
// Name mirrors XMLName for storage, since xml.Name itself is not meaningful
// once the document has left XML form.
type UnknownElement struct {
	XMLName  xml.Name   `bson:"-" json:"-"`
	Name     string     `xml:"-" bson:"name" json:"name"`
	Attrs    []xml.Attr `xml:",any,attr" bson:"attrs,omitempty" json:"attrs,omitempty"`
	InnerXML string     `xml:",innerxml" bson:"inner_xml,omitempty" json:"inner_xml,omitempty"`
}

// Actor is one cast member.
type Actor struct {
	Name  string `xml:"name,omitempty" bson:"name,omitempty" json:"name,omitempty"`
	Role  string `xml:"role,omitempty" bson:"role,omitempty" json:"role,omitempty"`
	Order string `xml:"order,omitempty" bson:"order,omitempty" json:"order,omitempty"`
	Thumb string `xml:"thumb,omitempty" bson:"thumb,omitempty" json:"thumb,omitempty"`
}

// Ratings wraps the per-site ratings collection.
type Ratings struct {
	Rating []Rating `xml:"rating,omitempty" bson:"rating,omitempty" json:"rating,omitempty"`
}

// UniqueID is one scraper-site identifier, e.g.
// <uniqueid type="tmdb" default="true">603</uniqueid>. This is the primary
// source for a media item's external provider links.
type UniqueID struct {
	Type    string `xml:"type,attr,omitempty" bson:"type,omitempty" json:"type,omitempty"`
	Default bool   `xml:"default,attr,omitempty" bson:"default,omitempty" json:"default,omitempty"`
	Value   string `xml:",chardata" bson:"value,omitempty" json:"value,omitempty"`
}

// Rating is one site's rating.
type Rating struct {
	Name    string `xml:"name,attr,omitempty" bson:"name,omitempty" json:"name,omitempty"`
	Max     string `xml:"max,attr,omitempty" bson:"max,omitempty" json:"max,omitempty"`
	Default bool   `xml:"default,attr,omitempty" bson:"default,omitempty" json:"default,omitempty"`
	Value   string `xml:"value,omitempty" bson:"value,omitempty" json:"value,omitempty"`
	Votes   string `xml:"votes,omitempty" bson:"votes,omitempty" json:"votes,omitempty"`
}

type MovieFields struct {
	SortTitle        string    `bson:"sort_title,omitempty" json:"sort_title,omitempty"`
	Fanart           *Fanart   `bson:"fanart,omitempty" json:"fanart,omitempty"`
	Set              *MovieSet `bson:"set,omitempty" json:"set,omitempty"`
	Countries        []string  `bson:"countries,omitempty" json:"countries,omitempty"`
	ShowLinks        []string  `bson:"show_links,omitempty" json:"show_links,omitempty"`
	OriginalLanguage string    `bson:"original_language,omitempty" json:"original_language,omitempty"`
}

type TVShowFields struct {
	Seasons []SeasonSummary `bson:"seasons,omitempty" json:"seasons,omitempty"`
}

// Fanart wraps the background artwork references.
type Fanart struct {
	Thumbs []Thumb `xml:"thumb,omitempty" bson:"thumbs,omitempty" json:"thumbs,omitempty"`
}

// Thumb is an artwork reference. The type and season attributes only appear in
// tvshow documents, where one file describes artwork for several seasons.
type Thumb struct {
	Aspect  string `xml:"aspect,attr,omitempty" bson:"aspect,omitempty" json:"aspect,omitempty"`
	Preview string `xml:"preview,attr,omitempty" bson:"preview,omitempty" json:"preview,omitempty"`
	Type    string `xml:"type,attr,omitempty" bson:"type,omitempty" json:"type,omitempty"`
	Season  string `xml:"season,attr,omitempty" bson:"season,omitempty" json:"season,omitempty"`
	Value   string `xml:",chardata" bson:"value,omitempty" json:"value,omitempty"`
}

// MovieSet is the collection a movie belongs to.
type MovieSet struct {
	Name     string `xml:"name,omitempty" bson:"name,omitempty" json:"name,omitempty"`
	Overview string `xml:"overview,omitempty" bson:"overview,omitempty" json:"overview,omitempty"`
}

// EpisodeFields holds the fields unique to an episode document.
type EpisodeFields struct {
	ShowTitle       string           `bson:"show_title,omitempty" json:"show_title,omitempty"`
	Season          string           `bson:"season,omitempty" json:"season,omitempty"`
	Episode         string           `bson:"episode,omitempty" json:"episode,omitempty"`
	DisplayEpisode  string           `bson:"display_episode,omitempty" json:"display_episode,omitempty"`
	DisplaySeason   string           `bson:"display_season,omitempty" json:"display_season,omitempty"`
	EpisodeBookmark *EpisodeBookmark `bson:"episode_bookmark,omitempty" json:"episode_bookmark,omitempty"`
}

// EpisodeBookmark marks the position where an episode's content proper begins.
type EpisodeBookmark struct {
	Position string `xml:"position,omitempty" bson:"position,omitempty" json:"position,omitempty"`
}

// MusicVideoFields holds the fields unique to a music-video document.
type MusicVideoFields struct {
	Track   string   `bson:"track,omitempty" json:"track,omitempty"`
	Album   string   `bson:"album,omitempty" json:"album,omitempty"`
	Artists []string `bson:"artists,omitempty" json:"artists,omitempty"`
}

// Scopes describing which level of the tree a sidecar applies to.
const (
	ScopeDirectory = "directory"
	ScopeSeason    = "season"
	ScopeVideo     = "video"
)
