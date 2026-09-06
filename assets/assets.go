// Package assets bundles the application's static resources, currently the
// built-in rain background sound seeded into the data directory on first run.
package assets

import _ "embed"

// RainSound is the bundled "Rain_Background.mp3" content.
//
//go:embed rain-sound.mp3
var RainSound []byte
