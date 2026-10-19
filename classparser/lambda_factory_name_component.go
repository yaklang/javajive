package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// This compiler spelling is queried only after an original factory edge has
// been proved. Initializers are JVM method roles, not Java method identifiers;
// a user method named static/new/null supplies no initializer permission.
func lambdaFactoryNameComponent(factory *lambdaSourceFactory) (string, bool) {
	if !factory.closed() {
		return "", false
	}
	name, named := sourceBridgeUTF8(factory.object, factory.method.NameIndex)
	descriptor, typed := sourceBridgeUTF8(factory.object, factory.method.DescriptorIndex)
	if !named || !typed {
		return "", false
	}
	switch name {
	case "<clinit>":
		return "static", descriptor == "()V" && factory.method.AccessFlags == 8
	case "<init>":
		_, result, err := callbinding.Descriptor(descriptor)
		return "new", err == nil && result == "V" && factory.method.AccessFlags&(8|0x1000) == 0
	}
	if factory.method.AccessFlags == 0x100a {
		return "null", true
	}
	return name, class_context.PreservesJava8Identifier(name)
}
