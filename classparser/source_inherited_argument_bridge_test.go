package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"testing"
)

func TestAdversarialInheritedArgumentBridgeRequiresExactChecksAndSlots(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, inheritedArgumentBridgeFixture, "BridgeOuter", "none", false)
	for _, variant := range []string{"exact", "wrong first cast", "wrong array cast", "swapped casts", "missing cast", "extra cast", "wrong wide slot", "wrong reference slot", "virtual", "handler", "missing parent", "generic target", "work", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(files["BridgeOuter$Leaf.class"]))
			parent := mustParseLowering(t, bytes.Clone(files["BridgeOuter.class"]))
			var method *MemberInfo
			for _, m := range obj.Methods {
				if m.AccessFlags == 0x1041 {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original bridge missing")
			}
			code := constructorLoweringCode(method)
			d := core.NewDecompiler(code.Code, nil)
			if d.ParseOpcode() != nil {
				t.Fatal("original parse")
			}
			ops := constructorMotionOps(d)
			var casts []int
			for _, op := range ops {
				if op.Instr.OpCode == core.OP_CHECKCAST {
					casts = append(casts, int(op.CurrentOffset))
				}
			}
			if len(casts) != 2 {
				t.Fatal("missing original reference and array checks")
			}
			var work *workbudget.Budget
			switch variant {
			case "wrong first cast":
				binary.BigEndian.PutUint16(code.Code[casts[0]+1:], uint16(obj.ConstantPoolManager.AddNewClassInfo("java/lang/Object")))
			case "wrong array cast":
				binary.BigEndian.PutUint16(code.Code[casts[1]+1:], uint16(obj.ConstantPoolManager.AddNewClassInfo("java/lang/String")))
			case "swapped casts":
				a, b := binary.BigEndian.Uint16(code.Code[casts[0]+1:]), binary.BigEndian.Uint16(code.Code[casts[1]+1:])
				binary.BigEndian.PutUint16(code.Code[casts[0]+1:], b)
				binary.BigEndian.PutUint16(code.Code[casts[1]+1:], a)
			case "missing cast":
				code.Code = append(code.Code[:casts[0]], code.Code[casts[0]+3:]...)
			case "extra cast":
				at := casts[1]
				part := bytes.Clone(code.Code[at : at+3])
				code.Code = append(append(bytes.Clone(code.Code[:at]), part...), code.Code[at:]...)
			case "wrong wide slot":
				code.Code[int(ops[1].CurrentOffset)] = core.OP_LLOAD_2
			case "wrong reference slot":
				at := casts[0] - 2
				if code.Code[at] != core.OP_ALOAD {
					t.Fatal("expected general local load")
				}
				code.Code[at+1]++
			case "virtual":
				code.Code[int(ops[len(ops)-2].CurrentOffset)] = core.OP_INVOKEVIRTUAL
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 0}}
			case "generic target":
				for _, m := range parent.Methods {
					n, _ := sourceBridgeUTF8(parent, m.NameIndex)
					if n == "exchange" && m.AccessFlags == 1 {
						m.Attributes = append(m.Attributes, &SignatureAttribute{SignatureIndex: uint16(parent.ConstantPoolManager.AddUtf8Info("(JDLjava/lang/String;[Ljava/lang/String;)Ljava/lang/String;"))})
					}
				}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				base, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(base, workbudget.Limits{})
			}
			reader := NewClassObjectDumper(obj)
			reader.Work = work
			reader.foldSiblingResolver = func(n string) ([]byte, bool) {
				if variant == "missing parent" {
					return nil, false
				}
				if n == "BridgeOuter" {
					return parent.Bytes(), true
				}
				b, k := files[n+".class"]
				return b, k
			}
			packet := reader.inheritedBridgeOriginal(method)
			if (packet != nil) != (variant == "exact") {
				t.Fatalf("accepted=%v", packet != nil)
			}
			if packet != nil && (!packet.checkedArguments || len(packet.argumentCasts) != 4 || packet.argumentCasts[2] != "Ljava/lang/String;" || packet.argumentCasts[3] != "[Ljava/lang/String;") {
				t.Fatalf("lost exact checks: %+v", packet)
			}
		})
	}
}

