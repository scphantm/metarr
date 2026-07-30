// Package nfo reads and writes Kodi NFO files, the sidecar XML documents that
// hold a media item's metadata on disk. Metarr treats these files as the
// system of record for metadata, so two properties matter more than
// convenience here:
//
//   - Nothing is silently discarded. Tags this package doesn't model are
//     captured verbatim (see UnknownElement) and re-emitted on write, because
//     Kodi and other tools write fields Metarr has no opinion about.
//   - Nothing is silently reinterpreted. Numeric-looking values are kept as
//     strings so a malformed or empty tag round-trips exactly as found rather
//     than being coerced or rejected.
//
// The document structures follow https://kodi.wiki/view/NFO_files/Templates.
// Every struct carries both `xml` and `bson` tags: the same types serialize to
// XML for disk and to BSON when embedded in scan results.
package nfo

import "encoding/xml"

// Movie is the root of a movie.nfo document.
type Movie struct {
	XMLName          xml.Name   `xml:"movie" bson:"-" json:"-"`
	Title            string     `xml:"title,omitempty" bson:"title,omitempty" json:"title,omitempty"`
	OriginalTitle    string     `xml:"originaltitle,omitempty" bson:"original_title,omitempty" json:"original_title,omitempty"`
	SortTitle        string     `xml:"sorttitle,omitempty" bson:"sort_title,omitempty" json:"sort_title,omitempty"`
	Ratings          *Ratings   `xml:"ratings,omitempty" bson:"ratings,omitempty" json:"ratings,omitempty"`
	UserRating       string     `xml:"userrating,omitempty" bson:"user_rating,omitempty" json:"user_rating,omitempty"`
	Top250           string     `xml:"top250,omitempty" bson:"top250,omitempty" json:"top250,omitempty"`
	Outline          string     `xml:"outline,omitempty" bson:"outline,omitempty" json:"outline,omitempty"`
	Plot             string     `xml:"plot,omitempty" bson:"plot,omitempty" json:"plot,omitempty"`
	Tagline          string     `xml:"tagline,omitempty" bson:"tagline,omitempty" json:"tagline,omitempty"`
	Runtime          string     `xml:"runtime,omitempty" bson:"runtime,omitempty" json:"runtime,omitempty"`
	Thumbs           []Thumb    `xml:"thumb,omitempty" bson:"thumbs,omitempty" json:"thumbs,omitempty"`
	Fanart           *Fanart    `xml:"fanart,omitempty" bson:"fanart,omitempty" json:"fanart,omitempty"`
	MPAA             string     `xml:"mpaa,omitempty" bson:"mpaa,omitempty" json:"mpaa,omitempty"`
	PlayCount        string     `xml:"playcount,omitempty" bson:"play_count,omitempty" json:"play_count,omitempty"`
	LastPlayed       string     `xml:"lastplayed,omitempty" bson:"last_played,omitempty" json:"last_played,omitempty"`
	UniqueIDs        []UniqueID `xml:"uniqueid,omitempty" bson:"unique_ids,omitempty" json:"unique_ids,omitempty"`
	ID               string     `xml:"id,omitempty" bson:"id,omitempty" json:"id,omitempty"`
	Genres           []string   `xml:"genre,omitempty" bson:"genres,omitempty" json:"genres,omitempty"`
	Tags             []string   `xml:"tag,omitempty" bson:"tags,omitempty" json:"tags,omitempty"`
	Set              *MovieSet  `xml:"set,omitempty" bson:"set,omitempty" json:"set,omitempty"`
	Countries        []string   `xml:"country,omitempty" bson:"countries,omitempty" json:"countries,omitempty"`
	Credits          []string   `xml:"credits,omitempty" bson:"credits,omitempty" json:"credits,omitempty"`
	Directors        []string   `xml:"director,omitempty" bson:"directors,omitempty" json:"directors,omitempty"`
	Premiered        string     `xml:"premiered,omitempty" bson:"premiered,omitempty" json:"premiered,omitempty"`
	Year             string     `xml:"year,omitempty" bson:"year,omitempty" json:"year,omitempty"`
	Studios          []string   `xml:"studio,omitempty" bson:"studios,omitempty" json:"studios,omitempty"`
	Trailer          string     `xml:"trailer,omitempty" bson:"trailer,omitempty" json:"trailer,omitempty"`
	FileInfo         *FileInfo  `xml:"fileinfo,omitempty" bson:"file_info,omitempty" json:"file_info,omitempty"`
	Actors           []Actor    `xml:"actor,omitempty" bson:"actors,omitempty" json:"actors,omitempty"`
	ShowLinks        []string   `xml:"showlink,omitempty" bson:"show_links,omitempty" json:"show_links,omitempty"`
	Resume           *Resume    `xml:"resume,omitempty" bson:"resume,omitempty" json:"resume,omitempty"`
	DateAdded        string     `xml:"dateadded,omitempty" bson:"date_added,omitempty" json:"date_added,omitempty"`
	OriginalLanguage string     `xml:"originallanguage,omitempty" bson:"original_language,omitempty" json:"original_language,omitempty"`

	Extra []UnknownElement `xml:",any" bson:"extra,omitempty" json:"extra,omitempty"`
}

