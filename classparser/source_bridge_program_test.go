package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func sourceBridgeTestCode(t *testing.T, object *ClassObject) (*MemberInfo, *CodeAttribute) {
	t.Helper()
	var bridge *MemberInfo
	var code *CodeAttribute
	for _, method := range object.Methods {
		if method.AccessFlags&0x0040 == 0 {
			continue
		}
		if bridge != nil {
			t.Fatal("one independent erased bridge required")
		}
		bridge = method
		for _, attribute := range method.Attributes {
			if value, ok := attribute.(*CodeAttribute); ok {
				code = value
			}
		}
	}
	if bridge == nil || code == nil || len(code.Code) != 9 || code.Code[0] != core.OP_ALOAD_0 || code.Code[1] != core.OP_ALOAD_1 || code.Code[2] != core.OP_CHECKCAST || code.Code[5] != core.OP_INVOKEVIRTUAL || code.Code[8] != core.OP_ARETURN {
		t.Fatal("actual compiler bridge protocol differs from this authored fixture")
	}
	return bridge, code
}

func TestNativeSourceArtifactBridgeProgramsKeepSymbolicCallsAndArgumentPorts(t *testing.T) {
	modern, _ := t04Tools(t)
	files := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, artifactBridgeMetadataFixture, "ArtifactLambdaOwner", "none", false)
	original := files["ArtifactLambdaOwner$1.class"]
	authority, err := Parse(bytes.Clone(original))
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"control", "equivalent CP alias", "equivalent explicit local load", "wrong receiver slot", "wrong argument slot", "wrong cast", "wrong delegate", "unknown opcode", "different stack", "different locals", "handler", "missing code", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(bytes.Clone(original))
			if err != nil {
				t.Fatal(err)
			}
			bridge, code := sourceBridgeTestCode(t, object)
			var work *workbudget.Budget
			switch variant {
			case "equivalent CP alias":
				index := binary.BigEndian.Uint16(code.Code[6:])
				ref := *object.ConstantPool[index-1].(*ConstantMethodrefInfo)
				object.ConstantPool = append(object.ConstantPool, &ref)
				binary.BigEndian.PutUint16(code.Code[6:], uint16(len(object.ConstantPool)))
			case "equivalent explicit local load":
				code.Code = append([]byte{core.OP_ALOAD, 0}, code.Code[1:]...)
				code.AttrLen++
			case "wrong receiver slot":
				code.Code[0] = core.OP_ALOAD_1
			case "wrong argument slot":
				code.Code[1] = core.OP_ALOAD_0
			case "wrong cast":
				binary.BigEndian.PutUint16(code.Code[3:], object.SuperClass)
			case "wrong delegate":
				index := object.ConstantPoolManager.AddNewMethodInfo(object.GetClassName(), "unrelated", "(Ljava/lang/String;)Ljava/lang/String;")
				binary.BigEndian.PutUint16(code.Code[6:], uint16(index))
			case "unknown opcode":
				code.Code = append([]byte{core.OP_NOP}, code.Code...)
				code.AttrLen++
			case "different stack":
				code.MaxStack++
			case "different locals":
				code.MaxLocals++
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 8}}
				code.AttrLen += 8
			case "missing code":
				var keep []AttributeInfo
				for _, attribute := range bridge.Attributes {
					if attribute != code {
						keep = append(keep, attribute)
					}
				}
				bridge.Attributes = keep
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := object.Bytes()
			if variant != "missing code" {
				if _, parseErr := Parse(input); parseErr != nil {
					t.Fatal("authored bridge variant must be parseable before the proof", parseErr)
				}
			}
			err = sourceArtifactBridgeProgramsClosed(authority, input, work)
			want := variant == "control" || variant == "equivalent CP alias" || variant == "equivalent explicit local load"
			if (err == nil) != want {
				t.Fatal("symbolic bridge-program equivalence", err)
			}
			if variant == "equivalent CP alias" && bytes.Equal(input, original) {
				t.Fatal("CP-alias control did not change the physical class")
			}
			if work != nil {
				var failure *workbudget.Error
				if !errors.As(err, &failure) {
					t.Fatal("bridge proof hid a resource/cancellation failure", err)
				}
			}
		})
	}
}

func TestAdversarialArtifactMetadataCannotHideEffectsInOriginalBridge(t *testing.T) {
	modern, java := t04Tools(t)
	fixture := strings.Replace(artifactBridgeMetadataFixture, "public class ArtifactLambdaOwner {", "public class ArtifactLambdaOwner {public static int bridgeCalls;public static void bridgeTick(){bridgeCalls++;}", 1)
	fixture += `class BridgeEffectDriver {public static void main(String[]args){ArtifactLambdaOwner owner=new ArtifactLambdaOwner("token");java.util.function.Function erased=owner.select;if(erased.apply("")!=owner.token||owner.calls!=1||ArtifactLambdaOwner.bridgeCalls!=1)throw new AssertionError("bridge effect before body");try{erased.apply(Integer.valueOf(17));throw new AssertionError("missing cast");}catch(ClassCastException expected){}if(owner.calls!=1||ArtifactLambdaOwner.bridgeCalls!=2)throw new AssertionError("bridge effect before failing cast");System.out.println("bridge:effects:receiver:castphase");}}`
	files := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, fixture, "ArtifactLambdaOwner", "none", false)
	child := "ArtifactLambdaOwner$1.class"
	object, err := Parse(bytes.Clone(files[child]))
	if err != nil {
		t.Fatal(err)
	}
	_, code := sourceBridgeTestCode(t, object)
	index := object.ConstantPoolManager.AddNewMethodInfo("ArtifactLambdaOwner", "bridgeTick", "()V")
	code.Code = append([]byte{core.OP_INVOKESTATIC, byte(index >> 8), byte(index)}, code.Code...)
	code.AttrLen += 3
	original := object.Bytes()
	modified := map[string][]byte{}
	for name, data := range files {
		modified[name] = bytes.Clone(data)
	}
	modified[child] = original
	if got := t04RunJava(t, java, sourceArtifactWriteFiles(t, modified), "BridgeEffectDriver"); got != "bridge:effects:receiver:castphase\n" {
		t.Fatal("authored original bridge effect oracle", got)
	}
	// The byte-identical caller exposes the missing effect in the actual fresh
	// compiler bridge. Table equality alone is deliberately insufficient.
	plain := sourceArtifactWriteFiles(t, files)
	log, err := exec.Command(java, "-Xverify:all", "-cp", plain, "BridgeEffectDriver").CombinedOutput()
	if err == nil || !strings.Contains(string(log), "bridge effect before body") {
		t.Fatal("wrong-body negative control did not lose the effect", err, string(log))
	}
	if output, err := RestoreMethodParameterMetadata(original, files[child], nil); err != nil || lambdaParameterMetadataTables(t, output) != lambdaParameterMetadataTables(t, original) {
		t.Fatal("standalone metadata operation must not claim algorithm validation", err)
	}
	jar := filepath.Join(t.TempDir(), "effect.jar")
	if err := os.WriteFile(jar, t23Zip(t, modified), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarSourceArtifactFromLocal(jar, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	before := bytes.Clone(files[child])
	if output, err := artifact.RestoreClassMetadata(child, files[child]); err == nil || output != nil {
		t.Fatal("parameter restoration hid a changed bridge algorithm")
	}
	if !bytes.Equal(before, files[child]) {
		t.Fatal("rejected compiler output mutated")
	}
}
