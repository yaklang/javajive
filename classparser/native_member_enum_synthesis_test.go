package javaclassparser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// These mutations independently distinguish the compiler-generated protocol
// from source spelling and flags. Replacing a factory, constant name/ordinal,
// backing-array order or hidden parameter use must invalidate the certificate.
func TestNativeMemberEnumSynthesisRequiresOriginalProtocol(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileDebugClasses(t, nativeEnumScopeFixture, debug)
		for _, variant := range []string{"original", "non-enum class", "abstract member", "nonstatic member", "nonfinal member", "mutable backing", "ordinary backing", "backing attribute", "duplicate backing", "ordinary constant", "duplicate factory", "factory flags", "factory handler", "no factory code", "duplicate factory code", "unknown factory attribute", "nil signature", "named mandated parameter", "wrong parameter flags", "no clone", "constant name", "constant ordinal", "constant allocation", "array order", "hidden parameter", "factory stack", "factory locals", "constructor stack", "constructor locals", "initializer stack", "array stack", "factory code annotation", "opaque factory code attribute", "malformed factory debug locals", "budget", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, err := Parse(files["EnumScopeOwner$Mode.class"])
				if err != nil {
					t.Fatal(err)
				}
				flags := uint16(0x4018)
				methods := map[string]*MemberInfo{}
				codes := map[string]*CodeAttribute{}
				var backing, constant *MemberInfo
				for _, field := range obj.Fields {
					if field.AccessFlags&0x1000 != 0 {
						backing = field
					}
					if constant == nil && field.AccessFlags&0x4000 != 0 {
						constant = field
					}
				}
				for _, method := range obj.Methods {
					name, _ := sourceBridgeUTF8(obj, method.NameIndex)
					methods[name] = method
					for _, attribute := range method.Attributes {
						if code, ok := attribute.(*CodeAttribute); ok {
							codes[name] = code
						}
					}
				}
				if backing == nil || constant == nil || codes["values"] == nil || codes["$values"] == nil || codes["<init>"] == nil || codes["<clinit>"] == nil {
					t.Fatal("original enum protocol missing")
				}
				var work *workbudget.Budget
				switch variant {
				case "non-enum class":
					obj.AccessFlags &^= 0x4000
				case "abstract member":
					flags |= 0x400
				case "nonstatic member":
					flags &^= 8
				case "nonfinal member":
					flags &^= 16
				case "mutable backing":
					backing.AccessFlags &^= 16
				case "ordinary backing":
					backing.AccessFlags &^= 0x1000
				case "backing attribute":
					backing.Attributes = append(backing.Attributes, &DeprecatedAttribute{})
				case "duplicate backing":
					obj.Fields = append(obj.Fields, backing)
				case "ordinary constant":
					constant.AccessFlags &^= 0x4000
				case "duplicate factory":
					obj.Methods = append(obj.Methods, methods["values"])
				case "factory flags":
					methods["values"].AccessFlags |= 0x1000
				case "factory handler":
					codes["values"].ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 3, HandlerPc: 0}}
				case "factory stack":
					codes["valueOf"].MaxStack = 1
				case "factory locals":
					codes["valueOf"].MaxLocals = 0
				case "constructor stack":
					codes["<init>"].MaxStack = 2
				case "constructor locals":
					codes["<init>"].MaxLocals = 2
				case "initializer stack":
					codes["<clinit>"].MaxStack = 3
				case "array stack":
					codes["$values"].MaxStack = 3
				case "factory code annotation":
					codes["values"].Attributes = append(codes["values"].Attributes, &RuntimeVisibleTypeAnnotationsAttribute{})
				case "opaque factory code attribute":
					codes["values"].Attributes = append(codes["values"].Attributes, &UnparsedAttribute{Name: "OpaqueFactory", Length: 0})
				case "malformed factory debug locals":
					codes["values"].Attributes = append(codes["values"].Attributes, &UnparsedAttribute{Name: "LocalVariableTable", Length: 2, Info: []byte{0, 1}})
				case "no factory code":
					methods["values"].Attributes = nil
				case "duplicate factory code":
					methods["values"].Attributes = append(methods["values"].Attributes, codes["values"])
				case "unknown factory attribute":
					methods["values"].Attributes = append(methods["values"].Attributes, &DeprecatedAttribute{})
				case "nil signature":
					methods["<init>"].Attributes = append(methods["<init>"].Attributes, (*SignatureAttribute)(nil))
				case "named mandated parameter", "wrong parameter flags":
					seen := false
					for _, attribute := range methods["valueOf"].Attributes {
						if parameters, ok := attribute.(*UnparsedAttribute); ok && parameters.Name == "MethodParameters" {
							seen = true
							if variant == "named mandated parameter" {
								parameters.Info[2] = byte(methods["valueOf"].NameIndex)
							} else {
								parameters.Info[3] = 0
							}
						}
					}
					if !seen {
						t.Fatal("original mandated parameter missing")
					}
				case "no clone":
					codes["values"].Code = append(append([]byte(nil), codes["values"].Code[:3]...), byte(core.OP_ARETURN))
				case "constant name":
					codes["<clinit>"].Code[5] = 0
				case "constant ordinal":
					codes["<clinit>"].Code[6] = byte(core.OP_ICONST_1)
				case "constant allocation":
					codes["<clinit>"].Code[0] = byte(core.OP_CHECKCAST)
				case "array order":
					ops, known := nativeEnumMethodOps(obj, methods["$values"], nil)
					if !known || len(ops) != 11 {
						t.Fatal("original backing-array packet")
					}
					// Keep the array stores and change which original field supplies slot 0.
					index, known := nativeEnumCPIndex(ops[8])
					if !known {
						t.Fatal("second original constant")
					}
					for i := 0; i+2 < len(codes["$values"].Code); i++ {
						if codes["$values"].Code[i] == byte(core.OP_GETSTATIC) {
							codes["$values"].Code[i+1], codes["$values"].Code[i+2] = byte(index>>8), byte(index)
							break
						}
					}
				case "hidden parameter":
					code := codes["<init>"]
					code.Code = append(append([]byte(nil), code.Code[:len(code.Code)-1]...), byte(core.OP_ALOAD_1), byte(core.OP_POP), byte(core.OP_RETURN))
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				if accepted := nativeMemberEnumSynthesisProof(obj, flags, work) != nil; accepted != (variant == "original") {
					t.Fatalf("original protocol admitted=%v", accepted)
				}
			})
		}
	}
}

