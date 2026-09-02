package services

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// errStaleETag is returned from a mutation closure when the etag a client sent
// on an Update / Delete no longer matches the stored section. It maps to
// connect.CodeAborted (AIP-154; see errors.go).
var errStaleETag = errors.New("etag does not match the current resource")

// etagFieldName is the field every AIP config resource / scalar section carries
// for its concurrency token. It is cleared before hashing so the token is a
// hash of the section's *content*, not of a section that already contains its
// own previous token.
const etagFieldName protoreflect.Name = "etag"

// sectionETag is the AIP-154 concurrency token for one config resource or
// scalar section: a hash of its deterministically-marshalled bytes with the
// etag field itself cleared. Derived on every read, never stored (ADR-0005),
// so two reads of an unchanged section produce the same token and any write in
// between changes it. Opaque to clients — they echo back what they last read
// and the server compares.
func sectionETag[T proto.Message](m T) string {
	clone := proto.Clone(m)
	clearETagField(clone)
	bytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(bytes)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// withETag returns a clone of m with its etag field set to m's current token.
// Read paths call this so the response carries a usable etag without the live
// config singleton ever holding one.
func withETag[T proto.Message](m T) T {
	clone := proto.Clone(m)
	setETagField(clone, sectionETag(m))
	return clone.(T)
}

// checkETag compares the etag a client supplied against the current section.
// An empty want is a deliberate blind write and always passes; otherwise a
// mismatch is errStaleETag.
func checkETag[T proto.Message](current T, want string) error {
	if want == "" {
		return nil
	}
	if sectionETag(current) != want {
		return errStaleETag
	}
	return nil
}

func clearETagField(m proto.Message) {
	r := m.ProtoReflect()
	if fd := r.Descriptor().Fields().ByName(etagFieldName); fd != nil {
		r.Clear(fd)
	}
}

func setETagField(m proto.Message, value string) {
	r := m.ProtoReflect()
	if fd := r.Descriptor().Fields().ByName(etagFieldName); fd != nil {
		r.Set(fd, protoreflect.ValueOfString(value))
	}
}
