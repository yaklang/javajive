package methodir

import "github.com/yaklang/javajive/classparser/decompiler/core"

func init() {
	core.ShadowIRBuilder = BuildFromRequest
}