// TVShow is the root of a tvshow.nfo document.
type TVShow struct {
	XMLName          xml.Name      `xml:"tvshow" bson:"-" json:"-"`
	Title            string        `xml:"title,omitempty" bson:"title,omitempty" json:"title,omitempty"`
	OriginalTitle    string        `xml:"originaltitle,omitempty" bson:"original_title,omitempty" json:"original_title,omitempty"`
	ShowTitle        string        `xml:"showtitle,omitempty" bson:"show_title,omitempty" json:"show_title,omitempty"`
	SortTitle        string        `xml:"sorttitle,omitempty" bson:"sort_title,omitempty" json:"sort_title,omitempty"`
	Ratings          *Ratings      `xml:"ratings,omitempty" bson:"ratings,omitempty" json:"ratings,omitempty"`
	UserRating       string        `xml:"userrating,omitempty" bson:"user_rating,omitempty" json:"user_rating,omitempty"`
	Top250           string        `xml:"top250,omitempty" bson:"top250,omitempty" json:"top250,omitempty"`
	Season           string        `xml:"season,omitempty" bson:"season,omitempty" json:"season,omitempty"`
	Episode          string        `xml:"episode,omitempty" bson:"episode,omitempty" json:"episode,omitempty"`
	DisplayEpisode   string        `xml:"displayepisode,omitempty" bson:"display_episode,omitempty" json:"display_episode,omitempty"`
	DisplaySeason    string        `xml:"displayseason,omitempty" bson:"display_season,omitempty" json:"display_season,omitempty"`
	Outline          string        `xml:"outline,omitempty" bson:"outline,omitempty" json:"outline,omitempty"`
	Plot             string        `xml:"plot,omitempty" bson:"plot,omitempty" json:"plot,omitempty"`
	Tagline          string        `xml:"tagline,omitempty" bson:"tagline,omitempty" json:"tagline,omitempty"`
	Runtime          string        `xml:"runtime,omitempty" bson:"runtime,omitempty" json:"runtime,omitempty"`
	Thumbs           []Thumb       `xml:"thumb,omitempty" bson:"thumbs,omitempty" json:"thumbs,omitempty"`
	Fanart           *Fanart       `xml:"fanart,omitempty" bson:"fanart,omitempty" json:"fanart,omitempty"`
	MPAA             string        `xml:"mpaa,omitempty" bson:"mpaa,omitempty" json:"mpaa,omitempty"`
	PlayCount        string        `xml:"playcount,omitempty" bson:"play_count,omitempty" json:"play_count,omitempty"`
	LastPlayed       string        `xml:"lastplayed,omitempty" bson:"last_played,omitempty" json:"last_played,omitempty"`
	EpisodeGuide     string        `xml:"episodeguide,omitempty" bson:"episode_guide,omitempty" json:"episode_guide,omitempty"`
	UniqueIDs        []UniqueID    `xml:"uniqueid,omitempty" bson:"unique_ids,omitempty" json:"unique_ids,omitempty"`
	ID               string        `xml:"id,omitempty" bson:"id,omitempty" json:"id,omitempty"`
	Genres           []string      `xml:"genre,omitempty" bson:"genres,omitempty" json:"genres,omitempty"`
	Tags             []string      `xml:"tag,omitempty" bson:"tags,omitempty" json:"tags,omitempty"`
	Premiered        string        `xml:"premiered,omitempty" bson:"premiered,omitempty" json:"premiered,omitempty"`
	Year             string        `xml:"year,omitempty" bson:"year,omitempty" json:"year,omitempty"`
	Status           string        `xml:"status,omitempty" bson:"status,omitempty" json:"status,omitempty"`
	Code             string        `xml:"code,omitempty" bson:"code,omitempty" json:"code,omitempty"`
	Aired            string        `xml:"aired,omitempty" bson:"aired,omitempty" json:"aired,omitempty"`
	Studios          []string      `xml:"studio,omitempty" bson:"studios,omitempty" json:"studios,omitempty"`
	Trailer          string        `xml:"trailer,omitempty" bson:"trailer,omitempty" json:"trailer,omitempty"`
	Actors           []Actor       `xml:"actor,omitempty" bson:"actors,omitempty" json:"actors,omitempty"`
	NamedSeasons     []NamedSeason `xml:"namedseason,omitempty" bson:"named_seasons,omitempty" json:"named_seasons,omitempty"`
	SeasonPlots      []SeasonPlot  `xml:"seasonplot,omitempty" bson:"season_plots,omitempty" json:"season_plots,omitempty"`
	Resume           *Resume       `xml:"resume,omitempty" bson:"resume,omitempty" json:"resume,omitempty"`
	DateAdded        string        `xml:"dateadded,omitempty" bson:"date_added,omitempty" json:"date_added,omitempty"`
	OriginalLanguage string        `xml:"originallanguage,omitempty" bson:"original_language,omitempty" json:"original_language,omitempty"`

	Extra []UnknownElement `xml:",any" bson:"extra,omitempty" json:"extra,omitempty"`
}

