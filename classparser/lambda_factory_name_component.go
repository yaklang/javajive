package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
)

// Native javac8 leaves the nested lambda's method owner unnamed during
// translation. Modern javac retains the enclosing source method. Reopen that
// original factory ancestry instead of extracting a guessed component from a
// synthetic spelling. Every edge still needs its own sealed body receipt.
func (c *ClassObjectDumper) lambdaSourceFactoryNameComponent(factory *lambdaSourceFactory) (string, bool) {
	if c == nil || c.obj == nil || !factory.closed() || factory.object != c.obj {
		return "", false
	}
	if c.options.SourceCompiler.effective() == NativeJavac8 {
		return lambdaFactoryNameComponent(factory)
	}
	if c.options.SourceCompiler.effective() != ModernJavac {
		return "", false
	}
	seen := map[*MemberInfo]bool{}
	for depth := 0; depth < methodSourceLambdaDepthLimit; depth++ {
		if !factory.closed() || !factory.sealed || factory.object != c.obj || seen[factory.method] || !nativeProofWork(c.Work, 1) {
			return "", false
		}
		if factory.method.AccessFlags != 0x100a && factory.method.AccessFlags != 0x1002 {
			return lambdaFactoryNameComponent(factory)
		}
		if c.Work != nil && c.Work.CheckAlloc(int64(len(seen)+1)*128) != nil {
			return "", false
		}
		seen[factory.method] = true
		receipt := c.lambdaMethodSourceBodies[factory.method]
		if receipt == nil || receipt.object != c.obj || receipt.method != factory.method || receipt.body == nil || receipt.request != receipt.body.sourceRequest || !c.lambdaMethodSourcePacketClosed(factory.method, receipt.body, c.Work) {
			return "", false
		}
		factory = receipt.request.factory
	}
	return "", false
}

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
	if factory.method.AccessFlags == 0x100a || factory.method.AccessFlags == 0x1002 {
		return "null", true
	}
	return name, class_context.PreservesJava8Identifier(name)
}
