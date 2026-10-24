package javaclassparser

import (
	"bytes"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

// ACC_BRIDGE alone must never make a candidate disappear. These mutations
// preserve the declaration shape while breaking receiver/argument identity or
// invocation semantics. They exercise the original bytes -> metadata -> source
// selection path, not a manually supplied positive certificate.
func TestInvocationCovariantBridgeRequiresOriginalTypedForwarder(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		compiler := modern
		if profile == NativeJavac8 {
			compiler = os.Getenv("JAVA8_JAVAC")
			if compiler == "" {
				t.Fatal("native compiler required")
			}
		}
		t.Run(string(profile), func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, compiler, profile, `class ForwardBase {public Object append(Object a,Object b,Object...items){return items;}}
public class ForwardOwner extends ForwardBase {public Object[] append(Object a,Object b,Object...items){return items;}}`, "ForwardOwner", "source,lines,vars", true)
			for _, variant := range []string{"original", "explicit local loads", "wrong receiver", "swapped arguments", "special dispatch", "static dispatch", "extra instruction", "handler", "no synthetic flag", "no bridge flag", "missing target", "wrong parent access"} {
				t.Run(variant, func(t *testing.T) {
					object := mustParseLowering(t, bytes.Clone(files["ForwardOwner.class"]))
					var bridge *MemberInfo
					for _, m := range object.Methods {
						if m.AccessFlags&0x40 != 0 {
							if bridge != nil {
								t.Fatal("duplicate compiler bridge")
							}
							bridge = m
						}
					}
					if bridge == nil {
						t.Fatal("original compiler bridge missing")
					}
					code := constructorLoweringCode(bridge)
					if len(code.Code) != 8 || !bytes.Equal(code.Code[:4], []byte{core.OP_ALOAD_0, core.OP_ALOAD_1, core.OP_ALOAD_2, core.OP_ALOAD_3}) || code.Code[4] != core.OP_INVOKEVIRTUAL {
						t.Fatal("authored forwarding shape")
					}
					dependencies := map[string][]byte{}
					for n, b := range files {
						dependencies[n] = bytes.Clone(b)
					}
					switch variant {
					case "explicit local loads":
						code.Code = append([]byte{core.OP_ALOAD, 0, core.OP_ALOAD, 1, core.OP_ALOAD, 2, core.OP_ALOAD, 3}, code.Code[4:]...)
					case "wrong receiver":
						code.Code[0] = core.OP_ALOAD_1
					case "swapped arguments":
						code.Code[1], code.Code[2] = code.Code[2], code.Code[1]
					case "special dispatch":
						code.Code[4] = core.OP_INVOKESPECIAL
					case "static dispatch":
						code.Code[4] = core.OP_INVOKESTATIC
					case "extra instruction":
						code.Code = append([]byte{core.OP_NOP}, code.Code...)
					case "handler":
						code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: 4, HandlerPc: 7, CatchType: 0})
					case "no synthetic flag":
						bridge.AccessFlags &^= 0x1000
					case "no bridge flag":
						bridge.AccessFlags &^= 0x40
					case "missing target":
						var kept []*MemberInfo
						for _, m := range object.Methods {
							n, _ := sourceBridgeUTF8(object, m.NameIndex)
							if n != "append" || m == bridge {
								kept = append(kept, m)
							}
						}
						object.Methods = kept
					case "wrong parent access":
						base := mustParseLowering(t, dependencies["ForwardBase.class"])
						for _, m := range base.Methods {
							n, _ := sourceBridgeUTF8(base, m.NameIndex)
							if n == "append" {
								m.AccessFlags &^= 1
							}
						}
						dependencies["ForwardBase.class"] = base.Bytes()
					}
					d := NewClassObjectDumper(object)
					d.options.TargetSourceVersion, d.options.SourceCompiler = 8, profile
					d.foldSiblingResolver = func(n string) ([]byte, bool) { b, k := dependencies[n+".class"]; return b, k }
					plan := callbinding.SelectVariableArityReference(callbinding.Witness{Owner: "ForwardOwner", Name: "append", Desc: "(Ljava/lang/Object;Ljava/lang/Object;[Ljava/lang/Object;)[Ljava/lang/Object;", Kind: callbinding.Virtual}, "(Ljava/lang/Object;Ljava/lang/Object;Ljava/lang/Object;)V", d.buildInvocationMetadata(), nil)
					want := variant == "original" || variant == "explicit local loads"
					if plan.Supported != want {
						t.Fatalf("%+v wanted supported=%v", plan, want)
					}
				})
			}
		})
	}
}