// EpisodeDetails is the root of an episode NFO document. A single legacy file
// may contain several of these concatenated together; see Parse.
type EpisodeDetails struct {
	XMLName         xml.Name         `xml:"episodedetails" bson:"-" json:"-"`
	Title           string           `xml:"title,omitempty" bson:"title,omitempty" json:"title,omitempty"`
	OriginalTitle   string           `xml:"originaltitle,omitempty" bson:"original_title,omitempty" json:"original_title,omitempty"`
	ShowTitle       string           `xml:"showtitle,omitempty" bson:"show_title,omitempty" json:"show_title,omitempty"`
	Ratings         *Ratings         `xml:"ratings,omitempty" bson:"ratings,omitempty" json:"ratings,omitempty"`
	UserRating      string           `xml:"userrating,omitempty" bson:"user_rating,omitempty" json:"user_rating,omitempty"`
	Top250          string           `xml:"top250,omitempty" bson:"top250,omitempty" json:"top250,omitempty"`
	Season          string           `xml:"season,omitempty" bson:"season,omitempty" json:"season,omitempty"`
	Episode         string           `xml:"episode,omitempty" bson:"episode,omitempty" json:"episode,omitempty"`
	DisplayEpisode  string           `xml:"displayepisode,omitempty" bson:"display_episode,omitempty" json:"display_episode,omitempty"`
	DisplaySeason   string           `xml:"displayseason,omitempty" bson:"display_season,omitempty" json:"display_season,omitempty"`
	Outline         string           `xml:"outline,omitempty" bson:"outline,omitempty" json:"outline,omitempty"`
	Plot            string           `xml:"plot,omitempty" bson:"plot,omitempty" json:"plot,omitempty"`
	Tagline         string           `xml:"tagline,omitempty" bson:"tagline,omitempty" json:"tagline,omitempty"`
	Runtime         string           `xml:"runtime,omitempty" bson:"runtime,omitempty" json:"runtime,omitempty"`
	Thumbs          []Thumb          `xml:"thumb,omitempty" bson:"thumbs,omitempty" json:"thumbs,omitempty"`
	MPAA            string           `xml:"mpaa,omitempty" bson:"mpaa,omitempty" json:"mpaa,omitempty"`
	PlayCount       string           `xml:"playcount,omitempty" bson:"play_count,omitempty" json:"play_count,omitempty"`
	LastPlayed      string           `xml:"lastplayed,omitempty" bson:"last_played,omitempty" json:"last_played,omitempty"`
	UniqueIDs       []UniqueID       `xml:"uniqueid,omitempty" bson:"unique_ids,omitempty" json:"unique_ids,omitempty"`
	ID              string           `xml:"id,omitempty" bson:"id,omitempty" json:"id,omitempty"`
	Genres          []string         `xml:"genre,omitempty" bson:"genres,omitempty" json:"genres,omitempty"`
	Credits         []string         `xml:"credits,omitempty" bson:"credits,omitempty" json:"credits,omitempty"`
	Directors       []string         `xml:"director,omitempty" bson:"directors,omitempty" json:"directors,omitempty"`
	Premiered       string           `xml:"premiered,omitempty" bson:"premiered,omitempty" json:"premiered,omitempty"`
	Year            string           `xml:"year,omitempty" bson:"year,omitempty" json:"year,omitempty"`
	Status          string           `xml:"status,omitempty" bson:"status,omitempty" json:"status,omitempty"`
	Code            string           `xml:"code,omitempty" bson:"code,omitempty" json:"code,omitempty"`
	Aired           string           `xml:"aired,omitempty" bson:"aired,omitempty" json:"aired,omitempty"`
	Studios         []string         `xml:"studio,omitempty" bson:"studios,omitempty" json:"studios,omitempty"`
	Trailer         string           `xml:"trailer,omitempty" bson:"trailer,omitempty" json:"trailer,omitempty"`
	EpisodeBookmark *EpisodeBookmark `xml:"episodebookmark,omitempty" bson:"episode_bookmark,omitempty" json:"episode_bookmark,omitempty"`
	FileInfo        *FileInfo        `xml:"fileinfo,omitempty" bson:"file_info,omitempty" json:"file_info,omitempty"`
	Actors          []Actor          `xml:"actor,omitempty" bson:"actors,omitempty" json:"actors,omitempty"`
	Resume          *Resume          `xml:"resume,omitempty" bson:"resume,omitempty" json:"resume,omitempty"`
	DateAdded       string           `xml:"dateadded,omitempty" bson:"date_added,omitempty" json:"date_added,omitempty"`

	Extra []UnknownElement `xml:",any" bson:"extra,omitempty" json:"extra,omitempty"`
}

