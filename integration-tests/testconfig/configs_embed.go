
//go:build embed
// +build embed

package testconfig

import "embed"

//go:embed default.toml
//go:embed ocr2/ocr2.toml
var embeddedConfigsFs embed.FS

func init() {
	areConfigsEmbedded = true
}
