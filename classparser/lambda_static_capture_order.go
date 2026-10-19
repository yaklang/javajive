package javaclassparser

import "github.com/yaklang/javajive/classparser/decompiler/core/callbinding"

// The original static implementation descriptor supplies physical capture
// words. The actual sealed callback supplies their count; a name prefix or
// MethodParameters absence cannot authorize a different capture binding.
func (c *ClassObjectDumper) staticLambdaCaptureOrder(method *MemberInfo, descriptor string, request methodSourceRequest) ([]string, bool) {
	if c == nil || c.obj == nil || method == nil {
		return nil, false
	}
	if request.role != methodSourceLambdaBody || request.captureCount == 0 || method.AccessFlags&8 == 0 || request.factory == nil {
		return nil, true
	}
	if !request.factory.closed() || !request.factory.sealed || request.factory.object != c.obj || request.captureCount < 0 || request.captureCount > 255 {
		return nil, false
	}
	parameters, _, err := callbinding.Descriptor(descriptor)
	if err != nil || request.captureCount > len(parameters) {
		return nil, false
	}
	return parameters[:request.captureCount], true
}
