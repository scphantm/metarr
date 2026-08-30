package metadata

import (
	"net/url"
	"regexp"
	"strings"
)

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
// stores in a trailer, e.g.
// plugin://plugin.video.youtube/?action=play_video&videoid=dQw4w9WgXcQ
var youtubePluginVideoIDPattern = regexp.MustCompile(`(?i)videoid=([A-Za-z0-9_-]{6,})`)

// ExtractLinks pulls every external provider identifier out of a metadata
// record, folding duplicates. The result populates a series record's
// external_links when run over item-level metadata, and a media file's
// episode_ids when run over that file's own metadata.
func ExtractLinks(m *Metadata) []*Link {
	if m == nil {
		return nil
	}

	collector := &linkCollector{}
	collector.addLinks(m.ExternalLinks)
	collector.addLegacyID(m.Id)
	collector.addTrailer(m.Trailer)
	return collector.links
}

// LinksFromUniqueIDs converts the on-disk <uniqueid> tags into links, folding
// the several spellings the same provider appears under onto one key. This is
// the read half of the NFO round trip; UniqueIDsFromLinks is the write half.
func LinksFromUniqueIDs(uniqueIDs []*UniqueID) []*Link {
	collector := &linkCollector{}
	for _, uniqueID := range uniqueIDs {
		if uniqueID.Type == "" {
			// A type-less uniqueid still carries an id; attribute it the same
			// way as the legacy id tag rather than dropping it.
			collector.addLegacyID(uniqueID.Value)
			continue
		}
		collector.add(uniqueID.Type, uniqueID.Value, uniqueID.Default)
	}
	return collector.links
}

// derivedLinkKeys are the keys a link can hold without a <uniqueid> tag ever
// having said so, because ExtractLinks synthesizes them from other fields. They
// are skipped when rebuilding the tags, since writing them back would invent
// elements the file never contained.
var derivedLinkKeys = map[string]string{
	"id":      "written from Metadata.ID as the legacy <id> tag",
	"youtube": "derived from Metadata.Trailer",
}

// UniqueIDsFromLinks rebuilds the on-disk <uniqueid> tags for writing.
//
// A link whose key is one this package synthesizes from another field is left
// out: those are re-emitted from the field they came from, and turning them into
// uniqueid tags would add elements the source file never had. The cost is that a
// genuine <uniqueid type="youtube"> does not survive a rewrite, which is the
// price of ExternalLinks being both the stored ids and the derived ones.
func UniqueIDsFromLinks(links []*Link) []*UniqueID {
	uniqueIDs := make([]*UniqueID, 0, len(links))
	for _, link := range links {
		if _, derived := derivedLinkKeys[link.Key]; derived {
			continue
		}
		if link.Key == "" || link.Value == "" {
			continue
		}
		uniqueIDs = append(uniqueIDs, &UniqueID{
			Type:    link.Key,
			Value:   link.Value,
			Default: link.Default,
		})
	}
	if len(uniqueIDs) == 0 {
		return nil
	}
	return uniqueIDs
}

// linkCollector accumulates links while suppressing duplicates, which arise
// routinely because the same id often appears in both a uniqueid and the legacy
// id tag.
type linkCollector struct {
	links []*Link
	seen  map[string]bool
}

// linkIdentity is the key and value alone — Link is a generated message and
// not comparable, so duplicate detection keys on this string rather than the
// value itself.
func linkIdentity(key, value string) string { return key + "\x00" + value }

func (c *linkCollector) add(key, value string, isDefault bool) {
	key = strings.TrimSpace(strings.ToLower(key))
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	if canonical, ok := canonicalLinkKeys[key]; ok {
		key = canonical
	}

	// The default flag is an attribute of the same link, not a different one,
	// so the same id arriving twice — once flagged, once not — must not become
	// two entries.
	identity := linkIdentity(key, value)
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	if c.seen[identity] {
		if isDefault {
			for i := range c.links {
				if c.links[i].Key == key && c.links[i].Value == value {
					c.links[i].Default = true
				}
			}
		}
		return
	}
	c.seen[identity] = true
	c.links = append(c.links, &Link{Key: key, Value: value, Default: isDefault})
}

// addLinks folds in links that were already resolved, which is how the ids read
// straight out of a document's uniqueid tags reach the union.
func (c *linkCollector) addLinks(links []*Link) {
	for _, link := range links {
		c.add(link.Key, link.Value, link.Default)
	}
}

// addLegacyID handles the pre-uniqueid single id, whose provider is implied by
// the value's shape.
func (c *linkCollector) addLegacyID(id string) {
	value := strings.TrimSpace(id)
	if value == "" {
		return
	}
	if imdbIDPattern.MatchString(value) {
		c.add("imdb", value, false)
		return
	}
	c.add("id", value, false)
}

// addTrailer extracts a YouTube video id from a trailer reference. Kodi stores
// these either as a plugin:// URL or an ordinary YouTube link, and the id is a
// genuine external reference worth capturing.
func (c *linkCollector) addTrailer(trailer string) {
	value := strings.TrimSpace(trailer)
	if value == "" {
		return
	}

	if matches := youtubePluginVideoIDPattern.FindStringSubmatch(value); matches != nil {
		c.add("youtube", matches[1], false)
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
			c.add("youtube", videoID, false)
		}
	case "youtu.be":
		if videoID := strings.Trim(parsed.Path, "/"); videoID != "" {
			c.add("youtube", videoID, false)
		}
	}
}
