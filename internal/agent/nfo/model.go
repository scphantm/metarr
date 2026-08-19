// Package nfo is the file-format adapter for Kodi NFO sidecars: it reads a
// .nfo file from disk into the canonical metadata.Metadata model, and writes a
// metadata.Metadata back out as a standard .nfo file. It is the first of what
// will be several such format adapters; each one is responsible only for disk
// I/O and the translation between its on-disk format and the metadata model.
//
// The document structs below are an internal implementation detail — one per
// NFO root element — mapping the XML on disk to and from the model. They are
// unexported: callers work in terms of metadata.Metadata, never these types.
// Their layout follows https://kodi.wiki/view/NFO_files/Templates.
package nfo

import (
	"encoding/xml"

	"Metarr/internal/shared/metadata"
)

// movie is the root of a movie.nfo document.
type movie struct {
	XMLName          xml.Name            `xml:"movie"`
	Title            string              `xml:"title,omitempty"`
	OriginalTitle    string              `xml:"originaltitle,omitempty"`
	SortTitle        string              `xml:"sorttitle,omitempty"`
	Ratings          *metadata.Ratings   `xml:"ratings,omitempty"`
	UserRating       string              `xml:"userrating,omitempty"`
	Top250           string              `xml:"top250,omitempty"`
	Outline          string              `xml:"outline,omitempty"`
	Plot             string              `xml:"plot,omitempty"`
	Tagline          string              `xml:"tagline,omitempty"`
	Runtime          string              `xml:"runtime,omitempty"`
	Thumbs           []metadata.Thumb    `xml:"thumb,omitempty"`
	Fanart           *metadata.Fanart    `xml:"fanart,omitempty"`
	MPAA             string              `xml:"mpaa,omitempty"`
	PlayCount        string              `xml:"playcount,omitempty"`
	LastPlayed       string              `xml:"lastplayed,omitempty"`
	UniqueIDs        []metadata.UniqueID `xml:"uniqueid,omitempty"`
	ID               string              `xml:"id,omitempty"`
	Genres           []string            `xml:"genre,omitempty"`
	Tags             []string            `xml:"tag,omitempty"`
	Set              *metadata.MovieSet  `xml:"set,omitempty"`
	Countries        []string            `xml:"country,omitempty"`
	Credits          []string            `xml:"credits,omitempty"`
	Directors        []string            `xml:"director,omitempty"`
	Premiered        string              `xml:"premiered,omitempty"`
	Year             string              `xml:"year,omitempty"`
	Studios          []string            `xml:"studio,omitempty"`
	Trailer          string              `xml:"trailer,omitempty"`
	Actors           []metadata.Actor    `xml:"actor,omitempty"`
	ShowLinks        []string            `xml:"showlink,omitempty"`
	Resume           *metadata.Resume    `xml:"resume,omitempty"`
	DateAdded        string              `xml:"dateadded,omitempty"`
	OriginalLanguage string              `xml:"originallanguage,omitempty"`

	Extra []metadata.UnknownElement `xml:",any"`
}

// tvShow is the root of a tvshow.nfo document.
type tvShow struct {
	XMLName          xml.Name            `xml:"tvshow"`
	Title            string              `xml:"title,omitempty"`
	OriginalTitle    string              `xml:"originaltitle,omitempty"`
	ShowTitle        string              `xml:"showtitle,omitempty"`
	SortTitle        string              `xml:"sorttitle,omitempty"`
	Ratings          *metadata.Ratings   `xml:"ratings,omitempty"`
	UserRating       string              `xml:"userrating,omitempty"`
	Top250           string              `xml:"top250,omitempty"`
	Season           string              `xml:"season,omitempty"`
	Episode          string              `xml:"episode,omitempty"`
	DisplayEpisode   string              `xml:"displayepisode,omitempty"`
	DisplaySeason    string              `xml:"displayseason,omitempty"`
	Outline          string              `xml:"outline,omitempty"`
	Plot             string              `xml:"plot,omitempty"`
	Tagline          string              `xml:"tagline,omitempty"`
	Runtime          string              `xml:"runtime,omitempty"`
	Thumbs           []metadata.Thumb    `xml:"thumb,omitempty"`
	Fanart           *metadata.Fanart    `xml:"fanart,omitempty"`
	MPAA             string              `xml:"mpaa,omitempty"`
	PlayCount        string              `xml:"playcount,omitempty"`
	LastPlayed       string              `xml:"lastplayed,omitempty"`
	EpisodeGuide     string              `xml:"episodeguide,omitempty"`
	UniqueIDs        []metadata.UniqueID `xml:"uniqueid,omitempty"`
	ID               string              `xml:"id,omitempty"`
	Genres           []string            `xml:"genre,omitempty"`
	Tags             []string            `xml:"tag,omitempty"`
	Premiered        string              `xml:"premiered,omitempty"`
	Year             string              `xml:"year,omitempty"`
	Status           string              `xml:"status,omitempty"`
	Code             string              `xml:"code,omitempty"`
	Aired            string              `xml:"aired,omitempty"`
	Studios          []string            `xml:"studio,omitempty"`
	Trailer          string              `xml:"trailer,omitempty"`
	Actors           []metadata.Actor    `xml:"actor,omitempty"`
	Resume           *metadata.Resume    `xml:"resume,omitempty"`
	DateAdded        string              `xml:"dateadded,omitempty"`
	OriginalLanguage string              `xml:"originallanguage,omitempty"`

	Extra []metadata.UnknownElement `xml:",any"`
}

