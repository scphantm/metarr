package nfo

import (
	"strconv"

	"Metarr/internal/shared/metadata"
)

// This file translates between the on-disk XML document structs and the
// canonical metadata.Metadata model, in both directions. toMetadata is the read
// path; documentFromMetadata is the write path. The metadata package owns the
// shared value types (Ratings, Thumb, UniqueID, Resume, Actor, Fanart,
// MovieSet, EpisodeBookmark), so those transfer by direct assignment; the
// numeric tags kept as strings on disk, and the date tags, are converted.

// toMetadata projects a parsed document into the canonical model. A legacy
// multi-episode file is represented by its first episode. A nil, unrecognized,
// or URL-only document yields an empty record carrying just its kind, so
// callers never have to nil-check the result.
func (doc *document) toMetadata() *metadata.Metadata {
	switch {
	case doc == nil:
		return &metadata.Metadata{}
	case doc.Movie != nil:
		return doc.Movie.toMetadata()
	case doc.TVShow != nil:
		return doc.TVShow.toMetadata()
	case len(doc.Episodes) > 0:
		return doc.Episodes[0].toMetadata()
	case doc.MusicVideo != nil:
		return doc.MusicVideo.toMetadata()
	default:
		return &metadata.Metadata{Kind: doc.Kind}
	}
}

func (m *movie) toMetadata() *metadata.Metadata {
	return &metadata.Metadata{
		Kind:          metadata.KindMovie,
		Title:         m.Title,
		OriginalTitle: m.OriginalTitle,
		Ratings:       m.Ratings,
		UserRating:    m.UserRating,
		Top250:        m.Top250,
		Outline:       m.Outline,
		Plot:          m.Plot,
		Tagline:       m.Tagline,
		Runtime:       m.Runtime,
		Thumbs:        m.Thumbs,
		MPAA:          m.MPAA,
		PlayCount:     atoiOrZero(m.PlayCount),
		LastPlayed:    m.LastPlayed,
		ExternalLinks: metadata.LinksFromUniqueIDs(m.UniqueIDs),
		ID:            m.ID,
		Genres:        m.Genres,
		Tags:          m.Tags,
		Premiered:     metadata.ParseDate(m.Premiered),
		Year:          atoiOrZero(m.Year),
		Studios:       m.Studios,
		Trailer:       m.Trailer,
		Resume:        m.Resume,
		DateAdded:     metadata.ParseDate(m.DateAdded),
		Extra:         m.Extra,
		CastCrew:      castCrew(m.Directors, m.Actors, m.Credits),
		Movie: &metadata.MovieFields{
			SortTitle:        m.SortTitle,
			Fanart:           m.Fanart,
			Set:              m.Set,
			Countries:        m.Countries,
			ShowLinks:        m.ShowLinks,
			OriginalLanguage: m.OriginalLanguage,
		},
	}
}

func (s *tvShow) toMetadata() *metadata.Metadata {
	return &metadata.Metadata{
		Kind:          metadata.KindTVShow,
		Title:         s.Title,
		OriginalTitle: s.OriginalTitle,
		Ratings:       s.Ratings,
		UserRating:    s.UserRating,
		Top250:        s.Top250,
		Outline:       s.Outline,
		Plot:          s.Plot,
		Tagline:       s.Tagline,
		Runtime:       s.Runtime,
		Thumbs:        s.Thumbs,
		MPAA:          s.MPAA,
		PlayCount:     atoiOrZero(s.PlayCount),
		LastPlayed:    s.LastPlayed,
		ExternalLinks: metadata.LinksFromUniqueIDs(s.UniqueIDs),
		ID:            s.ID,
		Genres:        s.Genres,
		Tags:          s.Tags,
		Premiered:     metadata.ParseDate(s.Premiered),
		Year:          atoiOrZero(s.Year),
		Status:        s.Status,
		Code:          s.Code,
		Aired:         s.Aired,
		Studios:       s.Studios,
		Trailer:       s.Trailer,
		Resume:        s.Resume,
		DateAdded:     metadata.ParseDate(s.DateAdded),
		Extra:         s.Extra,
		CastCrew:      castCrew(nil, s.Actors, nil),
		TVShow:        &metadata.TVShowFields{},
	}
}

func (e *episodeDetails) toMetadata() *metadata.Metadata {
	return &metadata.Metadata{
		Kind:          metadata.KindEpisode,
		Title:         e.Title,
		OriginalTitle: e.OriginalTitle,
		Ratings:       e.Ratings,
		UserRating:    e.UserRating,
		Top250:        e.Top250,
		Outline:       e.Outline,
		Plot:          e.Plot,
		Tagline:       e.Tagline,
		Runtime:       e.Runtime,
		Thumbs:        e.Thumbs,
		MPAA:          e.MPAA,
		PlayCount:     atoiOrZero(e.PlayCount),
		LastPlayed:    e.LastPlayed,
		ExternalLinks: metadata.LinksFromUniqueIDs(e.UniqueIDs),
		ID:            e.ID,
		Genres:        e.Genres,
		Premiered:     metadata.ParseDate(e.Premiered),
		Year:          atoiOrZero(e.Year),
		Status:        e.Status,
		Code:          e.Code,
		Aired:         e.Aired,
		Studios:       e.Studios,
		Trailer:       e.Trailer,
		Resume:        e.Resume,
		DateAdded:     metadata.ParseDate(e.DateAdded),
		Extra:         e.Extra,
		CastCrew:      castCrew(e.Directors, e.Actors, e.Credits),
		Episode: &metadata.EpisodeFields{
			ShowTitle:       e.ShowTitle,
			Season:          e.Season,
			Episode:         e.Episode,
			DisplayEpisode:  e.DisplayEpisode,
			DisplaySeason:   e.DisplaySeason,
			EpisodeBookmark: e.EpisodeBookmark,
		},
	}
}

