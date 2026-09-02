package services

import (
	"errors"

	"google.golang.org/protobuf/proto"

	"Metarr/internal/shared/appconfig"
)

// errStaleETag is returned from a mutation closure when the etag a client sent
// on an Update / Delete no longer matches the stored section. It maps to
// connect.CodeAborted (AIP-154; see errors.go).
var errStaleETag = errors.New("etag does not match the current resource")

// checkETag compares the etag a client supplied against the current section.
// An empty want is a deliberate blind write and always passes; otherwise a
// mismatch is errStaleETag. The token is computed the one way appconfig
// defines it, so a value read by GetEventBusConfig / GetLoggingConfig and one
// recomputed here under the store lock agree.
func checkETag(current proto.Message, want string) error {
	if want == "" {
		return nil
	}
	if appconfig.SectionETag(current) != want {
		return errStaleETag
	}
	return nil
}
