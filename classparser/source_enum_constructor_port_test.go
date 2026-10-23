package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func enumPortTestInputs(t *testing.T, debug string) (map[string][]byte, map[string][]byte, *JarReconstructionArtifact) {
	t.Helper()
	return enumFlatTestInputs(t, debug, flatEnumPortFixture("PortOwner"))
}

func enumFlatTestInputs(t *testing.T, debug, sourceText string) (map[string][]byte, map[string][]byte, *JarReconstructionArtifact) {
	t.Helper()
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, sourceText, "PortOwner", debug, false)
	owned, external := map[string][]byte{}, map[string][]byte{}
	for n, b := range files {
		if strings.HasPrefix(n, "PortOwner$Mode") {
			owned[n] = b
		} else {
			external[n] = b
		}
	}
	jar := filepath.Join(t.TempDir(), "enum.jar")
	if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	source, err := a.ReadFile("PortOwner$Mode.class")
	if err != nil || legacySourceHasFailureMarker(string(source)) {
		t.Fatal("source", err, string(source))
	}
	out := sourceArtifactWriteFiles(t, external)
	file := filepath.Join(out, "PortOwner$Mode.java")
	if err := os.WriteFile(file, source, 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, file).CombinedOutput(); err != nil {
		t.Fatal("fresh", err, string(log), string(source))
	}
	fresh := map[string][]byte{}
	for n := range owned {
		b, err := os.ReadFile(filepath.Join(out, n))
		if err != nil {
			t.Fatal(err)
		}
		fresh[n] = b
	}
	return owned, fresh, a
}

func enumPortFixCodeLength(t *testing.T, object *ClassObject, code *CodeAttribute) {
	t.Helper()
	code.AttrLen = uint32(12 + len(code.Code) + 8*len(code.ExceptionTable))
	for _, a := range code.Attributes {
		switch x := a.(type) {
		case *UnparsedAttribute:
			object.ConstantPoolManager.AddUtf8Info(x.Name)
			code.AttrLen += 6 + x.Length
		case *LineNumberTableAttribute:
			code.AttrLen += 6 + x.AttrLen
		default:
			t.Fatalf("unhandled test attribute %T", a)
		}
	}
}

func TestAdversarialEnumConstructorPortRejectsUnprovedConsumers(t *testing.T) {
	originals, fresh, a := enumPortTestInputs(t, "source,lines,vars")
	for _, kind := range []string{"control", "receiver", "wide category", "null port", "extra instruction", "handler", "max stack", "max locals", "wrong invoke", "method handle", "broken handle target", "additional consumer", "class identity", "unreturned source", "pending source", "work", "output", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			const name = "PortOwner$Mode$1.class"
			object := mustParseLowering(t, bytes.Clone(fresh[name]))
			var ctor *MemberInfo
			for _, m := range object.Methods {
				n, _ := sourceBridgeUTF8(object, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
			}
			call := enumConstructorPortForward(object, ctor, "PortOwner$Mode", nil)
			if call == nil {
				t.Fatal("authored exact forward control")
			}
			code := constructorLoweringCode(ctor)
			ops, ok := nativeEnumMethodOps(object, ctor, nil)
			if !ok {
				t.Fatal("decoded constructor")
			}
			authority := a.source.jar.sourceConstructorLowering
			var work *workbudget.Budget
			switch kind {
			case "receiver":
				code.Code[0] = core.OP_ALOAD_1
			case "wide category":
				for _, op := range ops {
					if op.Instr.OpCode == core.OP_LLOAD {
						code.Code[int(op.CurrentOffset)] = core.OP_DLOAD
						break
					}
				}
			case "null port":
				code.Code[int(ops[len(ops)-3].CurrentOffset)] = core.OP_ALOAD_0
			case "extra instruction":
				code.Code = append([]byte{core.OP_NOP}, code.Code...)
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: uint16(len(code.Code) - 1)}}
			case "max stack":
				code.MaxStack = 0
			case "max locals":
				code.MaxLocals = 0
			case "wrong invoke":
				code.Code[call.pc] = core.OP_INVOKEVIRTUAL
			case "method handle":
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: call.index})
			case "broken handle target":
				ref := *object.ConstantPool[call.index-1].(*ConstantMethodrefInfo)
				ref.NameAndTypeIndex = 0
				object.ConstantPool = append(object.ConstantPool, &ref)
				index := uint16(len(object.ConstantPool))
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: index})
			case "additional consumer":
				copy := *ctor
				copy.NameIndex = uint16(object.ConstantPoolManager.AddUtf8Info("additionalConsumer"))
				object.Methods = append(object.Methods, &copy)
			case "class identity":
				object.ThisClass = uint16(object.ConstantPoolManager.AddNewClassInfo("UnrelatedOwner"))
			case "unreturned source":
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar}
			case "pending source":
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, parent: authority, enumPending: map[string]constructorNativeSourceOrigin{"PortOwner$Mode": {}}}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "output":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			enumPortFixCodeLength(t, object, code)
			input := object.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("structural negative", err)
			}
			before := bytes.Clone(input)
			done, err := authority.lowerEnumConstructorPort(originals[name], input, work)
			if kind == "control" {
				if err != nil || bytes.Equal(input, done) {
					t.Fatal("positive null forward rewrite", err)
				}
				if _, err := a.FinishClass(name, input); err != nil {
					t.Fatal("full completion", err)
				}
			} else if err == nil && !bytes.Equal(input, done) {
				t.Fatal("unproved caller rewritten")
			} else if err != nil && done != nil {
				t.Fatal("partial result")
			}
			if (kind == "work" || kind == "output" || kind == "canceled") && err == nil {
				t.Fatal("budget bypass")
			}
			if !bytes.Equal(input, before) {
				t.Fatal("mutated input")
			}
		})
	}
}