// A valid JVM class can implement values() with an exposed backing array.
// Java source cannot redeclare that factory, so rebuilding it as an enum would
// silently insert a clone. Validate the original executable counterexample,
// then require both protocol and ownership admission to refuse it.
func TestNativeMemberEnumRefusesValidNoncanonicalFactory(t *testing.T) {
	const source = `class FactoryOwner{enum Choice{LEFT,RIGHT;}}class FactoryDriver{public static void main(String[]args){if(FactoryOwner.Choice.values()!=FactoryOwner.Choice.values())throw new AssertionError("original array identity");FactoryOwner.Choice.values()[0]=null;if(FactoryOwner.Choice.values()[0]!=null)throw new AssertionError("original shared mutation");System.out.println("original:shared:array");}}`
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, source, debug)
			obj, err := Parse(files["FactoryOwner$Choice.class"])
			if err != nil {
				t.Fatal(err)
			}
			if nativeMemberEnumSynthesisProof(obj, 0x4018, nil) == nil {
				t.Fatal("canonical compiler profile missing")
			}
			modified := false
			for _, method := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if name != "values" {
					continue
				}
				for _, attr := range method.Attributes {
					code, ok := attr.(*CodeAttribute)
					if !ok {
						continue
					}
					if len(code.Code) != 10 || code.Code[0] != byte(core.OP_GETSTATIC) {
						t.Fatal("original clone factory")
					}
					code.Code = append(append([]byte(nil), code.Code[:3]...), byte(core.OP_ARETURN))
					code.Attributes, code.ExceptionTable = nil, nil
					code.MaxStack, code.MaxLocals, code.AttrLen = 1, 0, uint32(12+len(code.Code))
					modified = true
				}
			}
			if !modified {
				t.Fatal("factory not modified")
			}
			files["FactoryOwner$Choice.class"] = obj.Bytes()
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "FactoryDriver"); got != "original:shared:array\n" {
				t.Fatalf("independent original oracle=%q", got)
			}
			parsed, err := Parse(files["FactoryOwner$Choice.class"])
			if err != nil {
				t.Fatal(err)
			}
			if nativeMemberEnumSynthesisProof(parsed, 0x4018, nil) != nil || nativeMemberProof(parsed, nil) != nil {
				t.Fatal("noncanonical original factory certified for regeneration")
			}
			z := nativeArchive(t, files)
			defer z.Close()
			entry := z.nativeMemberEntry(parsed)
			if entry != nil && entry.family != nil {
				t.Fatal("noncanonical original factory published through archive ownership")
			}
			// Refusing the ownership plan is insufficient: the ordinary raw
			// renderer must not silently regenerate a different values() body.
			if _, err := NewClassObjectDumper(parsed).DumpClass(); err == nil || !strings.Contains(err.Error(), "enum regeneration") {
				t.Fatalf("raw renderer must refuse semantic replacement: %v", err)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					fresh := nativeArchive(t, files)
					defer fresh.Close()
					raw, err := fresh.ReadFile("FactoryOwner$Choice.class")
					if err != nil || !strings.Contains(string(raw), "enum regeneration") || !strings.Contains(string(raw), "decompile dump failed") {
						t.Fatalf("archive raw fallback must refuse replacement: %v\n%s", err, raw)
					}
				})
			}
		})
	}
}

