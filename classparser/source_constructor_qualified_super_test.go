package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialPartialMemberSuperCompletionRequiresExactQualifierAndDeclarations(t *testing.T) {
	original, fresh, artifact := constructorSuffixFamilyAtTargetRoundTrip(t, partialMemberSuperFixture, "source,lines,vars", []string{"SuffixOwner$Member"}, "24:partial:member:super:nullable:identity:oracle\n", 8, 16)
	for _, variant := range []string{"control", "call owner", "call name", "call descriptor", "interface methodref", "invocation kind", "duplicate kind", "discard kind", "qualifier slot", "missing publication", "missing qualified proof", "changed parent declaration", "changed enclosing declaration", "work", "memory"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			z, authority := artifact.source.jar, artifact.source.jar.sourceConstructorLowering
			reader := z.nativeMemberReader(mustParseLowering(t, original))
			var packet *constructorLoweringPacket
			for _, method := range obj.Methods {
				if p := constructorLoweringTypedPacketWithQualifier(obj, method, false, false, true, nil, reader.buildInvocationMetadata(), 1); p != nil {
					packet = p
				}
			}
			if packet == nil || len(packet.qualifierGuards) != 1 {
				t.Fatal("fresh original compiler qualifier control missing")
			}
			guard := packet.qualifierGuards[0]
			pc := guard.start + 1
			code := packet.code.Code
			index := int(binary.BigEndian.Uint16(code[pc+1 : pc+3]))
			methodref := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
			member := constructorMotionMember(obj, &core.OpCode{Instr: &core.Instruction{OpCode: int(code[pc])}, Data: code[pc+1 : pc+3]}, int(code[pc]))
			if member == nil {
				t.Fatal("fresh qualifier method")
			}
			rebind := func(owner, name, desc string) {
				ref := obj.ConstantPoolManager.AddNewMethodInfo(owner, name, desc)
				binary.BigEndian.PutUint16(code[pc+1:], uint16(ref))
			}
			active := authority.active
			resolve := z.declarationResolver
			defer func() { authority.active = active; z.declarationResolver = resolve }()
			var work *workbudget.Budget
			switch variant {
			case "call owner":
				rebind("DifferentChecks", member.Member, member.Description)
			case "call name":
				rebind(member.Name, "differentCheck", member.Description)
			case "call descriptor":
				rebind(member.Name, member.Member, "(Ljava/lang/Object;)Ljava/lang/String;")
			case "interface methodref":
				obj.ConstantPool[index-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: methodref.ConstantMemberrefInfo}
			case "invocation kind":
				code[pc] = core.OP_INVOKESPECIAL
			case "duplicate kind":
				code[guard.start] = core.OP_DUP_X1
			case "discard kind":
				code[guard.end-1] = core.OP_POP2
			case "qualifier slot":
				code[guard.start-1] = core.OP_ALOAD_0
			case "missing publication":
				authority.active = map[string]*sourceConstructorLoweringPublication{}
			case "missing qualified proof":
				publication := *active["SuffixOwner$Member"]
				publication.plans = map[string]*sourceConstructorLoweringPlan{}
				for desc, plan := range active["SuffixOwner$Member"].plans {
					copy := *plan
					copy.qualified = nil
					publication.plans[desc] = &copy
				}
				authority.active = map[string]*sourceConstructorLoweringPublication{"SuffixOwner$Member": &publication}
			case "changed parent declaration", "changed enclosing declaration":
				target := "SuffixAncestor$Base"
				if variant == "changed enclosing declaration" {
					target = "SuffixAncestor"
				}
				z.declarationResolver = func(name string) ([]byte, bool) {
					raw, ok := resolve(name)
					if !ok || name != target {
						return raw, ok
					}
					other := mustParseLowering(t, bytes.Clone(raw))
					other.ConstantPoolManager.AddUtf8Info("changed independent declaration")
					return other.Bytes(), true
				}
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1, MaxGraphEdges: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			}
			input := obj.Bytes()
			saved := bytes.Clone(input)
			if _, err := Parse(input); err != nil {
				t.Fatal("mutation did not reach semantic completion", err)
			}
			_, err := authority.lower(original, input, work)
			if (err == nil) != (variant == "control") {
				t.Fatalf("qualifier proof %s: %v", variant, err)
			}
			if !bytes.Equal(saved, input) {
				t.Fatal("completion mutated caller bytes")
			}
		})
	}
}

// A source-level qualified SUPER has its own observable null check. The
// reconstruction proof for an implicit enclosing port must not accept it.
func TestAdversarialPartialMemberSuperKeepsOriginalExplicitNullCheck(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	_, java := t04Tools(t)
	fixture := strings.Replace(partialMemberSuperFixture, "super(word)", "SuffixOwner.this.super(word)", 1)
	fixture = strings.Replace(fixture, "long expected=", `if(outer==null){if(!(error instanceof NullPointerException)||SuffixWitness.escaped!=null||!SuffixWitness.trace.equals(""))throw new AssertionError("original qualifier check",error);rows++;continue;}long expected=`, 1)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			original := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, fixture, "SuffixOwner", debug, nil, 8, false)
			dir := sourceArtifactWriteFiles(t, original)
			for _, mode := range []string{"-ea", "-da"} {
				output, err := enumFreshOracleRun(t, java, mode, dir, "SuffixDriver")
				if err != nil || string(output) != "24:partial:member:super:nullable:identity:oracle\n" {
					t.Fatal("original explicit null check oracle", err, string(output))
				}
			}
			object := mustParseLowering(t, original["SuffixOwner$Member.class"])
			reader := NewClassObjectDumper(object)
			reader.declarationResolver = func(name string) ([]byte, bool) { raw, ok := original[name+".class"]; return raw, ok }
			provider := reader.buildInvocationMetadata()
			found := false
			for _, method := range object.Methods {
				name, _ := sourceBridgeUTF8(object, method.NameIndex)
				if name != "<init>" {
					continue
				}
				found = true
				if constructorLoweringTypedPacketFor(object, method, true, true, nil, provider) != nil {
					t.Fatal("explicit original qualifier admitted as implicit nullable SUPER")
				}
				if constructorLoweringTypedPacketWithQualifier(object, method, true, true, true, nil, provider) != nil {
					t.Fatal("fresh-only guard proof accepted original qualifier")
				}
			}
			if !found {
				t.Fatal("missing original constructor")
			}
			implicit := compileLambdaParameterMetadataFixtureWithSourcesAtRelease(t, native, NativeJavac8, partialMemberSuperFixture, "SuffixOwner", debug, nil, 8, false)
			control := mustParseLowering(t, implicit["SuffixOwner$Member.class"])
			accepted := false
			for _, method := range control.Methods {
				accepted = accepted || constructorLoweringTypedPacketFor(control, method, true, true, nil, provider) != nil
			}
			if !accepted {
				t.Fatal("implicit nullable original positive control was not admitted")
			}
		})
	}
}
