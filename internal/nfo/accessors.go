package nfo

import (
	"encoding/xml"

	"Metarr/internal/metadata"
)

// restoreUnknownElementNames repopulates XMLName from Name for any preserved
// element that lost it.
//
// XMLName is dropped when a document is stored somewhere that doesn't keep an
// xml.Name, so a record loaded back out arrives with XMLName empty — and
// encoding/xml needs it to know what to call the element. Without this, a
// preserved unknown tag would be silently dropped or mis-emitted on the first
// write of a record that had been through storage.
func restoreUnknownElementNames(elements []metadata.UnknownElement) {
	for i := range elements {
		if elements[i].XMLName.Local == "" {
			elements[i].XMLName = xml.Name{Local: elements[i].Name}
		}
	}
}

// captureUnknownElementNames mirrors XMLName into Name so the element survives
// being stored somewhere that doesn't keep xml.Name.
func captureUnknownElementNames(elements []metadata.UnknownElement) {
	for i := range elements {
		if elements[i].Name == "" {
			elements[i].Name = elements[i].XMLName.Local
		}
	}
}