func TestAdversarialEnumConstructorPortRequiresIgnoredOriginalAndFreshSlot(t *testing.T) {
	originals, fresh, a := enumPortTestInputs(t, "source,lines,vars")
	const name = "PortOwner$Mode.class"
	for _, kind := range []string{"control", "fresh reads port", "original reads port", "fresh target visibility", "fresh marker primitive", "fresh extra constructor", "fresh unknown debug", "fresh wrong local descriptor", "stale original", "fresh preview"} {
		t.Run(kind, func(t *testing.T) {
			want, obj := mustParseLowering(t, bytes.Clone(originals[name])), mustParseLowering(t, bytes.Clone(fresh[name]))
			authority := a.source.jar.sourceConstructorLowering
			selectBridge := func(o *ClassObject) *nativeConstructorAccessBridge {
				d := NewClassObjectDumper(o)
				b := d.originalNativeConstructorAccessBridges()
				if len(b) != 1 {
					t.Fatal("single exact bridge control", len(b))
				}
				for _, v := range b {
					return v
				}
				return nil
			}
			old, b := selectBridge(want), selectBridge(obj)
			body := constructorLoweringCode(b.method)
			switch kind {
			case "fresh reads port":
				body.Code = append([]byte{core.OP_ALOAD, byte(body.MaxLocals - 1), core.OP_POP}, body.Code...)
			case "original reads port":
				c := constructorLoweringCode(old.method)
				c.Code = append([]byte{core.OP_ALOAD, byte(c.MaxLocals - 1), core.OP_POP}, c.Code...)
				enumPortFixCodeLength(t, want, c)
			case "fresh target visibility":
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
					if n == "<init>" && d == b.target {
						m.AccessFlags = 1
					}
				}
			case "fresh marker primitive":
				b.method.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info(strings.TrimSuffix(b.target, ")V") + "I)V"))
			case "fresh extra constructor":
				copy := *b.method
				copy.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("(Ljava/lang/String;IIJDLBox;Ljava/lang/Object;)V"))
				obj.Methods = append(obj.Methods, &copy)
			case "fresh unknown debug":
				body.Attributes = append(body.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleTypeAnnotations", Length: 2, Info: []byte{0, 0}})
			case "fresh wrong local descriptor":
				changed := false
				for _, attr := range body.Attributes {
					if raw, ok := attr.(*UnparsedAttribute); ok && raw.Name == "LocalVariableTable" {
						for pos := 2; pos < len(raw.Info); pos += 10 {
							if binary.BigEndian.Uint16(raw.Info[pos+8:]) == body.MaxLocals-1 {
								binary.BigEndian.PutUint16(raw.Info[pos+6:], uint16(obj.ConstantPoolManager.AddUtf8Info("Ljava/lang/Object;")))
								changed = true
							}
						}
					}
				}
				if !changed {
					t.Fatal("missing last-slot local control")
				}
			case "stale original":
				want.ConstantPoolManager.AddUtf8Info("stale original")
			case "fresh preview":
				obj.MinorVersion = 65535
			}
			enumPortFixCodeLength(t, obj, body)
			original, input := want.Bytes(), obj.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("fresh parse", err)
			}
			if _, err := Parse(original); err != nil {
				t.Fatal("original parse", err)
			}
			if kind == "original reads port" {
				origin := authority.enumActive["PortOwner$Mode"]
				origin.originalHash = sha256.Sum256(original)
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, enumActive: map[string]constructorNativeSourceOrigin{"PortOwner$Mode": origin}}
			}
			before := bytes.Clone(input)
			done, err := authority.lowerEnumConstructorPort(original, input, nil)
			if kind == "control" {
				if err != nil || bytes.Equal(done, input) {
					t.Fatal("positive ignored slot", err)
				}
			} else if err == nil && !bytes.Equal(done, input) {
				t.Fatal("unproved ignored slot rewritten")
			} else if err != nil && done != nil {
				t.Fatal("partial result")
			}
			if !bytes.Equal(input, before) {
				t.Fatal("mutated input")
			}
		})
	}
}
