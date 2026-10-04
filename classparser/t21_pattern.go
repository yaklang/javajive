package javaclassparser

import (
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func init() {
	PatternSwitchReconstructionComplete = patternSwitchReconstructionComplete
}

type patternWork struct {
	complete bool
	reason   string
}

var patternWorkByObj sync.Map // *ClassObject -> *patternWork

func patternSwitchReconstructionComplete(obj *ClassObject) bool {
	if obj == nil {
		return false
	}
	if d := dumperFor(obj); d != nil {
		applyPatternSwitchRewrites(d)
		if d.report != nil && d.report.Source != "" {
			d.report.Source = rewritePatternSwitchSource(d, d.report.Source)
		}
	}
	if v, ok := patternWorkByObj.Load(obj); ok {
		return v.(*patternWork).complete
	}
	return false
}

func applyPatternSwitchRewrites(c *ClassObjectDumper) {
	if c == nil || c.obj == nil {
		return
	}
	target := c.options.TargetSourceVersion
	if target == 0 {
		target = core.ClassMajorToSourceVersion(c.obj.MajorVersion)
	}
	hasType, hasEnum, hasComplex := classSwitchFamilies(c.obj)
	if hasEnum && !hasType {
		patternWorkByObj.Store(c.obj, &patternWork{complete: false, reason: "enumSwitch unsupported"})
		return
	}
	if !hasType {
		return
	}
	if target < 21 {
		patternWorkByObj.Store(c.obj, &patternWork{complete: false, reason: "target < 21"})
		return
	}
	if hasComplex {
		patternWorkByObj.Store(c.obj, &patternWork{complete: false, reason: "complex/deconstruction pattern"})
		return
	}

	allOK := true
	reason := ""
	rewrote := false
	for _, m := range c.obj.Methods {
		name, err1 := c.obj.getUtf8(m.NameIndex)
		desc, err2 := c.obj.getUtf8(m.DescriptorIndex)
		if err1 != nil || err2 != nil {
			continue
		}
		if !methodHasTypeSwitch(c.obj, m) {
			continue
		}
		body, ok, why := reconstructPatternMethod(c, m, name, desc)
		if !ok {
			allOK = false
			reason = why
			continue
		}
		rewrote = true
		key := fmt.Sprintf("name:%s,desc:%s", name, desc)
		if dumped, ok := c.dumpedMethodsSet[key]; ok && dumped != nil {
			dumped.code = replaceMethodBody(dumped.code, body)
			dumped.bodyCode = body
		}
	}
	if !rewrote {
		allOK = false
		if reason == "" {
			reason = "typeSwitch present but not reconstructed"
		}
	}
	patternWorkByObj.Store(c.obj, &patternWork{complete: allOK, reason: reason})
}

func rewritePatternSwitchSource(c *ClassObjectDumper, src string) string {
	if c == nil || c.obj == nil || src == "" {
		return src
	}
	for _, m := range c.obj.Methods {
		name, err1 := c.obj.getUtf8(m.NameIndex)
		desc, err2 := c.obj.getUtf8(m.DescriptorIndex)
		if err1 != nil || err2 != nil {
			continue
		}
		if !methodHasTypeSwitch(c.obj, m) {
			continue
		}
		body, ok, _ := reconstructPatternMethod(c, m, name, desc)
		if !ok {
			continue
		}
		key := fmt.Sprintf("name:%s,desc:%s", name, desc)
		header := ""
		if dumped, ok := c.dumpedMethodsSet[key]; ok && dumped != nil && dumped.code != "" {
			header = methodHeader(dumped.code)
		}
		if header == "" {
			continue
		}
		src = replaceSourceMethod(src, header, header+" {\n"+body+"\n\t}")
	}
	return src
}

func reconstructPatternMethod(c *ClassObjectDumper, m *MemberInfo, name, desc string) (string, bool, string) {
	code := methodCodeBytes(m)
	if len(code) == 0 {
		return "", false, "no code"
	}
	d := core.NewDecompiler(code, func(id int) values.JavaValue {
		return GetValueFromCP(c.obj.ConstantPool, id)
	})
	d.ConstantPoolLiteralGetter = func(id int) values.JavaValue {
		return GetLiteralFromCP(c.obj.ConstantPool, id)
	}
	d.ConstantPoolInvokeDynamicInfo = func(index int) (uint16, string, string) {
		constant := c.obj.ConstantPool[index-1]
		switch ret := constant.(type) {
		case *ConstantInvokeDynamicInfo:
			n, ds := getNameAndType(c.obj.ConstantPool, ret.NameAndTypeIndex)
			return ret.BootstrapMethodAttrIndex, n, ds
		default:
			return 0, "", ""
		}
	}
	attachBootstrap(d, c.obj)
	if c.FuncCtx != nil {
		d.FunctionContext = c.FuncCtx
		savedName := c.FuncCtx.FunctionName
		savedStatic := c.FuncCtx.IsStatic
		c.FuncCtx.FunctionName = name
		c.FuncCtx.IsStatic = m.AccessFlags&StaticFlag == StaticFlag
		defer func() {
			c.FuncCtx.FunctionName = savedName
			c.FuncCtx.IsStatic = savedStatic
		}()
	}
	rew, ok := core.TryReconstructPatternSwitch(d)
	if !ok || !rew.Complete || strings.TrimSpace(rew.Body) == "" {
		if rew.Reason != "" {
			return "", false, rew.Reason
		}
		return "", false, "pattern switch not reconstructed"
	}
	return rew.Body, true, ""
}

func methodHasTypeSwitch(obj *ClassObject, m *MemberInfo) bool {
	code := methodCodeBytes(m)
	if len(code) == 0 {
		return false
	}
	d := core.NewDecompiler(code, func(id int) values.JavaValue {
		return GetValueFromCP(obj.ConstantPool, id)
	})
	d.ConstantPoolInvokeDynamicInfo = func(index int) (uint16, string, string) {
		constant := obj.ConstantPool[index-1]
		switch ret := constant.(type) {
		case *ConstantInvokeDynamicInfo:
			n, ds := getNameAndType(obj.ConstantPool, ret.NameAndTypeIndex)
			return ret.BootstrapMethodAttrIndex, n, ds
		default:
			return 0, "", ""
		}
	}
	attachBootstrap(d, obj)
	if err := d.ParseOpcode(); err != nil {
		return false
	}
	for _, op := range d.Opcodes() {
		if op == nil || op.Instr == nil || op.Instr.OpCode != core.OP_INVOKEDYNAMIC || len(op.Data) < 2 {
			continue
		}
		index, name, _ := d.ConstantPoolInvokeDynamicInfo(int(core.Convert2bytesToInt(op.Data)))
		if name != "typeSwitch" {
			continue
		}
		if int(index) >= 0 && int(index) < len(d.BootstrapMethods) && d.BootstrapMethods[index] != nil {
			if mem, ok := d.BootstrapMethods[index].Ref.(*values.JavaClassMember); ok && mem != nil {
				id, err := core.IdentityFromMember(mem)
				if err == nil && id.Equal(core.IdentityTypeSwitch) {
					return true
				}
			}
		}
	}
	return false
}

func classSwitchFamilies(obj *ClassObject) (hasType, hasEnum, complex bool) {
	ids := classBootstrapIdentities(obj)
	for _, id := range ids {
		if fam, ok := core.LookupBuiltin(id); ok {
			switch fam {
			case core.FamilyTypeSwitch:
				hasType = true
			case core.FamilyEnumSwitch:
				hasEnum = true
			}
		}
	}
	for _, m := range obj.Methods {
		if methodHasMatchException(obj, m) {
			complex = true
		}
	}
	return hasType, hasEnum, complex
}

func methodHasMatchException(obj *ClassObject, m *MemberInfo) bool {
	code := methodCodeBytes(m)
	if len(code) == 0 {
		return false
	}
	d := core.NewDecompiler(code, func(id int) values.JavaValue {
		return GetValueFromCP(obj.ConstantPool, id)
	})
	if err := d.ParseOpcode(); err != nil {
		return false
	}
	for _, op := range d.Opcodes() {
		if op == nil || op.Instr == nil || len(op.Data) < 2 {
			continue
		}
		switch op.Instr.OpCode {
		case core.OP_NEW, core.OP_CHECKCAST, core.OP_INVOKESPECIAL:
			v := GetValueFromCP(obj.ConstantPool, int(core.Convert2bytesToInt(op.Data)))
			switch t := v.(type) {
			case *values.JavaClassValue:
				if t != nil && t.JavaType != nil && strings.Contains(strings.ReplaceAll(fmt.Sprint(t.JavaType), "/", "."), "MatchException") {
					return true
				}
			case *values.JavaClassMember:
				if t != nil && strings.Contains(strings.ReplaceAll(t.Name, "/", "."), "MatchException") {
					return true
				}
			}
		}
	}
	return false
}

func replaceMethodBody(methodCode, newBody string) string {
	i := strings.Index(methodCode, "{")
	if i < 0 {
		return methodCode
	}
	header := strings.TrimRight(methodCode[:i], " \t")
	return header + " {\n\t" + newBody + "\n}"
}

func methodHeader(methodCode string) string {
	i := strings.Index(methodCode, "{")
	if i < 0 {
		return strings.TrimSpace(methodCode)
	}
	return strings.TrimSpace(methodCode[:i])
}

func replaceSourceMethod(src, header, replacement string) string {
	idx := strings.Index(src, header)
	if idx < 0 {
		return src
	}
	rest := src[idx:]
	brace := strings.Index(rest, "{")
	if brace < 0 {
		return src
	}
	end, ok := matchingBrace(rest, brace)
	if !ok {
		return src
	}
	return src[:idx] + replacement + rest[end+1:]
}

func matchingBrace(s string, open int) (int, bool) {
	depth := 0
	inStr := byte(0)
	esc := false
	for i := open; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == inStr {
				inStr = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = c
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}
