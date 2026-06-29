package assets

import "embed"

//go:embed all:manifests/**
var Manifests embed.FS