func TestAdversarialInheritedArgumentBridgeFreshChecksCannotBeRemovedOrRetargeted(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	testIndependentInheritedBridgeWithOriginal(t, modern, java, native, inheritedArgumentBridgeFixture, "BridgeOuter", "source,lines,vars", true, "288:inherited:argument:casts:wide:dispatch:exception\n", nil,
		func(t *testing.T, owner string, original, fresh []byte, artifact *JarReconstructionArtifact) {
			for _, variant := range []string{"changed check", "removed check", "virtual helper", "wrong argument slot"} {
				t.Run(variant, func(t *testing.T) {
					obj := mustParseLowering(t, bytes.Clone(fresh))
					var method, helper *MemberInfo
					for _, m := range obj.Methods {
						n, _ := sourceBridgeUTF8(obj, m.NameIndex)
						if n != "exchange" {
							continue
						}
						if m.AccessFlags == 0x1041 {
							method = m
						} else if m.AccessFlags == 1 {
							helper = m
						}
					}
					if method == nil || helper == nil {
						t.Fatal("fresh pair missing")
					}
					code := constructorLoweringCode(method)
					d := core.NewDecompiler(code.Code, nil)
					if d.ParseOpcode() != nil {
						t.Fatal("fresh bridge parse")
					}
					ops := constructorMotionOps(d)
					cast := -1
					for _, op := range ops {
						if op.Instr.OpCode == core.OP_CHECKCAST {
							cast = int(op.CurrentOffset)
							break
						}
					}
					if cast < 0 {
						t.Fatal("fresh check absent")
					}
					switch variant {
					case "changed check":
						binary.BigEndian.PutUint16(code.Code[cast+1:], uint16(obj.ConstantPoolManager.AddNewClassInfo("java/lang/Object")))
					case "removed check":
						// Keep the Code attribute and all PCs structurally valid so
						// this tests the forwarding proof, not the binary parser.
						copy(code.Code[cast:cast+3], []byte{core.OP_NOP, core.OP_NOP, core.OP_NOP})
					case "wrong argument slot":
						code.Code[cast-1]++
					case "virtual helper":
						c := constructorLoweringCode(helper)
						p := inheritedBridgeForwardPacket(obj, helper, core.OP_INVOKESPECIAL, nil)
						if p == nil {
							t.Fatal("fresh helper")
						}
						c.Code[p.invokePC] = core.OP_INVOKEVIRTUAL
					}
					input := obj.Bytes()
					saved := bytes.Clone(input)
					out, err := artifact.source.jar.sourceConstructorLowering.lowerInheritedBridges(owner, original, input, nil)
					if err != nil || !bytes.Equal(saved, input) || !bytes.Equal(out, input) {
						t.Fatal("unproved checks admitted", err)
					}
				})
			}
		})
}

func TestAdversarialInheritedArgumentBridgeKeepsFreshCodeOutsideInvocation(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	testIndependentInheritedBridgeWithOriginal(t, modern, java, native, inheritedArgumentBridgeFixture, "BridgeOuter", "source,lines,vars", true, "288:inherited:argument:casts:wide:dispatch:exception\n", nil,
		func(t *testing.T, owner string, original, fresh []byte, artifact *JarReconstructionArtifact) {
			before := mustParseLowering(t, fresh)
			var bridge, helper *MemberInfo
			for _, m := range before.Methods {
				n, _ := sourceBridgeUTF8(before, m.NameIndex)
				if n == "exchange" {
					if m.AccessFlags == 0x1041 {
						bridge = m
					} else if m.AccessFlags == 1 {
						helper = m
					}
				}
			}
			if bridge == nil || helper == nil {
				t.Fatal("fresh pair")
			}
			forward := inheritedBridgeForwardPacket(before, bridge, core.OP_INVOKEVIRTUAL, nil)
			target := inheritedBridgeForwardPacket(before, helper, core.OP_INVOKESPECIAL, nil)
			if forward == nil || target == nil {
				t.Fatal("fresh program proof")
			}
			out, err := artifact.source.jar.sourceConstructorLowering.lowerInheritedBridges(owner, original, fresh, nil)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := NewClassReaderWithBudget(out, nil)
			if err != nil {
				t.Fatal(err)
			}
			layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
			after, err := parseWithMetadataLayout(reader, nil, layout)
			if err != nil {
				t.Fatal(err)
			}
			var lowered *MemberInfo
			for _, m := range after.Methods {
				n, _ := sourceBridgeUTF8(after, m.NameIndex)
				if n == "exchange" {
					if m.AccessFlags != 0x1041 || lowered != nil {
						t.Fatal("temporary helper retained")
					}
					lowered = m
				}
			}
			if lowered == nil {
				t.Fatal("bridge lost")
			}
			packet := inheritedBridgeForwardPacket(after, lowered, core.OP_INVOKESPECIAL, nil)
			if packet == nil || packet.targetOwner != "BridgeOuter" || !packet.checkedArguments {
				t.Fatal("lost checks or original special binding")
			}
			oldReader, _ := NewClassReaderWithBudget(fresh, nil)
			oldLayout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
			oldObj, e := parseWithMetadataLayout(oldReader, nil, oldLayout)
			if e != nil {
				t.Fatal(e)
			}
			var oldBridge *MemberInfo
			for _, m := range oldObj.Methods {
				if m.AccessFlags == 0x1041 {
					oldBridge = m
				}
			}
			oldCode, e := enumArrayCodePayload(fresh, oldObj, oldLayout.methods[oldBridge], nil)
			if e != nil {
				t.Fatal(e)
			}
			newCode, e := enumArrayCodePayload(out, after, layout.methods[lowered], nil)
			if e != nil {
				t.Fatal(e)
			}
			expected := bytes.Clone(oldCode.data)
			at := 8 + forward.invokePC
			expected[at] = core.OP_INVOKESPECIAL
			binary.BigEndian.PutUint16(expected[at+1:], target.invokeIndex)
			if !bytes.Equal(expected, newCode.data) {
				t.Fatal("changed fresh checks, slots, stack, PCs or Code attributes")
			}
		})
}
