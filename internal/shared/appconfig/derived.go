package appconfig

import (
	"crypto/sha256"
	"encoding/base64"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// etagFieldName is the field every AIP config resource / scalar section carries
// for its AIP-154 concurrency token.
const etagFieldName protoreflect.Name = "etag"

// SectionETag is the concurrency token for one config resource or scalar
// section: a base64url SHA-256 of its deterministically-marshalled bytes with
// the etag field itself cleared, so the token hashes the section's content and
// two reads of an unchanged section always produce the same value. Derived on
// read, never stored (ADR-0005). Opaque to clients — they echo back what they
// last read and the server compares.
func SectionETag(m proto.Message) string {
	if m == nil {
		return ""
	}
	r := m.ProtoReflect()
	if !r.IsValid() {
		return ""
	}
	clone := proto.Clone(m)
	clearField(clone, etagFieldName)
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// WithSectionETag returns a clone of m with its etag field set to m's current
// token. A read path uses it so a response carries a usable etag without the
// caller's message being mutated.
func WithSectionETag[T proto.Message](m T) T {
	clone := proto.Clone(m)
	setStringField(clone, etagFieldName, SectionETag(m))
	return clone.(T)
}

// ClearDerived strips every field the AIP config API derives on read and must
// never persist (ADR-0005): today the two scalar sections' etag. The
// collection slices extend it for each resource's name and etag and for agent
// presence. It mutates cfg in place; MarshalStored calls it on a clone so a
// caller's config (which may be the live singleton) is never disturbed.
func ClearDerived(cfg *Config) {
	if cfg == nil {
		return
	}
	clearField(cfg.EventBus, etagFieldName)
	clearField(cfg.Logging, etagFieldName)
}

// normalizeDerivedETags stamps each scalar section's current etag onto it, so
// a read through live config carries a concurrency token without any service
// method computing one. One independent Normalize block per ADR-0010's seam;
// the per-resource-kind name/etag backfill for collections lands here too.
func normalizeDerivedETags(cfg *Config) {
	if cfg.EventBus != nil {
		setStringField(cfg.EventBus, etagFieldName, SectionETag(cfg.EventBus))
	}
	if cfg.Logging != nil {
		setStringField(cfg.Logging, etagFieldName, SectionETag(cfg.Logging))
	}
}

func clearField(m proto.Message, name protoreflect.Name) {
	if m == nil {
		return
	}
	r := m.ProtoReflect()
	if !r.IsValid() {
		return
	}
	if fd := r.Descriptor().Fields().ByName(name); fd != nil {
		r.Clear(fd)
	}
}

func setStringField(m proto.Message, name protoreflect.Name, value string) {
	if m == nil {
		return
	}
	r := m.ProtoReflect()
	if !r.IsValid() {
		return
	}
	if fd := r.Descriptor().Fields().ByName(name); fd != nil {
		r.Set(fd, protoreflect.ValueOfString(value))
	}
}
