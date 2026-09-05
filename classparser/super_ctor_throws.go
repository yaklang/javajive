package javaclassparser

import (
	"os"
	"strings"
)

// superCtorCheckedThrows returns the checked exceptions declared by the
// superclass constructor this `<init>` invokes, when this constructor itself
// has no Exceptions attribute. javac (classfile 49, junit MaxCore$1$1) omits
// Exceptions on the synthetic anonymous-class constructor even though
// `super(...)` throws a checked exception, so the decompiled ctor fails
// "unreported exception InitializationError". Kill-switch:
// JDEC_SUPER_CTOR_THROWS_OFF=1.
func (c *ClassObjectDumper) superCtorCheckedThrows() string {
	if os.Getenv("JDEC_SUPER_CTOR_THROWS_OFF") == "1" {
		return ""
	}
	if c == nil || c.obj == nil {
		return ""
	}
	super := c.obj.GetSupperClassName()
	if super == "" || super == "java/lang/Object" {
		return ""
	}
	desc := c.superInitDescriptor(super)
	if desc == "" {
		return ""
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
		return extra
	}
	if c.foldSiblingResolver == nil {
		return ""
	}
	data, ok := c.foldSiblingResolver(super)
	if !ok || len(data) == 0 {
		return ""
	}
	sObj, err := Parse(data)
	if err != nil || sObj == nil {
		return ""
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
	return ""
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

func (c *ClassObjectDumper) checkedThrowsFromMethod(obj *ClassObject, m *MemberInfo) string {
	if obj == nil || m == nil || c.FuncCtx == nil {
		return ""
	}
	var names []string
	for _, attribute := range m.Attributes {
		ea, ok := attribute.(*ExceptionsAttribute)
		if !ok {
			continue
		}
		for _, u := range ea.ExceptionIndexTable {
			info, err := obj.getConstantInfo(u)
			if err != nil {
				continue
			}
			classInfo, ok := info.(*ConstantClassInfo)
			if !ok {
				continue
			}
			name, err := obj.getUtf8(classInfo.NameIndex)
			if err != nil || name == "" {
				continue
			}
			dot := strings.ReplaceAll(name, "/", ".")
			if isUncheckedThrowable(dot) {
				continue
			}
			c.FuncCtx.Import(dot)
			short := c.FuncCtx.ShortTypeName(dot)
			if short != "" {
				names = append(names, short)
			}
		}
	}
	return strings.Join(names, ", ")
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

func isUncheckedThrowable(dot string) bool {
	switch dot {
	case "java.lang.RuntimeException", "java.lang.Error":
		return true
	}
	if strings.HasPrefix(dot, "java.lang.") {
		switch {
		case strings.HasSuffix(dot, "Error"),
			strings.HasSuffix(dot, "RuntimeException"):
			return true
		}
	}
	return false
}
