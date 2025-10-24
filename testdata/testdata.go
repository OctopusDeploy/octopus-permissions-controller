package testdata

import (
	"embed"
)

//go:embed all:e2e
var E2E embed.FS

//go:embed all:unit
var Unit embed.FS
