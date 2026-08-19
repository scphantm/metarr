package scanmodel

// This file carries what an artwork sidecar's header says about it. Only the
// record lives here; reading the header means opening the file, so that lives
// in the agent's mediascan package.

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
