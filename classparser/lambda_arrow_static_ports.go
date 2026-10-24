package javaclassparser

import (
	"slices"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// An ordinary noncapturing arrow is lowered with its instantiated SAM input
// types. A compiler adapter may instead have the selected declaration's
// expanded input types (for example long for an int SAM, or Object for String).
// If the exact original bootstrap demonstrates that difference, falling back
// to an arrow would silently change the implementation ABI. Failure to prove a
// reference cannot erase this independent negative evidence.
func (c *ClassObjectDumper) ordinaryArrowChangesStaticImplementationPorts(name, descriptor string, request methodSourceRequest) bool {
	if c == nil || request.captureCount != 0 || !request.factory.closed() || !request.factory.sealed {
		return false
	}
	site, known := c.originalCompilerReferenceSite(request.factory)
	if !known || site.captureCount != 0 || site.implementation.Member != name || site.implementation.Description != descriptor {
		return false
	}
	physical, _, physicalError := callbinding.Descriptor(descriptor)
	instantiated, _, samError := callbinding.Descriptor(site.actual)
	return physicalError != nil || samError != nil || !slices.Equal(physical, instantiated)
}
