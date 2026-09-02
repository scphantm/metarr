package services

import (
	"errors"
	"fmt"

	"go.einride.tech/aip/fieldmask"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

// errEmptyMask is returned when an Update carries no update_mask, or one with no
// paths. AIP-134: a partial update must name the fields it changes, so an empty
// mask is rejected rather than treated as "replace everything". errUnknownPath
// wraps a path that names no field of the resource or descends through a
// scalar. Both map to connect.CodeInvalidArgument (see errors.go).
var (
	errEmptyMask   = errors.New("update_mask must name at least one field")
	errUnknownPath = errors.New("update_mask names an unknown field")
)

// applyUpdateMask copies the fields named by mask from src onto dst, both the
// same message type. It is the one FieldMask application path for the AIP config
// Update methods, wrapping go.einride.tech/aip/fieldmask with the config API's
// stricter error model: einride's Update treats an absent mask as "copy non-zero
// src fields" and never reports a bad path, whereas ADR-0010 requires an empty
// mask or an unknown path to fail InvalidArgument.
func applyUpdateMask(dst, src proto.Message, mask *fieldmaskpb.FieldMask) error {
	if mask == nil || len(mask.GetPaths()) == 0 {
		return errEmptyMask
	}
	if err := fieldmask.Validate(mask, dst); err != nil {
		return fmt.Errorf("%w: %w", errUnknownPath, err)
	}
	fieldmask.Update(mask, dst, src)
	return nil
}
