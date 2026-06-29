package classes

import (
	"embed"
	"github.com/yaklang/javajive/internal/log"
	"github.com/yaklang/javajive/internal/gzip_embed"
)

//go:embed static.tar.gz
var resourceFS embed.FS

var FS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	FS, err = gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", true)
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		FS = gzip_embed.NewEmptyPreprocessingEmbed()
	}
}