func (v *musicVideo) toMetadata() *metadata.Metadata {
	return &metadata.Metadata{
		Kind:          metadata.KindMusicVideo,
		Title:         v.Title,
		UserRating:    v.UserRating,
		Top250:        v.Top250,
		Outline:       v.Outline,
		Plot:          v.Plot,
		Tagline:       v.Tagline,
		Runtime:       v.Runtime,
		Thumbs:        v.Thumbs,
		MPAA:          v.MPAA,
		PlayCount:     atoiOrZero(v.PlayCount),
		LastPlayed:    v.LastPlayed,
		ExternalLinks: metadata.LinksFromUniqueIDs(v.UniqueIDs),
		ID:            v.ID,
		Genres:        v.Genres,
		Tags:          v.Tags,
		Premiered:     metadata.ParseDate(v.Premiered),
		Year:          atoiOrZero(v.Year),
		Status:        v.Status,
		Code:          v.Code,
		Aired:         v.Aired,
		Studios:       v.Studios,
		Trailer:       v.Trailer,
		Resume:        v.Resume,
		DateAdded:     metadata.ParseDate(v.DateAdded),
		Extra:         v.Extra,
		CastCrew:      castCrew(v.Directors, v.Actors, nil),
		MusicVideo: &metadata.MusicVideoFields{
			Track:   v.Track,
			Album:   v.Album,
			Artists: v.Artists,
		},
	}
}

// documentFromMetadata builds the on-disk document for a metadata record,
// choosing the root element from its Kind. An unrecognized kind yields a nil
// document, which marshal rejects.
func documentFromMetadata(m *metadata.Metadata) *document {
	if m == nil {
		return nil
	}
	switch m.Kind {
	case metadata.KindMovie:
		return &document{Kind: m.Kind, Movie: movieFromMetadata(m)}
	case metadata.KindTVShow:
		return &document{Kind: m.Kind, TVShow: tvShowFromMetadata(m)}
	case metadata.KindEpisode:
		return &document{Kind: m.Kind, Episodes: []episodeDetails{*episodeFromMetadata(m)}}
	case metadata.KindMusicVideo:
		return &document{Kind: m.Kind, MusicVideo: musicVideoFromMetadata(m)}
	default:
		return &document{Kind: m.Kind}
	}
}

func movieFromMetadata(m *metadata.Metadata) *movie {
	directors, actors, credits := castCrewParts(m.CastCrew)
	out := &movie{
		Title:         m.Title,
		OriginalTitle: m.OriginalTitle,
		Ratings:       m.Ratings,
		UserRating:    m.UserRating,
		Top250:        m.Top250,
		Outline:       m.Outline,
		Plot:          m.Plot,
		Tagline:       m.Tagline,
		Runtime:       m.Runtime,
		Thumbs:        m.Thumbs,
		MPAA:          m.MPAA,
		PlayCount:     itoaOrEmpty(m.PlayCount),
		LastPlayed:    m.LastPlayed,
		UniqueIDs:     metadata.UniqueIDsFromLinks(m.ExternalLinks),
		ID:            m.ID,
		Genres:        m.Genres,
		Tags:          m.Tags,
		Credits:       credits,
		Directors:     directors,
		Premiered:     metadata.FormatDate(m.Premiered),
		Year:          itoaOrEmpty(m.Year),
		Studios:       m.Studios,
		Trailer:       m.Trailer,
		Actors:        actors,
		Resume:        m.Resume,
		DateAdded:     metadata.FormatDate(m.DateAdded),
		Extra:         m.Extra,
	}
	if f := m.Movie; f != nil {
		out.SortTitle = f.SortTitle
		out.Fanart = f.Fanart
		out.Set = f.Set
		out.Countries = f.Countries
		out.ShowLinks = f.ShowLinks
		out.OriginalLanguage = f.OriginalLanguage
	}
	return out
}

