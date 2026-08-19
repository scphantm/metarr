package mediascan

// This file reads the header of an artwork sidecar. Only the header: the point
// is to know how large a poster is and what it is stored as, which the leading
// bytes already say, so no pixels are ever decoded.

import (
	"image"
	"os"

	// The blank imports below are the set of formats the scanner can read.
	// image.DecodeConfig dispatches to whichever decoder has registered itself,
	// so adding a format here is the whole of adding support for it, and
	// removing one silently narrows what the scan can describe.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// ImageInfo is what the image header says about an artwork sidecar: the format
// it is stored in and its pixel dimensions.
type ImageInfo struct {
	Codec  string `bson:"codec" json:"codec"`
	Width  int    `bson:"width" json:"width"`
	Height int    `bson:"height" json:"height"`

	// Error records why the header could not be read, for a file named like
	// artwork that cannot be decoded. It is the only field set when it is set,
	// so a corrupt poster stays distinguishable from one never examined.
	Error string `bson:"error,omitempty" json:"error,omitempty"`
}

// readImageInfo reads the codec and dimensions from an image file's header.
//
// The format is taken from the file's contents rather than its extension. A
// .tbn is a Kodi-era artwork extension holding whatever the encoder produced —
// usually JPEG — and artwork saved with the wrong extension is common enough
// that the bytes are the only trustworthy answer to what a file actually is.
func readImageInfo(path string) (*ImageInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// DecodeConfig reads only as far as the header, buffering internally, so
	// this does not pull a multi-megabyte poster into memory.
	config, format, err := image.DecodeConfig(file)
	if err != nil {
		return nil, err
	}

	return &ImageInfo{
		Codec:  format,
		Width:  config.Width,
		Height: config.Height,
	}, nil
}
