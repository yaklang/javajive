package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialInheritedBridgeRequiresExactTypedSuperForwarding(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, inheritedCovariantBridgeFixture, "BridgeOuter", "none", false)
	for _, kind := range []string{"control", "not bridge", "wrong receiver", "wrong wide slot", "virtual target", "effectful prefix", "wrong return", "handler", "missing parent", "foreign parent", "final target", "generic target", "duplicate method", "own narrow declaration", "budget", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			o := mustParseLowering(t, bytes.Clone(files["BridgeOuter$Leaf.class"]))
			parent := mustParseLowering(t, bytes.Clone(files["BridgeOuter.class"]))
			var m, target *MemberInfo
			for _, candidate := range o.Methods {
				if candidate.AccessFlags == 0x1041 {
					m = candidate
				}
			}
			for _, candidate := range parent.Methods {
				n, _ := sourceBridgeUTF8(parent, candidate.NameIndex)
				d, _ := sourceBridgeUTF8(parent, candidate.DescriptorIndex)
				if n == "read" && d == "(JDLjava/lang/Object;)Ljava/lang/String;" {
					target = candidate
				}
			}
			if m == nil || target == nil {
				t.Fatal("original actual bridge")
			}
			code := constructorLoweringCode(m)
			d := core.NewDecompiler(code.Code, nil)
			if d.ParseOpcode() != nil {
				t.Fatal("original packet")
			}
			ops := constructorMotionOps(d)
			var work *workbudget.Budget
			switch kind {
			case "not bridge":
				m.AccessFlags = 1
			case "wrong receiver":
				code.Code[0] = core.OP_ALOAD_1
			case "wrong wide slot":
				code.Code[int(ops[1].CurrentOffset)] = core.OP_LLOAD_2
			case "virtual target":
				code.Code[int(ops[len(ops)-2].CurrentOffset)] = core.OP_INVOKEVIRTUAL
			case "effectful prefix":
				code.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, code.Code...)
			case "wrong return":
				code.Code[len(code.Code)-1] = core.OP_IRETURN
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 0}}
			case "foreign parent":
				parent.ThisClass = uint16(parent.ConstantPoolManager.AddNewClassInfo("AnotherParent"))
			case "final target":
				target.AccessFlags |= 0x10
			case "generic target":
				target.Attributes = append(target.Attributes, &SignatureAttribute{SignatureIndex: uint16(parent.ConstantPoolManager.AddUtf8Info("(JDLjava/lang/Object;)Ljava/lang/String;"))})
			case "duplicate method":
				o.Methods = append(o.Methods, m)
			case "own narrow declaration":
				copy := *m
				copy.AccessFlags = 1
				copy.DescriptorIndex = uint16(o.ConstantPoolManager.AddUtf8Info("(JDLjava/lang/Object;)Ljava/lang/String;"))
				o.Methods = append(o.Methods, &copy)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			reader := NewClassObjectDumper(o)
			reader.Work = work
			reader.foldSiblingResolver = func(n string) ([]byte, bool) {
				if kind == "missing parent" {
					return nil, false
				}
				if n == "BridgeOuter" {
					return parent.Bytes(), true
				}
				b, k := files[n+".class"]
				return b, k
			}
			if (reader.inheritedBridgeOriginal(m) != nil) != (kind == "control") {
				t.Fatal("unproved super binding", kind)
			}
		})
	}
}

