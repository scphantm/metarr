package nfo

// The on-disk XML shapes of the value types the metadata model shares. The
// model itself is generated from proto now (see docs/adr/0005) and carries
// no XML marshalling directives, so this file holds the XML twins and the
// translation between them. The conversions are the same kind of
// serialization boundary as the numeric- and date-tag conversions in
// translate.go.

import (
	"encoding/xml"

	"Metarr/internal/shared/metadata"
)

type xmlRating struct {
	Name    string `xml:"name,attr,omitempty"`
	Max     string `xml:"max,attr,omitempty"`
	Default bool   `xml:"default,attr,omitempty"`
	Value   string `xml:"value,omitempty"`
	Votes   string `xml:"votes,omitempty"`
}

type xmlRatings struct {
	Rating []xmlRating `xml:"rating,omitempty"`
}

type xmlUniqueID struct {
	Type    string `xml:"type,attr,omitempty"`
	Default bool   `xml:"default,attr,omitempty"`
	Value   string `xml:",chardata"`
}

type xmlThumb struct {
	Aspect  string `xml:"aspect,attr,omitempty"`
	Preview string `xml:"preview,attr,omitempty"`
	Type    string `xml:"type,attr,omitempty"`
	Season  string `xml:"season,attr,omitempty"`
	Value   string `xml:",chardata"`
}

type xmlFanart struct {
	Thumbs []xmlThumb `xml:"thumb,omitempty"`
}

type xmlActor struct {
	Name  string `xml:"name,omitempty"`
	Role  string `xml:"role,omitempty"`
	Order string `xml:"order,omitempty"`
	Thumb string `xml:"thumb,omitempty"`
}

type xmlResume struct {
	Position string `xml:"position,omitempty"`
	Total    string `xml:"total,omitempty"`
}

type xmlMovieSet struct {
	Name     string `xml:"name,omitempty"`
	Overview string `xml:"overview,omitempty"`
}

type xmlEpisodeBookmark struct {
	Position string `xml:"position,omitempty"`
}

// xmlUnknownElement captures a tag the model doesn't recognise so it
// round-trips instead of being silently dropped.
type xmlUnknownElement struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	InnerXML string     `xml:",innerxml"`
}

// --- read direction: XML -> model ---

func ratingsFromXML(r *xmlRatings) *metadata.Ratings {
	if r == nil {
		return nil
	}
	out := &metadata.Ratings{Rating: make([]*metadata.Rating, 0, len(r.Rating))}
	for _, x := range r.Rating {
		out.Rating = append(out.Rating, &metadata.Rating{
			Name: x.Name, Max: x.Max, Default: x.Default, Value: x.Value, Votes: x.Votes,
		})
	}
	return out
}

func thumbsFromXML(in []xmlThumb) []*metadata.Thumb {
	if len(in) == 0 {
		return nil
	}
	out := make([]*metadata.Thumb, 0, len(in))
	for _, x := range in {
		out = append(out, &metadata.Thumb{
			Aspect: x.Aspect, Preview: x.Preview, Type: x.Type, Season: x.Season, Value: x.Value,
		})
	}
	return out
}

func fanartFromXML(f *xmlFanart) *metadata.Fanart {
	if f == nil {
		return nil
	}
	return &metadata.Fanart{Thumbs: thumbsFromXML(f.Thumbs)}
}

func actorsFromXML(in []xmlActor) []*metadata.Actor {
	if len(in) == 0 {
		return nil
	}
	out := make([]*metadata.Actor, 0, len(in))
	for _, x := range in {
		out = append(out, &metadata.Actor{Name: x.Name, Role: x.Role, Order: x.Order, Thumb: x.Thumb})
	}
	return out
}

func resumeFromXML(r *xmlResume) *metadata.Resume {
	if r == nil {
		return nil
	}
	return &metadata.Resume{Position: r.Position, Total: r.Total}
}

func movieSetFromXML(s *xmlMovieSet) *metadata.MovieSet {
	if s == nil {
		return nil
	}
	return &metadata.MovieSet{Name: s.Name, Overview: s.Overview}
}

