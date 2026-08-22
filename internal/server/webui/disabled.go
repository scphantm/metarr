//go:build !embed_ui

package webui

import "io/fs"

func FS() (fs.FS, bool) {
	return nil, false
}
