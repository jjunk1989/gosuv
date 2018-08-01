// +build !vfs
//go:generate go run assets_generate.go

package main

import (
	"net/http"
	"path"
)

// Assets contains project assets.
var Assets http.FileSystem = http.Dir(http.Dir(path.Join(defaultGosuvDir, "res")))
