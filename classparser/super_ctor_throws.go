package javaclassparser

import (
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// superCtorCheckedThrows returns the checked exceptions declared by the
// superclass constructor this `<init>` invokes, when this constructor itself
// has no Exceptions attribute. javac (classfile 49, junit MaxCore$1$1) omits
// Exceptions on the synthetic anonymous-class constructor even though
// `super(...)` throws a checked exception, so the decompiled ctor fails
// "unreported exception InitializationError". Kill-switch:
// JDEC_SUPER_CTOR_THROWS_OFF=1.
func (c *ClassObjectDumper) superCtorCheckedThrows(plans ...*constructorSourceBoundary) (string, error) {
	if c.getenv("JDEC_SUPER_CTOR_THROWS_OFF") == "1" {
		return "", nil
	}
	if c == nil || c.obj == nil {
		return "", nil
	}
	super := c.obj.GetSupperClassName()
	if super == "" || super == "java/lang/Object" {
		return "", nil
	}
	desc := c.superInitDescriptor(super)
	if desc == "" {
		return "", nil
	}
	if len(plans) == 1 && plans[0] != nil && plans[0].throwsReceipt != nil {
		r := plans[0].throwsReceipt
		if !r.consumed(c.FuncCtx) {
			return "", fmt.Errorf("stale constructor invocation throws source binding")
		}
		if !r.allUnchecked {
			return "", fmt.Errorf("checked constructor delegation requires original declared throws")
		}
		return "", nil
	}
	if extra := jdkSuperCtorThrows(super, desc); extra != "" {
		if c.FuncCtx != nil {
			for _, n := range strings.Split(extra, ", ") {
				if strings.Contains(n, ".") {
					c.FuncCtx.Import(n)
				} else {
					c.FuncCtx.Import("java.io." + n)
				}
			}
		}
		return extra, nil
	}
	if c.foldSiblingResolver == nil {
		return "", nil
	}
	data, ok := c.foldSiblingResolver(super)
	if !ok || len(data) == 0 {
		return "", nil
	}
	sObj, err := c.parseResolved(data)
	if err != nil || sObj == nil {
		return "", nil
	}
	for _, m := range sObj.Methods {
		mn, err := sObj.getUtf8(m.NameIndex)
		if err != nil || mn != "<init>" {
			continue
		}
		md, err := sObj.getUtf8(m.DescriptorIndex)
		if err != nil || md != desc {
			continue
		}
		return c.checkedThrowsFromMethod(sObj, m)
	}
	return "", nil
}

func (c *ClassObjectDumper) superInitDescriptor(superInternal string) string {
	if c.obj == nil || c.obj.ConstantPoolManager == nil {
		return ""
	}
	cp := c.obj.ConstantPoolManager
	superDot := strings.ReplaceAll(superInternal, "/", ".")
	var desc string
	for _, info := range cp.GetData() {
		mr, ok := info.(*ConstantMethodrefInfo)
		if !ok {
			continue
		}
		cls := cp.GetClassName(int(mr.ClassIndex))
		if cls != superInternal && cls != superDot {
			continue
		}
		nt, ok := cp.IndexInfo(int(mr.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
		if !ok || nt == nil {
			continue
		}
		nameStr := cp.GetUtf8(int(nt.NameIndex))
		descStr := cp.GetUtf8(int(nt.DescriptorIndex))
		if nameStr == nil || descStr == nil || nameStr.Value != "<init>" {
			continue
		}
		if desc != "" && desc != descStr.Value {
			return ""
		}
		desc = descStr.Value
	}
	return desc
}

func (c *ClassObjectDumper) checkedThrowsFromMethod(obj *ClassObject, m *MemberInfo) (string, error) {
	if obj == nil || m == nil || c.FuncCtx == nil {
		return "", nil
	}
	if !nativeProofWork(c.Work, int64(len(m.Attributes))+1) {
		return "", fmt.Errorf("superclass constructor exception proof budget exceeded")
	}
	for _, attribute := range m.Attributes {
		if exceptions, ok := attribute.(*ExceptionsAttribute); ok && (exceptions == nil || len(exceptions.ExceptionIndexTable) > 128) {
			return "", fmt.Errorf("unknown superclass constructor Exceptions metadata")
		}
	}
	exceptions, known := originalMethodExceptions(obj, m)
	if !known {
		return "", fmt.Errorf("unknown superclass constructor Exceptions metadata")
	}
	provider := c.FuncCtx.InvocationMetadata
	exhausted := false
	query := func(name string) (callbinding.Class, bool) {
		if !nativeProofWork(c.Work, 1) {
			exhausted = true
			return callbinding.Class{}, false
		}
		if provider == nil {
			return callbinding.Class{}, false
		}
		return provider(name)
	}
	var names []string
	for _, name := range exceptions {
		// Exception spelling supplies no subtype evidence. The same original
		// hierarchy used by checked-escape analysis proves unchecked subclasses,
		// including user-defined names, without changing the caller's declaration.
		unchecked := originalExceptionUnchecked(name, query)
		if exhausted || !nativeProofWork(c.Work, 1) {
			return "", fmt.Errorf("superclass constructor exception proof budget exceeded")
		}
		if unchecked {
			continue
		}
		throwable := originalExceptionCovered(name, "java/lang/Throwable", query)
		if exhausted || !nativeProofWork(c.Work, 1) {
			return "", fmt.Errorf("superclass constructor exception proof budget exceeded")
		}
		if !throwable {
			return "", fmt.Errorf("unknown superclass constructor exception hierarchy")
		}
		dot := strings.ReplaceAll(name, "/", ".")
		c.FuncCtx.Import(dot)
		short := c.FuncCtx.ShortTypeName(dot)
		if short != "" {
			names = append(names, short)
		}
	}
	return strings.Join(names, ", "), nil
}

func jdkSuperCtorThrows(super, desc string) string {
	super = strings.ReplaceAll(super, ".", "/")
	switch super + desc {
	case "java/io/ObjectInputStream(Ljava/io/InputStream;)V",
		"java/io/ObjectOutputStream(Ljava/io/OutputStream;)V",
		"java/io/ObjectInputStream()V",
		"java/io/ObjectOutputStream()V":
		return "IOException"
	}
	return ""
}
