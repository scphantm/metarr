package nfo

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Link is one external-provider identifier found in an NFO file, such as
// {tmdb, 603} or {youtube, dQw4w9WgXcQ}. Keeping these as key/value pairs
// rather than named fields means a provider Metarr has never heard of is still
// captured and queryable.
type Link struct {
	Key   string `bson:"key" json:"key"`
	Value string `bson:"value" json:"value"`
}

// canonicalLinkKeys folds the spellings the same provider appears under onto
// one key, so a lookup doesn't have to try every variant. Providers missing
// from this table are kept under their own lowercased name.
var canonicalLinkKeys = map[string]string{
	"tvdb":        "tvdb",
	"tvdbid":      "tvdb",
	"thetvdb":     "tvdb",
	"tmdb":        "tmdb",
	"tmdbid":      "tmdb",
	"themoviedb":  "tmdb",
	"imdb":        "imdb",
	"imdbid":      "imdb",
	"tvmaze":      "tvmaze",
	"tvmazeid":    "tvmaze",
	"tvrage":      "tvrage",
	"anidb":       "anidb",
	"trakt":       "trakt",
	"musicbrainz": "musicbrainz",
	"youtube":     "youtube",
}

// imdbIDPattern recognizes an IMDb identifier, which lets the legacy single
// <id> tag be attributed to the right provider: Kodi wrote either an IMDb
// "tt"-prefixed id or a bare numeric id from whichever scraper was in use.
var imdbIDPattern = regexp.MustCompile(`^tt\d+$`)

// youtubePluginVideoIDPattern extracts the video id from the plugin URL Kodi
// stores in <trailer>, e.g.
// plugin://plugin.video.youtube/?action=play_video&videoid=dQw4w9WgXcQ
var youtubePluginVideoIDPattern = regexp.MustCompile(`(?i)videoid=([A-Za-z0-9_-]{6,})`)

// ExtractLinks pulls every external provider identifier out of doc, folding
// duplicates. The result populates a directory record's external_links when
// run over an item-level NFO, and a media file's episode_ids when run over
// that file's own sidecar.
func ExtractLinks(doc *Document) []Link {
	if doc == nil {
		return nil
	}

	collector := &linkCollector{}

	switch {
	case doc.Movie != nil:
		collector.addUniqueIDs(doc.Movie.UniqueIDs)
		collector.addLegacyID(doc.Movie.ID)
		collector.addTrailer(doc.Movie.Trailer)

	case doc.TVShow != nil:
		collector.addUniqueIDs(doc.TVShow.UniqueIDs)
		collector.addLegacyID(doc.TVShow.ID)
		collector.addTrailer(doc.TVShow.Trailer)
		collector.addEpisodeGuide(doc.TVShow.EpisodeGuide)

	case doc.MusicVideo != nil:
		collector.addUniqueIDs(doc.MusicVideo.UniqueIDs)
		collector.addLegacyID(doc.MusicVideo.ID)
		collector.addTrailer(doc.MusicVideo.Trailer)
	}

	for _, episode := range doc.Episodes {
		collector.addUniqueIDs(episode.UniqueIDs)
		collector.addLegacyID(episode.ID)
		collector.addTrailer(episode.Trailer)
	}

	return collector.links
}

// linkCollector accumulates links while suppressing exact duplicates, which
// arise routinely because the same id often appears in both <uniqueid> and the
// legacy <id> tag.
type linkCollector struct {
	links []Link
	seen  map[Link]bool
}

func (c *linkCollector) add(key, value string) {
	key = strings.TrimSpace(strings.ToLower(key))
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	if canonical, ok := canonicalLinkKeys[key]; ok {
		key = canonical
	}

	link := Link{Key: key, Value: value}
	if c.seen == nil {
		c.seen = map[Link]bool{}
	}
	if c.seen[link] {
		return
	}
	c.seen[link] = true
	c.links = append(c.links, link)
}

// addUniqueIDs handles the <uniqueid type="..."> tags, the modern and primary
// source of provider identity.
func (c *linkCollector) addUniqueIDs(uniqueIDs []UniqueID) {
	for _, uniqueID := range uniqueIDs {
		key := uniqueID.Type
		if key == "" {
			// A type-less uniqueid still carries an id; attribute it the same
			// way as the legacy <id> tag rather than dropping it.
			c.addLegacyID(uniqueID.Value)
			continue
		}
		c.add(key, uniqueID.Value)
	}
}

// addLegacyID handles the pre-uniqueid single <id> tag, whose provider is
// implied by the value's shape.
func (c *linkCollector) addLegacyID(id string) {
	value := strings.TrimSpace(id)
	if value == "" {
		return
	}
	if imdbIDPattern.MatchString(value) {
		c.add("imdb", value)
		return
	}
	c.add("id", value)
}

// addTrailer extracts a YouTube video id from a <trailer> tag. Kodi stores
// these either as a plugin:// URL or an ordinary YouTube link, and the id is
// a genuine external reference worth capturing.
func (c *linkCollector) addTrailer(trailer string) {
	value := strings.TrimSpace(trailer)
	if value == "" {
		return
	}

	if matches := youtubePluginVideoIDPattern.FindStringSubmatch(value); matches != nil {
		c.add("youtube", matches[1])
		return
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Host), "www.")
	switch host {
	case "youtube.com", "m.youtube.com":
		if videoID := parsed.Query().Get("v"); videoID != "" {
			c.add("youtube", videoID)
		}
	case "youtu.be":
		if videoID := strings.Trim(parsed.Path, "/"); videoID != "" {
			c.add("youtube", videoID)
		}
	}
}

// addEpisodeGuide reads the <episodeguide> tag, which Kodi v19+ writes as a
// small JSON object of provider ids, e.g. {"tvdb":"12345"}. Older Kodi wrote a
// nested <url> element there instead, which carries no id worth extracting.
func (c *linkCollector) addEpisodeGuide(episodeGuide string) {
	value := strings.TrimSpace(episodeGuide)
	if !strings.HasPrefix(value, "{") {
		return
	}

	var providerIDs map[string]any
	if err := json.Unmarshal([]byte(value), &providerIDs); err != nil {
		return
	}
	for key, rawValue := range providerIDs {
		switch typed := rawValue.(type) {
		case string:
			c.add(key, typed)
		case float64:
			// JSON numbers decode to float64. 'f' with -1 precision keeps an
			// id like 12345 from rendering as "1.2345e+04".
			c.add(key, strconv.FormatFloat(typed, 'f', -1, 64))
		}
	}
}