// MusicVideo is the root of a musicvideo.nfo document.
type MusicVideo struct {
	XMLName    xml.Name   `xml:"musicvideo" bson:"-" json:"-"`
	Title      string     `xml:"title,omitempty" bson:"title,omitempty" json:"title,omitempty"`
	UserRating string     `xml:"userrating,omitempty" bson:"user_rating,omitempty" json:"user_rating,omitempty"`
	Top250     string     `xml:"top250,omitempty" bson:"top250,omitempty" json:"top250,omitempty"`
	Track      string     `xml:"track,omitempty" bson:"track,omitempty" json:"track,omitempty"`
	Album      string     `xml:"album,omitempty" bson:"album,omitempty" json:"album,omitempty"`
	Outline    string     `xml:"outline,omitempty" bson:"outline,omitempty" json:"outline,omitempty"`
	Plot       string     `xml:"plot,omitempty" bson:"plot,omitempty" json:"plot,omitempty"`
	Tagline    string     `xml:"tagline,omitempty" bson:"tagline,omitempty" json:"tagline,omitempty"`
	Runtime    string     `xml:"runtime,omitempty" bson:"runtime,omitempty" json:"runtime,omitempty"`
	Thumbs     []Thumb    `xml:"thumb,omitempty" bson:"thumbs,omitempty" json:"thumbs,omitempty"`
	MPAA       string     `xml:"mpaa,omitempty" bson:"mpaa,omitempty" json:"mpaa,omitempty"`
	PlayCount  string     `xml:"playcount,omitempty" bson:"play_count,omitempty" json:"play_count,omitempty"`
	LastPlayed string     `xml:"lastplayed,omitempty" bson:"last_played,omitempty" json:"last_played,omitempty"`
	UniqueIDs  []UniqueID `xml:"uniqueid,omitempty" bson:"unique_ids,omitempty" json:"unique_ids,omitempty"`
	ID         string     `xml:"id,omitempty" bson:"id,omitempty" json:"id,omitempty"`
	Genres     []string   `xml:"genre,omitempty" bson:"genres,omitempty" json:"genres,omitempty"`
	Tags       []string   `xml:"tag,omitempty" bson:"tags,omitempty" json:"tags,omitempty"`
	Directors  []string   `xml:"director,omitempty" bson:"directors,omitempty" json:"directors,omitempty"`
	Premiered  string     `xml:"premiered,omitempty" bson:"premiered,omitempty" json:"premiered,omitempty"`
	Year       string     `xml:"year,omitempty" bson:"year,omitempty" json:"year,omitempty"`
	Status     string     `xml:"status,omitempty" bson:"status,omitempty" json:"status,omitempty"`
	Code       string     `xml:"code,omitempty" bson:"code,omitempty" json:"code,omitempty"`
	Aired      string     `xml:"aired,omitempty" bson:"aired,omitempty" json:"aired,omitempty"`
	Studios    []string   `xml:"studio,omitempty" bson:"studios,omitempty" json:"studios,omitempty"`
	Trailer    string     `xml:"trailer,omitempty" bson:"trailer,omitempty" json:"trailer,omitempty"`
	Artists    []string   `xml:"artist,omitempty" bson:"artists,omitempty" json:"artists,omitempty"`
	FileInfo   *FileInfo  `xml:"fileinfo,omitempty" bson:"file_info,omitempty" json:"file_info,omitempty"`
	Actors     []Actor    `xml:"actor,omitempty" bson:"actors,omitempty" json:"actors,omitempty"`
	Resume     *Resume    `xml:"resume,omitempty" bson:"resume,omitempty" json:"resume,omitempty"`
	DateAdded  string     `xml:"dateadded,omitempty" bson:"date_added,omitempty" json:"date_added,omitempty"`

	Extra []UnknownElement `xml:",any" bson:"extra,omitempty" json:"extra,omitempty"`
}