func tvShowFromMetadata(m *metadata.Metadata) *tvShow {
	_, actors, _ := castCrewParts(m.CastCrew)
	return &tvShow{
		Title:         m.Title,
		OriginalTitle: m.OriginalTitle,
		Ratings:       m.Ratings,
		UserRating:    m.UserRating,
		Top250:        m.Top250,
		Outline:       m.Outline,
		Plot:          m.Plot,
		Tagline:       m.Tagline,
		Runtime:       m.Runtime,
		Thumbs:        m.Thumbs,
		MPAA:          m.MPAA,
		PlayCount:     itoaOrEmpty(m.PlayCount),
		LastPlayed:    m.LastPlayed,
		UniqueIDs:     metadata.UniqueIDsFromLinks(m.ExternalLinks),
		ID:            m.ID,
		Genres:        m.Genres,
		Tags:          m.Tags,
		Premiered:     metadata.FormatDate(m.Premiered),
		Year:          itoaOrEmpty(m.Year),
		Status:        m.Status,
		Code:          m.Code,
		Aired:         m.Aired,
		Studios:       m.Studios,
		Trailer:       m.Trailer,
		Actors:        actors,
		Resume:        m.Resume,
		DateAdded:     metadata.FormatDate(m.DateAdded),
		Extra:         m.Extra,
	}
}

func episodeFromMetadata(m *metadata.Metadata) *episodeDetails {
	directors, actors, credits := castCrewParts(m.CastCrew)
	out := &episodeDetails{
		Title:         m.Title,
		OriginalTitle: m.OriginalTitle,
		Ratings:       m.Ratings,
		UserRating:    m.UserRating,
		Top250:        m.Top250,
		Outline:       m.Outline,
		Plot:          m.Plot,
		Tagline:       m.Tagline,
		Runtime:       m.Runtime,
		Thumbs:        m.Thumbs,
		MPAA:          m.MPAA,
		PlayCount:     itoaOrEmpty(m.PlayCount),
		LastPlayed:    m.LastPlayed,
		UniqueIDs:     metadata.UniqueIDsFromLinks(m.ExternalLinks),
		ID:            m.ID,
		Genres:        m.Genres,
		Credits:       credits,
		Directors:     directors,
		Premiered:     metadata.FormatDate(m.Premiered),
		Year:          itoaOrEmpty(m.Year),
		Status:        m.Status,
		Code:          m.Code,
		Aired:         m.Aired,
		Studios:       m.Studios,
		Trailer:       m.Trailer,
		Actors:        actors,
		Resume:        m.Resume,
		DateAdded:     metadata.FormatDate(m.DateAdded),
		Extra:         m.Extra,
	}
	if f := m.Episode; f != nil {
		out.ShowTitle = f.ShowTitle
		out.Season = f.Season
		out.Episode = f.Episode
		out.DisplayEpisode = f.DisplayEpisode
		out.DisplaySeason = f.DisplaySeason
		out.EpisodeBookmark = f.EpisodeBookmark
	}
	return out
}

func musicVideoFromMetadata(m *metadata.Metadata) *musicVideo {
	directors, actors, _ := castCrewParts(m.CastCrew)
	out := &musicVideo{
		Title:      m.Title,
		UserRating: m.UserRating,
		Top250:     m.Top250,
		Outline:    m.Outline,
		Plot:       m.Plot,
		Tagline:    m.Tagline,
		Runtime:    m.Runtime,
		Thumbs:     m.Thumbs,
		MPAA:       m.MPAA,
		PlayCount:  itoaOrEmpty(m.PlayCount),
		LastPlayed: m.LastPlayed,
		UniqueIDs:  metadata.UniqueIDsFromLinks(m.ExternalLinks),
		ID:         m.ID,
		Genres:     m.Genres,
		Tags:       m.Tags,
		Directors:  directors,
		Premiered:  metadata.FormatDate(m.Premiered),
		Year:       itoaOrEmpty(m.Year),
		Status:     m.Status,
		Code:       m.Code,
		Aired:      m.Aired,
		Studios:    m.Studios,
		Trailer:    m.Trailer,
		Actors:     actors,
		Resume:     m.Resume,
		DateAdded:  metadata.FormatDate(m.DateAdded),
		Extra:      m.Extra,
	}
	if f := m.MusicVideo; f != nil {
		out.Track = f.Track
		out.Album = f.Album
		out.Artists = f.Artists
	}
	return out
}

// castCrew groups the people credited on an item, returning nil when there are
// none so an empty block is omitted from storage.
func castCrew(directors []string, actors []metadata.Actor, credits []string) *metadata.CastCrew {
	if len(directors) == 0 && len(actors) == 0 && len(credits) == 0 {
		return nil
	}
	return &metadata.CastCrew{Directors: directors, Actors: actors, Credits: credits}
}

// castCrewParts unpacks a cast/crew block back into its slices, tolerating nil.
func castCrewParts(cc *metadata.CastCrew) (directors []string, actors []metadata.Actor, credits []string) {
	if cc == nil {
		return nil, nil, nil
	}
	return cc.Directors, cc.Actors, cc.Credits
}

// atoiOrZero parses a numeric NFO tag, returning zero when it is empty or not a
// number.
func atoiOrZero(raw string) int {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

// itoaOrEmpty renders an integer back to its tag text, leaving zero as empty so
// an unset value is omitted from the written file.
func itoaOrEmpty(value int) string {
	if value == 0 {
		return ""
	}
	return strconv.Itoa(value)
}
