package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"slices"
)

// This source name is keyed by the original descriptor-seeded parameter slot,
// not by a local spelling or a captured field's name. The existing lambda
// renderer resolves its capture at the actual invokedynamic source site.
func (c *ClassObjectDumper) nativeLambdaParameterCaptureName(ref *values.JavaRef) (string, bool) {
	if c == nil || c.obj == nil || c.FuncCtx == nil || c.CurrentMethod == nil || ref == nil {
		return "", false
	}
	name, desc := c.FuncCtx.FunctionName, c.FuncCtx.CurrentMethodDesc
	originalName, nk := sourceBridgeUTF8(c.obj, c.CurrentMethod.NameIndex)
	originalDesc, dk := sourceBridgeUTF8(c.obj, c.CurrentMethod.DescriptorIndex)
	if !nk || !dk || originalName != name || originalDesc != desc {
		return "", false
	}
	if !slices.Contains(c.lambdaMethods[name], desc) {
		return "", false
	}
	slot, original := ref.OriginalParameterSlot()
	params, _, err := callbinding.Descriptor(desc)
	if !original || err != nil {
		return "", false
	}
	captureCount := c.lambdaCaptureCount[name+desc]
	word, offset := 0, 0
	if c.CurrentMethod.AccessFlags&StaticFlag == 0 {
		word, offset = 1, 1
	}
	if captureCount <= offset || captureCount-offset > len(params) {
		return "", false
	}
	for index, param := range params[:captureCount-offset] {
		if word == slot {
			return fmt.Sprintf("\x00LCAP%d\x00", index+offset), true
		}
		word++
		if param == "J" || param == "D" {
			word++
		}
	}
	return "", false
}
