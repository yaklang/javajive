package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func TestAdversarialConstructorStaticArgumentRequiresOriginalSourceWitness(t *testing.T) {
	for _, kind := range []string{"control", "no witness", "changed owner", "changed name", "changed descriptor", "changed pc", "missing pc", "method handle", "restamp", "parameter impostor"} {
		t.Run(kind, func(t *testing.T) {
			member := values.NewJavaClassMember("packet/Settings", "mode", "I", types.NewJavaPrimer(types.JavaInteger))
			field := *member
			field.OriginPC, field.HasOriginPC = 9, true
			if kind != "no witness" {
				field.MarkOriginalFieldRead(member, 9)
			}
			arg := constructorLoweringArgument{owner: "packet/Settings", name: "mode", descriptor: "I"}
			switch kind {
			case "changed owner":
				field.Name = "decoy/Settings"
			case "changed name":
				field.Member = "other"
			case "changed descriptor":
				field.Description = "J"
			case "changed pc":
				field.OriginPC++
			case "missing pc":
				field.HasOriginPC = false
			case "method handle":
				field.RefKind = 2
			case "restamp":
				member.Name = "decoy/Settings"
				field.MarkOriginalFieldRead(member, 9)
				field.Name, arg.owner = member.Name, member.Name
			case "parameter impostor":
				arg.slot = 1
			}
			if constructorLoweringSourceArgument(arg, 9, &field, nil, nil, nil) != (kind == "control") {
				t.Fatal("static operand source identity was not proved", kind)
			}
		})
	}
}

func TestAdversarialConstructorWideningRequiresCompleteBoundedHierarchy(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original compiler required")
	}
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, constructorWideningFixture("static field"), "SuffixOwner", "none", false)
	obj := mustParseLowering(t, files["SuffixOwner$Member.class"])
	var ctor *MemberInfo
	for _, m := range obj.Methods {
		if name, _ := sourceBridgeUTF8(obj, m.NameIndex); name == "<init>" {
			ctor = m
		}
	}
	for _, kind := range []string{"control", "unknown", "wrong declaration", "incomplete", "unrelated", "cycle", "excessive edges"} {
		t.Run(kind, func(t *testing.T) {
			d := NewClassObjectDumper(obj)
			d.declarationResolver = func(n string) ([]byte, bool) { raw, found := files[n+".class"]; return raw, found }
			original := d.buildInvocationMetadata()
			calls := 0
			provider := func(n string) (callbinding.Class, bool) {
				calls++
				c, found := original(n)
				if n == "SuffixToken" {
					switch kind {
					case "unknown":
						found = false
					case "wrong declaration":
						c.Name = "DifferentToken"
					case "incomplete":
						c.ParentsComplete = false
					case "unrelated":
						c.Parents = []string{"java/lang/Object"}
					case "cycle":
						c.Parents = []string{n}
					case "excessive edges":
						c.Parents = make([]string, 65)
						for i := range c.Parents {
							c.Parents[i] = "java/lang/Runnable"
						}
					}
				}
				return c, found
			}
			packet := constructorLoweringTypedPacketFor(obj, ctor, true, true, nil, provider)
			if (packet != nil) != (kind == "control") || calls == 0 || calls > 64 {
				t.Fatal("reference conversion admitted without bounded original hierarchy", kind, calls)
			}
		})
	}
}

func TestAdversarialConstructorStaticArgumentRejectsFreshRebinding(t *testing.T) {
	original, fresh, artifact := constructorSuffixRoundTrip(t, constructorStaticArgumentFixture(), "none", "SuffixOwner$Member", "72:constructor:closed:suffix:oracle\n")
	for _, kind := range []string{"control", "field owner", "field name", "field descriptor", "method call", "substituted constant"} {
		t.Run(kind, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			var packet *constructorLoweringPacket
			for _, method := range obj.Methods {
				if p := constructorLoweringPacketFor(obj, method, false, false, nil, 1); p != nil {
					if packet != nil {
						t.Fatal("ambiguous fixture constructor")
					}
					packet = p
				}
			}
			if packet == nil {
				t.Fatal("fresh compiler did not produce the expected typed packet")
			}
			var op *core.OpCode
			for _, candidate := range packet.ops {
				if member := constructorMotionMember(obj, candidate, core.OP_GETSTATIC); member != nil && member.Description == "I" {
					op = candidate
					break
				}
			}
			if op == nil {
				t.Fatal("fixture static operand absent")
			}
			pc := int(op.CurrentOffset)
			member := constructorMotionMember(obj, op, core.OP_GETSTATIC)
			switch kind {
			case "field owner", "field name", "field descriptor":
				owner, name, descriptor := member.Name, member.Member, member.Description
				if kind == "field owner" {
					owner = "DifferentSettings"
				} else if kind == "field name" {
					name = "differentValue"
				} else {
					descriptor = "J"
				}
				ref := obj.ConstantPoolManager.AppendConstantInfo(&ConstantFieldrefInfo{ConstantMemberrefInfo: *obj.ConstantPoolManager.newMemberrefInfo(owner, name, descriptor)})
				binary.BigEndian.PutUint16(packet.code.Code[pc+1:pc+3], uint16(ref))
			case "method call":
				ref := obj.ConstantPoolManager.AddNewMethodInfo(member.Name, member.Member, "()I")
				packet.code.Code[pc] = core.OP_INVOKESTATIC
				binary.BigEndian.PutUint16(packet.code.Code[pc+1:pc+3], uint16(ref))
			case "substituted constant":
				copy(packet.code.Code[pc:pc+3], []byte{core.OP_ICONST_0, core.OP_NOP, core.OP_NOP})
			}
			input := obj.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("mutation is structurally malformed", err)
			}
			saved := bytes.Clone(input)
			result, err := artifact.source.jar.sourceConstructorLowering.lower(original, input, nil)
			if kind == "control" {
				if err != nil || bytes.Equal(input, result) {
					t.Fatal("positive original/fresh binding control", err)
				}
			} else if err == nil || result != nil {
				t.Fatal("unproved fresh static operand accepted", kind, err)
			}
			if !bytes.Equal(saved, input) {
				t.Fatal("caller input mutated")
			}
		})
	}
}
