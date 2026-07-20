package galleries

import (
	"embed"
)

//go:embed */*.html
var EmbeddedGalleries embed.FS
