package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"maps"
)

func maskMethodFormalErasure(ctx *class_context.ClassContext, names []string) func() {
	if ctx == nil || len(names) == 0 {
		return func() {}
	}
	raw, standalone, forced := ctx.RawEraseTypeVars, ctx.StandaloneEraseTypeVars, ctx.ForceParamEraseTypeVars
	ctx.RawEraseTypeVars = maps.Clone(raw)
	ctx.StandaloneEraseTypeVars = maps.Clone(standalone)
	ctx.ForceParamEraseTypeVars = maps.Clone(forced)
	for _, name := range names {
		delete(ctx.RawEraseTypeVars, name)
		delete(ctx.StandaloneEraseTypeVars, name)
		delete(ctx.ForceParamEraseTypeVars, name)
	}
	return func() {
		ctx.RawEraseTypeVars = raw
		ctx.StandaloneEraseTypeVars = standalone
		ctx.ForceParamEraseTypeVars = forced
	}
}