func episodeBookmarkFromXML(b *xmlEpisodeBookmark) *metadata.EpisodeBookmark {
	if b == nil {
		return nil
	}
	return &metadata.EpisodeBookmark{Position: b.Position}
}

func uniqueIDsFromXML(in []xmlUniqueID) []*metadata.UniqueID {
	if len(in) == 0 {
		return nil
	}
	out := make([]*metadata.UniqueID, 0, len(in))
	for _, x := range in {
		out = append(out, &metadata.UniqueID{Type: x.Type, Default: x.Default, Value: x.Value})
	}
	return out
}

func extraFromXML(in []xmlUnknownElement) []*metadata.UnknownElement {
	if len(in) == 0 {
		return nil
	}
	out := make([]*metadata.UnknownElement, 0, len(in))
	for _, x := range in {
		element := &metadata.UnknownElement{Name: x.XMLName.Local, InnerXml: x.InnerXML}
		for _, attr := range x.Attrs {
			element.Attrs = append(element.Attrs, &metadata.XMLAttr{Name: attr.Name.Local, Value: attr.Value})
		}
		out = append(out, element)
	}
	return out
}

// --- write direction: model -> XML ---

func ratingsToXML(r *metadata.Ratings) *xmlRatings {
	if r == nil {
		return nil
	}
	out := &xmlRatings{Rating: make([]xmlRating, 0, len(r.Rating))}
	for _, m := range r.Rating {
		out.Rating = append(out.Rating, xmlRating{
			Name: m.Name, Max: m.Max, Default: m.Default, Value: m.Value, Votes: m.Votes,
		})
	}
	return out
}

func thumbsToXML(in []*metadata.Thumb) []xmlThumb {
	if len(in) == 0 {
		return nil
	}
	out := make([]xmlThumb, 0, len(in))
	for _, m := range in {
		out = append(out, xmlThumb{
			Aspect: m.Aspect, Preview: m.Preview, Type: m.Type, Season: m.Season, Value: m.Value,
		})
	}
	return out
}

func fanartToXML(f *metadata.Fanart) *xmlFanart {
	if f == nil {
		return nil
	}
	return &xmlFanart{Thumbs: thumbsToXML(f.Thumbs)}
}

func actorsToXML(in []*metadata.Actor) []xmlActor {
	if len(in) == 0 {
		return nil
	}
	out := make([]xmlActor, 0, len(in))
	for _, m := range in {
		out = append(out, xmlActor{Name: m.Name, Role: m.Role, Order: m.Order, Thumb: m.Thumb})
	}
	return out
}

func resumeToXML(r *metadata.Resume) *xmlResume {
	if r == nil {
		return nil
	}
	return &xmlResume{Position: r.Position, Total: r.Total}
}

func movieSetToXML(s *metadata.MovieSet) *xmlMovieSet {
	if s == nil {
		return nil
	}
	return &xmlMovieSet{Name: s.Name, Overview: s.Overview}
}

func episodeBookmarkToXML(b *metadata.EpisodeBookmark) *xmlEpisodeBookmark {
	if b == nil {
		return nil
	}
	return &xmlEpisodeBookmark{Position: b.Position}
}

func uniqueIDsToXML(in []*metadata.UniqueID) []xmlUniqueID {
	if len(in) == 0 {
		return nil
	}
	out := make([]xmlUniqueID, 0, len(in))
	for _, m := range in {
		out = append(out, xmlUniqueID{Type: m.Type, Default: m.Default, Value: m.Value})
	}
	return out
}

func extraToXML(in []*metadata.UnknownElement) []xmlUnknownElement {
	if len(in) == 0 {
		return nil
	}
	out := make([]xmlUnknownElement, 0, len(in))
	for _, m := range in {
		element := xmlUnknownElement{
			XMLName:  xml.Name{Local: m.Name},
			InnerXML: m.InnerXml,
		}
		for _, attr := range m.Attrs {
			element.Attrs = append(element.Attrs, xml.Attr{
				Name:  xml.Name{Local: attr.Name},
				Value: attr.Value,
			})
		}
		out = append(out, element)
	}
	return out
}