// This valid factory ignores its input, including null. A name-only deletion
// would change both return identity and exception priority on recompile.
func TestNativeEnumRefusesValidNoncanonicalValueOf(t *testing.T) {
	const fixture = `enum LiteralFactoryChoice{LEFT,RIGHT;}class LiteralFactoryDriver{public static void main(String[]args){for(String s:new String[]{null,"LEFT","missing"})if(LiteralFactoryChoice.valueOf(s)!=LiteralFactoryChoice.RIGHT)throw new AssertionError("original literal factory");System.out.println("literal:factory:null:identity");}}`
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			obj, err := Parse(files["LiteralFactoryChoice.class"])
			if err != nil {
				t.Fatal(err)
			}
			var fieldIndex uint16
			for i, entry := range obj.ConstantPool {
				ref, ok := entry.(*ConstantFieldrefInfo)
				if !ok || ref == nil {
					continue
				}
				owner, closed := sourceBridgeClassName(obj, ref.ClassIndex)
				if !closed || owner != obj.GetClassName() {
					continue
				}
				nt, ok := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				if !ok || nt == nil {
					continue
				}
				n, _ := sourceBridgeUTF8(obj, nt.NameIndex)
				d, _ := sourceBridgeUTF8(obj, nt.DescriptorIndex)
				if n == "RIGHT" && d == "LLiteralFactoryChoice;" {
					fieldIndex = uint16(i + 1)
				}
			}
			if fieldIndex == 0 {
				t.Fatal("original constant field absent")
			}
			modified := false
			for _, method := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if n != "valueOf" {
					continue
				}
				for _, attribute := range method.Attributes {
					if code, ok := attribute.(*CodeAttribute); ok {
						code.Code = []byte{byte(core.OP_GETSTATIC), byte(fieldIndex >> 8), byte(fieldIndex), byte(core.OP_ARETURN)}
						code.Attributes, code.ExceptionTable = nil, nil
						code.MaxStack, code.MaxLocals, code.AttrLen = 1, 1, 16
						modified = true
					}
				}
			}
			if !modified {
				t.Fatal("original factory absent")
			}
			files["LiteralFactoryChoice.class"] = obj.Bytes()
			original := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "LiteralFactoryDriver"); got != "literal:factory:null:identity\n" {
				t.Fatalf("independent oracle=%q", got)
			}
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					defer z.Close()
					raw, err := z.ReadFile("LiteralFactoryChoice.class")
					if err != nil || !strings.Contains(string(raw), "enum regeneration") || !strings.Contains(string(raw), "decompile dump failed") {
						t.Fatalf("noncanonical valueOf must fail honestly: %v\n%s", err, raw)
					}
				})
			}
		})
	}
}

func TestNativeEnumFactoryNamedParameterMetadataIsNotCertified(t *testing.T) {
	const fixture = `enum NamedChoice{LEFT,RIGHT;}class NamedFactoryDriver{public static void main(String[]args)throws Exception{java.lang.reflect.Parameter p=NamedChoice.class.getDeclaredMethod("valueOf",String.class).getParameters()[0];if(!p.isNamePresent()||!p.getName().equals("valueOf")||NamedChoice.valueOf("LEFT")!=NamedChoice.LEFT)throw new AssertionError("original named factory");System.out.println("named:factory:original:identity");}}`
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			obj, err := Parse(files["NamedChoice.class"])
			if err != nil {
				t.Fatal(err)
			}
			modified := false
			for _, method := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if n != "valueOf" {
					continue
				}
				for _, a := range method.Attributes {
					if parameters, ok := a.(*UnparsedAttribute); ok && parameters.Name == "MethodParameters" {
						parameters.Info[1], parameters.Info[2] = byte(method.NameIndex>>8), byte(method.NameIndex)
						modified = true
					}
				}
			}
			if !modified {
				t.Fatal("original mandated factory parameter missing")
			}
			raw := obj.Bytes()
			files["NamedChoice.class"] = raw
			original := t.TempDir()
			for n, b := range files {
				if e := os.WriteFile(filepath.Join(original, n), b, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if got := t04RunJava(t, java, original, "NamedFactoryDriver"); got != "named:factory:original:identity\n" {
				t.Fatalf("original metadata oracle=%q", got)
			}
			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					result, err := DecompileWithOptions(raw, DecompileOptions{Mode: mode})
					if err != nil || result.Source == "" || result.Status != "unsupported" || len(result.StubMethods) != 0 {
						t.Fatalf("execution proof must retain metadata limitation: %v %+v", err, result)
					}
					found := false
					for _, d := range result.Diagnostics {
						found = found || d.Code == "enum_factory_parameter_metadata" && d.Method == "valueOf(Ljava/lang/String;)LNamedChoice;"
					}
					if !found {
						t.Fatalf("missing metadata diagnostic:%+v", result.Diagnostics)
					}
				})
			}
		})
	}
}
