//go:build embed_ui

package webui

import (
	"io/fs"

	"Metarr/ui"
)

func FS() (fs.FS, bool) {
	sub, err := fs.Sub(ui.DistFS, "dist")
	if err != nil {
		return nil, false
	}
	return sub, true
}