func TestAdversarialInheritedBridgeFreshInliningRequiresUniqueConsumerAndPublication(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, inheritedCovariantBridgeFixture, "BridgeOuter", "none", false)
	const owner = "BridgeOuter$Leaf"
	original := files[owner+".class"]
	jar := filepath.Join(t.TempDir(), "input.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": original}), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, func(n string) ([]byte, bool) { b, k := files[n+".class"]; return b, k })
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	source, err := a.ReadFile(owner + ".class")
	if err != nil {
		t.Fatal(err)
	}
	external := map[string][]byte{}
	for n, b := range files {
		if n != owner+".class" {
			external[n] = b
		}
	}
	out := sourceArtifactWriteFiles(t, external)
	path := filepath.Join(out, owner+".java")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:none", "-cp", out, "-sourcepath", out, "-d", out, path).CombinedOutput(); err != nil {
		t.Fatal("actual fresh helper compiler", err, string(log))
	}
	compiled, err := os.ReadFile(filepath.Join(out, owner+".class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"control", "unreturned source", "pending namespace", "stale original", "helper effects", "virtual super call", "wrong bridge slot", "nonbridge flags", "other consumer", "method handle", "changed throws", "invalid parameter table", "duplicate parameter table", "unknown attribute", "budget", "canceled", "output"} {
		t.Run(kind, func(t *testing.T) {
			o := mustParseLowering(t, bytes.Clone(compiled))
			var helper, bridge, ctor *MemberInfo
			for _, m := range o.Methods {
				n, _ := sourceBridgeUTF8(o, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
				if n == "read" {
					if m.AccessFlags == 0x1041 {
						bridge = m
					} else {
						helper = m
					}
				}
			}
			if helper == nil || bridge == nil || ctor == nil {
				t.Fatal("real fresh narrow declaration and generated bridge", string(source))
			}
			hc, bc := constructorLoweringCode(helper), constructorLoweringCode(bridge)
			decoder := core.NewDecompiler(bc.Code, nil)
			if decoder.ParseOpcode() != nil {
				t.Fatal("fresh forwarding")
			}
			ops := constructorMotionOps(decoder)
			call := ops[len(ops)-2]
			reference, ok := nativeEnumCPIndex(call)
			if !ok {
				t.Fatal("fresh bridge binding")
			}
			authority := a.source.jar.sourceConstructorLowering
			want := bytes.Clone(original)
			var work *workbudget.Budget
			switch kind {
			case "unreturned source":
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar}
			case "pending namespace":
				authority = &sourceConstructorLoweringAuthority{jar: authority.jar, parent: authority, bridgePending: map[string]constructorNativeSourceOrigin{owner: {}}}
			case "stale original":
				old := mustParseLowering(t, want)
				old.ConstantPoolManager.AddUtf8Info("another exact original namespace")
				want = old.Bytes()
			case "helper effects":
				hc.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, hc.Code...)
			case "virtual super call":
				hc.Code[len(hc.Code)-4] = core.OP_INVOKEVIRTUAL
			case "wrong bridge slot":
				bc.Code[int(ops[1].CurrentOffset)] = core.OP_LLOAD_2
			case "nonbridge flags":
				bridge.AccessFlags = 1
			case "other consumer":
				code := constructorLoweringCode(ctor)
				code.Code = append(bytes.Clone(code.Code[:len(code.Code)-1]), core.OP_ALOAD_0, core.OP_LCONST_0, core.OP_DCONST_0, core.OP_ACONST_NULL, core.OP_INVOKEVIRTUAL, byte(reference>>8), byte(reference), core.OP_POP, core.OP_RETURN)
				code.MaxStack = 6
			case "method handle":
				o.ConstantPool = append(o.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 5, ReferenceIndex: reference})
			case "changed throws":
				for _, attr := range helper.Attributes {
					if ex, ok := attr.(*ExceptionsAttribute); ok {
						ex.ExceptionIndexTable = []uint16{uint16(o.ConstantPoolManager.AddNewClassInfo("java/lang/RuntimeException"))}
					}
				}
			case "invalid parameter table", "duplicate parameter table":
				for _, attr := range bridge.Attributes {
					if raw, ok := attr.(*UnparsedAttribute); ok && raw.Name == "MethodParameters" {
						if kind == "invalid parameter table" {
							raw.Info[0]++
						} else {
							bridge.Attributes = append(bridge.Attributes, raw)
						}
						break
					}
				}
			case "unknown attribute":
				o.ConstantPoolManager.AddUtf8Info("UnprovedBridgeAttribute")
				bridge.Attributes = append(bridge.Attributes, &UnparsedAttribute{Name: "UnprovedBridgeAttribute"})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			case "output":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			for _, m := range o.Methods {
				code := constructorLoweringCode(m)
				if code == nil {
					continue
				}
				if len(code.Attributes) != 0 {
					t.Fatal("debug-free structural mutant")
				}
				code.AttrLen = uint32(12 + len(code.Code) + 8*len(code.ExceptionTable))
			}
			input := o.Bytes()
			if _, err := Parse(input); err != nil && kind != "duplicate parameter table" {
				t.Fatal("mutation did not reach proof", err)
			}
			saved := bytes.Clone(input)
			got, err := authority.lowerInheritedBridges(owner, want, input, work)
			if kind == "control" {
				if err != nil || bytes.Equal(got, input) {
					d := NewClassObjectDumper(mustParseLowering(t, want))
					d.foldSiblingResolver = authority.jar.enumSiblingResolver()
					d.declarationResolver = authority.jar.declarationResolver
					t.Log("receipt", authority.bridgeSourceReturned(owner, want, nil), "identity", sourceParameterClassIdentityClosed(d.obj, o), "flags", d.obj.AccessFlags, o.AccessFlags)
					for _, originalMethod := range d.obj.Methods {
						t.Log("original", d.inheritedBridgeOriginal(originalMethod))
					}
					t.Log("fresh", inheritedBridgeForwardPacket(o, helper, core.OP_INVOKESPECIAL, nil), "forward", inheritedBridgeForwardPacket(o, bridge, core.OP_INVOKEVIRTUAL, nil))
					t.Fatal("proved fresh code was not inlined", err)
				}
				if _, err := a.FinishClass(owner+".class", input); err != nil {
					t.Fatal("complete original bridge contract", err)
				}
			} else if err == nil && !bytes.Equal(got, input) || err != nil && got != nil {
				t.Fatal("unproved partial lowering", kind, err)
			}
			if (kind == "budget" || kind == "canceled" || kind == "output") && err == nil {
				t.Fatal("resource failure lost")
			}
			if kind == "duplicate parameter table" && err == nil {
				t.Fatal("duplicate metadata must fail during the strict class parse")
			}
			if !bytes.Equal(saved, input) {
				t.Fatal("input mutation")
			}
		})
	}
}