// episodeDetails is the root of an episode NFO document. A single legacy file
// may contain several of these concatenated together; see parse.
type episodeDetails struct {
	XMLName         xml.Name                  `xml:"episodedetails"`
	Title           string                    `xml:"title,omitempty"`
	OriginalTitle   string                    `xml:"originaltitle,omitempty"`
	ShowTitle       string                    `xml:"showtitle,omitempty"`
	Ratings         *metadata.Ratings         `xml:"ratings,omitempty"`
	UserRating      string                    `xml:"userrating,omitempty"`
	Top250          string                    `xml:"top250,omitempty"`
	Season          string                    `xml:"season,omitempty"`
	Episode         string                    `xml:"episode,omitempty"`
	DisplayEpisode  string                    `xml:"displayepisode,omitempty"`
	DisplaySeason   string                    `xml:"displayseason,omitempty"`
	Outline         string                    `xml:"outline,omitempty"`
	Plot            string                    `xml:"plot,omitempty"`
	Tagline         string                    `xml:"tagline,omitempty"`
	Runtime         string                    `xml:"runtime,omitempty"`
	Thumbs          []metadata.Thumb          `xml:"thumb,omitempty"`
	MPAA            string                    `xml:"mpaa,omitempty"`
	PlayCount       string                    `xml:"playcount,omitempty"`
	LastPlayed      string                    `xml:"lastplayed,omitempty"`
	UniqueIDs       []metadata.UniqueID       `xml:"uniqueid,omitempty"`
	ID              string                    `xml:"id,omitempty"`
	Genres          []string                  `xml:"genre,omitempty"`
	Credits         []string                  `xml:"credits,omitempty"`
	Directors       []string                  `xml:"director,omitempty"`
	Premiered       string                    `xml:"premiered,omitempty"`
	Year            string                    `xml:"year,omitempty"`
	Status          string                    `xml:"status,omitempty"`
	Code            string                    `xml:"code,omitempty"`
	Aired           string                    `xml:"aired,omitempty"`
	Studios         []string                  `xml:"studio,omitempty"`
	Trailer         string                    `xml:"trailer,omitempty"`
	EpisodeBookmark *metadata.EpisodeBookmark `xml:"episodebookmark,omitempty"`
	Actors          []metadata.Actor          `xml:"actor,omitempty"`
	Resume          *metadata.Resume          `xml:"resume,omitempty"`
	DateAdded       string                    `xml:"dateadded,omitempty"`

	Extra []metadata.UnknownElement `xml:",any"`
}

// musicVideo is the root of a musicvideo.nfo document.
type musicVideo struct {
	XMLName    xml.Name            `xml:"musicvideo"`
	Title      string              `xml:"title,omitempty"`
	UserRating string              `xml:"userrating,omitempty"`
	Top250     string              `xml:"top250,omitempty"`
	Track      string              `xml:"track,omitempty"`
	Album      string              `xml:"album,omitempty"`
	Outline    string              `xml:"outline,omitempty"`
	Plot       string              `xml:"plot,omitempty"`
	Tagline    string              `xml:"tagline,omitempty"`
	Runtime    string              `xml:"runtime,omitempty"`
	Thumbs     []metadata.Thumb    `xml:"thumb,omitempty"`
	MPAA       string              `xml:"mpaa,omitempty"`
	PlayCount  string              `xml:"playcount,omitempty"`
	LastPlayed string              `xml:"lastplayed,omitempty"`
	UniqueIDs  []metadata.UniqueID `xml:"uniqueid,omitempty"`
	ID         string              `xml:"id,omitempty"`
	Genres     []string            `xml:"genre,omitempty"`
	Tags       []string            `xml:"tag,omitempty"`
	Directors  []string            `xml:"director,omitempty"`
	Premiered  string              `xml:"premiered,omitempty"`
	Year       string              `xml:"year,omitempty"`
	Status     string              `xml:"status,omitempty"`
	Code       string              `xml:"code,omitempty"`
	Aired      string              `xml:"aired,omitempty"`
	Studios    []string            `xml:"studio,omitempty"`
	Trailer    string              `xml:"trailer,omitempty"`
	Artists    []string            `xml:"artist,omitempty"`
	Actors     []metadata.Actor    `xml:"actor,omitempty"`
	Resume     *metadata.Resume    `xml:"resume,omitempty"`
	DateAdded  string              `xml:"dateadded,omitempty"`

	Extra []metadata.UnknownElement `xml:",any"`
}
