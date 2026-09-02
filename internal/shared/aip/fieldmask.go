package aip

import (
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// ErrEmptyMask is returned when an Update carries no update_mask, or one with
// no paths. AIP-134: a partial update must name the fields it changes, so an
// empty mask is rejected rather than treated as "replace everything".
var ErrEmptyMask = errors.New("update_mask must name at least one field")

// ErrUnknownPath is wrapped when an update_mask path names no field of the
// resource, or tries to descend through a non-message field. A typo fails
// loudly instead of silently no-opping.
var ErrUnknownPath = errors.New("update_mask names an unknown field")

// ApplyUpdateMask copies the fields named by mask from src onto dst, both of
// which must be the same message type. It is the one FieldMask application
// path for the AIP config Update methods.
//
//   - A nil mask, or one with no paths, returns ErrEmptyMask.
//   - A path naming no field of the resource, or descending through a
//     scalar / list / map, returns ErrUnknownPath.
//   - Dotted paths are honoured: `storage.ttl` sets exactly that nested
//     field and leaves its siblings on dst untouched.
//   - A path naming a message-, list- or map-typed field replaces that
//     field on dst wholesale (a deep copy of src's value, or a clear when
//     src does not carry it).
func ApplyUpdateMask(dst, src proto.Message, mask *fieldmaskpb.FieldMask) error {
	if mask == nil || len(mask.GetPaths()) == 0 {
		return ErrEmptyMask
	}

	dstReflect := dst.ProtoReflect()
	srcReflect := src.ProtoReflect()
	if dstReflect.Descriptor() != srcReflect.Descriptor() {
		return fmt.Errorf("aip: ApplyUpdateMask dst is %s, src is %s",
			dstReflect.Descriptor().FullName(), srcReflect.Descriptor().FullName())
	}

	// Normalize sorts, de-dupes and drops any path already covered by a
	// shorter one on a clone, so the caller's mask is never mutated.
	normalized := proto.Clone(mask).(*fieldmaskpb.FieldMask)
	normalized.Normalize()

	// Validate every path against the descriptor before touching dst, so a
	// mask carrying one bad path leaves dst exactly as it was rather than
	// half-applied.
	segmentsByPath := make([][]string, len(normalized.GetPaths()))
	for i, path := range normalized.GetPaths() {
		segments := strings.Split(path, ".")
		if err := validatePath(dstReflect.Descriptor(), segments, path); err != nil {
			return err
		}
		segmentsByPath[i] = segments
	}

	for _, segments := range segmentsByPath {
		applyPath(dstReflect, srcReflect, segments)
	}
	return nil
}

// validatePath reports whether every segment of one dotted path names a
// field, and that each non-final segment is a singular message it can
// descend into. It reads only descriptors — it never mutates dst.
func validatePath(md protoreflect.MessageDescriptor, segments []string, fullPath string) error {
	fd := md.Fields().ByName(protoreflect.Name(segments[0]))
	if fd == nil {
		return fmt.Errorf("%w: %q on %s", ErrUnknownPath, fullPath, md.FullName())
	}
	if len(segments) == 1 {
		return nil
	}
	if fd.Kind() != protoreflect.MessageKind || fd.IsList() || fd.IsMap() {
		return fmt.Errorf("%w: %q cannot descend through %s", ErrUnknownPath, fullPath, fd.FullName())
	}
	return validatePath(fd.Message(), segments[1:], fullPath)
}

// applyPath walks one already-validated dotted path in lock-step down dst
// and src, creating intermediate messages on dst as it descends and
// replacing the leaf field at the bottom.
func applyPath(dst, src protoreflect.Message, segments []string) {
	fd := dst.Descriptor().Fields().ByName(protoreflect.Name(segments[0]))
	if len(segments) == 1 {
		replaceField(dst, src, fd)
		return
	}

	childDst := dst.Mutable(fd).Message()
	childSrc := childDst.New()
	if src.Has(fd) {
		childSrc = src.Get(fd).Message()
	}
	applyPath(childDst, childSrc, segments[1:])
}

// replaceField overwrites fd on dst with a deep copy of fd on src, or clears
// it when src does not carry the field.
func replaceField(dst, src protoreflect.Message, fd protoreflect.FieldDescriptor) {
	if !src.Has(fd) {
		dst.Clear(fd)
		return
	}
	value := src.Get(fd)
	switch {
	case fd.IsList():
		srcList := value.List()
		dst.Clear(fd)
		dstList := dst.Mutable(fd).List()
		for i := 0; i < srcList.Len(); i++ {
			dstList.Append(cloneElem(dstList, fd, srcList.Get(i)))
		}
	case fd.IsMap():
		dst.Clear(fd)
		dstMap := dst.Mutable(fd).Map()
		value.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
			if fd.MapValue().Kind() == protoreflect.MessageKind {
				msg := dstMap.NewValue().Message()
				proto.Merge(msg.Interface(), v.Message().Interface())
				dstMap.Set(k, protoreflect.ValueOfMessage(msg))
			} else {
				dstMap.Set(k, v)
			}
			return true
		})
	case fd.Kind() == protoreflect.MessageKind:
		msg := dst.NewField(fd).Message()
		proto.Merge(msg.Interface(), value.Message().Interface())
		dst.Set(fd, protoreflect.ValueOfMessage(msg))
	default:
		dst.Set(fd, value)
	}
}

// cloneElem returns a list-appendable copy of one repeated-field element,
// deep-copying a message element and passing a scalar through.
func cloneElem(dstList protoreflect.List, fd protoreflect.FieldDescriptor, v protoreflect.Value) protoreflect.Value {
	if fd.Kind() != protoreflect.MessageKind {
		return v
	}
	msg := dstList.NewElement().Message()
	proto.Merge(msg.Interface(), v.Message().Interface())
	return protoreflect.ValueOfMessage(msg)
}