// UniqueID is one scraper-site identifier, e.g.
// <uniqueid type="tmdb" default="true">603</uniqueid>. This is the primary
// source for a media item's external provider links.
type UniqueID struct {
	Type    string `xml:"type,attr,omitempty" bson:"type,omitempty" json:"type,omitempty"`
	Default bool   `xml:"default,attr,omitempty" bson:"default,omitempty" json:"default,omitempty"`
	Value   string `xml:",chardata" bson:"value,omitempty" json:"value,omitempty"`
}

// Ratings wraps the per-site ratings collection.
type Ratings struct {
	Rating []Rating `xml:"rating,omitempty" bson:"rating,omitempty" json:"rating,omitempty"`
}

// Rating is one site's rating.
type Rating struct {
	Name    string `xml:"name,attr,omitempty" bson:"name,omitempty" json:"name,omitempty"`
	Max     string `xml:"max,attr,omitempty" bson:"max,omitempty" json:"max,omitempty"`
	Default bool   `xml:"default,attr,omitempty" bson:"default,omitempty" json:"default,omitempty"`
	Value   string `xml:"value,omitempty" bson:"value,omitempty" json:"value,omitempty"`
	Votes   string `xml:"votes,omitempty" bson:"votes,omitempty" json:"votes,omitempty"`
}

// Actor is one cast member.
type Actor struct {
	Name  string `xml:"name,omitempty" bson:"name,omitempty" json:"name,omitempty"`
	Role  string `xml:"role,omitempty" bson:"role,omitempty" json:"role,omitempty"`
	Order string `xml:"order,omitempty" bson:"order,omitempty" json:"order,omitempty"`
	Thumb string `xml:"thumb,omitempty" bson:"thumb,omitempty" json:"thumb,omitempty"`
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

// Fanart wraps the background artwork references.
type Fanart struct {
	Thumbs []Thumb `xml:"thumb,omitempty" bson:"thumbs,omitempty" json:"thumbs,omitempty"`
}

// MovieSet is the collection a movie belongs to.
type MovieSet struct {
	Name     string `xml:"name,omitempty" bson:"name,omitempty" json:"name,omitempty"`
	Overview string `xml:"overview,omitempty" bson:"overview,omitempty" json:"overview,omitempty"`
}

// Resume is playback position state.
type Resume struct {
	Position string `xml:"position,omitempty" bson:"position,omitempty" json:"position,omitempty"`
	Total    string `xml:"total,omitempty" bson:"total,omitempty" json:"total,omitempty"`
}

// EpisodeBookmark marks the position where an episode's content proper begins.
type EpisodeBookmark struct {
	Position string `xml:"position,omitempty" bson:"position,omitempty" json:"position,omitempty"`
}

// NamedSeason gives a season a display name.
type NamedSeason struct {
	Number string `xml:"number,attr,omitempty" bson:"number,omitempty" json:"number,omitempty"`
	Value  string `xml:",chardata" bson:"value,omitempty" json:"value,omitempty"`
}

// SeasonPlot is a per-season synopsis.
type SeasonPlot struct {
	Number string `xml:"number,attr,omitempty" bson:"number,omitempty" json:"number,omitempty"`
	Value  string `xml:",chardata" bson:"value,omitempty" json:"value,omitempty"`
}

// FileInfo wraps the technical stream description.
type FileInfo struct {
	StreamDetails *StreamDetails `xml:"streamdetails,omitempty" bson:"stream_details,omitempty" json:"stream_details,omitempty"`
}

// StreamDetails describes the media streams inside the video file.
type StreamDetails struct {
	Video    []VideoStream    `xml:"video,omitempty" bson:"video,omitempty" json:"video,omitempty"`
	Audio    []AudioStream    `xml:"audio,omitempty" bson:"audio,omitempty" json:"audio,omitempty"`
	Subtitle []SubtitleStream `xml:"subtitle,omitempty" bson:"subtitle,omitempty" json:"subtitle,omitempty"`
}

// VideoStream is one video track.
type VideoStream struct {
	Codec             string `xml:"codec,omitempty" bson:"codec,omitempty" json:"codec,omitempty"`
	Aspect            string `xml:"aspect,omitempty" bson:"aspect,omitempty" json:"aspect,omitempty"`
	Width             string `xml:"width,omitempty" bson:"width,omitempty" json:"width,omitempty"`
	Height            string `xml:"height,omitempty" bson:"height,omitempty" json:"height,omitempty"`
	DurationInSeconds string `xml:"durationinseconds,omitempty" bson:"duration_in_seconds,omitempty" json:"duration_in_seconds,omitempty"`
	StereoMode        string `xml:"stereomode,omitempty" bson:"stereo_mode,omitempty" json:"stereo_mode,omitempty"`
	HDRType           string `xml:"hdrtype,omitempty" bson:"hdr_type,omitempty" json:"hdr_type,omitempty"`
}

// AudioStream is one audio track.
type AudioStream struct {
	Codec    string `xml:"codec,omitempty" bson:"codec,omitempty" json:"codec,omitempty"`
	Language string `xml:"language,omitempty" bson:"language,omitempty" json:"language,omitempty"`
	Channels string `xml:"channels,omitempty" bson:"channels,omitempty" json:"channels,omitempty"`
}

// SubtitleStream is one embedded subtitle track.
type SubtitleStream struct {
	Language string `xml:"language,omitempty" bson:"language,omitempty" json:"language,omitempty"`
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
