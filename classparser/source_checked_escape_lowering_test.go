package javaclassparser

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

// Build the original independently of the decompiler, then remove only the
// Java checked-exception declaration (which is not a JVM verifier obligation).
func checkedEscapeCompletionFixture(t *testing.T) ([]byte, []byte, []byte, *JarReconstructionArtifact) {
	t.Helper()
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	const fixture = `public class EscapeProof {public static int run(int n) throws Exception {if(n==0)throw new Exception("same");return n*17;} private static int jdec$rethrow$0(int n){return n+1;}}`
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "EscapeProof", "none", false)
	original := mustParseLowering(t, files["EscapeProof.class"])
	for _, m := range original.Methods {
		if name, _ := sourceBridgeUTF8(original, m.NameIndex); name == "run" {
			attrs := []AttributeInfo{}
			for _, a := range m.Attributes {
				if _, ok := a.(*ExceptionsAttribute); !ok {
					attrs = append(attrs, a)
				}
			}
			m.Attributes = attrs
		}
	}
	raw := original.Bytes()
	path := filepath.Join(t.TempDir(), "fixture.jar")
	if err := os.WriteFile(path, t23Zip(t, map[string][]byte{"EscapeProof.class": raw}), 0600); err != nil {
		t.Fatal(err)
	}
	artifact, err := NewJarReconstructionArtifactFromLocal(path, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { artifact.Close() })
	// Reading through the older source API cannot grant executable authority.
	source, err := artifact.source.ReadFile("EscapeProof.class")
	if err != nil {
		t.Fatal(err)
	}
	fresh := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, string(source), "EscapeProof", "none", false)["EscapeProof.class"]
	if result, err := artifact.FinishClass("EscapeProof.class", fresh); err == nil || result != nil {
		t.Fatal("unreturned executable source accepted")
	}
	if _, err = artifact.ReadFile("EscapeProof.class"); err != nil {
		t.Fatal(err)
	}
	return raw, fresh, source, artifact
}

func TestAdversarialCheckedEscapeCompletionRequiresCaughtIdentityAndClosedUses(t *testing.T) {
	original, fresh, source, artifact := checkedEscapeCompletionFixture(t)
	base := artifact.source.jar.sourceConstructorLowering
	origin := base.escapeActive["EscapeProof"]
	if origin.helper == "" || origin.helper == "jdec$rethrow$0" {
		t.Fatal("concrete writer must avoid the independent original declaration")
	}
	variants := []string{"control", "unreturned", "pending", "stale original", "changed source", "conflicting source", "public helper", "synchronized helper", "helper returns", "helper handler", "helper annotation", "extra method", "helper handle", "null operand", "changed local", "return continuation", "different catch coverage", "continuation frame", "continuation branch", "normal handler entry", "work", "output", "canceled", "finished twice"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			want := bytes.Clone(original)
			obj := mustParseLowering(t, bytes.Clone(fresh))
			authority := &sourceConstructorLoweringAuthority{jar: base.jar, escapeActive: map[string]checkedEscapeSourceOrigin{"EscapeProof": origin}}
			var helper, caller *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == origin.helper {
					helper = m
				}
				if n == "run" {
					caller = m
				}
			}
			if helper == nil || caller == nil {
				t.Fatal("fresh method identity")
			}
			body := constructorLoweringCode(helper)
			code := constructorLoweringCode(caller)
			ops, ok := checkedEscapeMethodOps(code, nil)
			if !ok {
				t.Fatal("fresh instructions")
			}
			call := -1
			index := uint16(0)
			var load, entry int
			for i, op := range ops {
				if nativeEnumMemberOperand(obj, op, core.OP_INVOKESTATIC, "EscapeProof", origin.helper, "(Ljava/lang/Throwable;)Ljava/lang/RuntimeException;") {
					call = int(op.CurrentOffset)
					index, _ = nativeEnumCPIndex(op)
					load = int(ops[i-1].CurrentOffset)
					entry = int(ops[i-2].CurrentOffset)
				}
			}
			if call < 0 {
				t.Fatal("actual adapter call")
			}
			var work *workbudget.Budget
			switch variant {
			case "unreturned":
				authority.escapeActive = nil
			case "pending":
				authority = &sourceConstructorLoweringAuthority{jar: base.jar, parent: base, escapePending: map[string]checkedEscapeSourceOrigin{"EscapeProof": origin}}
			case "stale original":
				old := mustParseLowering(t, want)
				old.ConstantPoolManager.AddUtf8Info("independent original")
				want = old.Bytes()
			case "changed source":
				bad := origin
				bad.sourceHash = sha256.Sum256([]byte("different"))
				authority.escapePending = map[string]checkedEscapeSourceOrigin{"EscapeProof": bad}
				if err := authority.sourceReturned("EscapeProof", source); err == nil {
					t.Fatal("changed source accepted")
				}
				return
			case "conflicting source":
				authority.escapeActive = nil
				authority.escapePending = map[string]checkedEscapeSourceOrigin{"EscapeProof": origin}
				authority.active = map[string]*sourceConstructorLoweringPublication{"EscapeProof": {}}
				if err := authority.sourceReturned("EscapeProof", source); err == nil || len(authority.escapeActive) != 0 {
					t.Fatal("failed transaction leaked executable authority")
				}
				return
			case "public helper":
				helper.AccessFlags = 9
			case "synchronized helper":
				helper.AccessFlags |= 0x20
			case "helper returns":
				body.Code[1] = core.OP_ARETURN
			case "helper handler":
				body.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 1}}
				body.AttrLen += 8
			case "helper annotation":
				helper.Attributes = append(helper.Attributes, &UnparsedAttribute{Name: "RuntimeVisibleAnnotations", Info: []byte{0, 0}, Length: 2})
			case "extra method":
				copy := *helper
				copy.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("unrelatedExtra"))
				obj.Methods = append(obj.Methods, &copy)
			case "helper handle":
				obj.ConstantPoolManager.AppendConstantInfo(&ConstantMethodHandleInfo{ReferenceKind: 6, ReferenceIndex: index})
			case "null operand":
				code.Code[load] = core.OP_ACONST_NULL
			case "changed local":
				code.Code[load] = core.OP_ALOAD_0
			case "return continuation":
				code.Code[call+3] = core.OP_ARETURN
			case "different catch coverage":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: uint16(call), EndPc: uint16(call + 3), HandlerPc: uint16(entry)})
				code.AttrLen += 8
			case "continuation frame":
				// A well-formed extra frame at the old nominal return continuation must
				// not be retained after replacing RuntimeException with Throwable.
				for _, a := range code.Attributes {
					if u, ok := a.(*UnparsedAttribute); ok && u.Name == "StackMapTable" {
						u.Info = []byte{0, 1, byte(call + 3)}
						u.Length = 3
					}
				}
			case "continuation branch", "normal handler entry":
				target := call + 3
				if variant == "normal handler entry" {
					target = entry
				}
				changed := false
				for _, op := range ops {
					if constructorMotionPacketBranch(op.Instr.OpCode) && len(op.Data) == 2 {
						delta := target - int(op.CurrentOffset)
						code.Code[int(op.CurrentOffset)+1] = byte(delta >> 8)
						code.Code[int(op.CurrentOffset)+2] = byte(delta)
						changed = true
						break
					}
				}
				if !changed {
					t.Fatal("independent original must have conditional edge")
				}
			case "work":
				work = workbudget.New(context.Background(), workbudget.Limits{MaxRequestWork: 1})
			case "output":
				work = workbudget.New(context.Background(), workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			input := obj.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("negative must reach lowering proof", err)
			}
			before := bytes.Clone(input)
			result, err := authority.lowerCheckedEscapeHelper("EscapeProof", want, input, work)
			if !bytes.Equal(input, before) {
				t.Fatal("caller bytes mutated")
			}
			if variant == "control" || variant == "finished twice" {
				if err != nil || bytes.Equal(result, input) {
					t.Fatal("proved adapter not discharged", err)
				}
				output := mustParseLowering(t, result)
				if len(output.Methods) != len(mustParseLowering(t, original).Methods) {
					t.Fatal("original method universe")
				}
				if variant == "finished twice" {
					again, e := authority.lowerCheckedEscapeHelper("EscapeProof", want, result, nil)
					if e != nil || !bytes.Equal(again, result) {
						t.Fatal("not idempotent", e)
					}
				}
				return
			}
			if err == nil && !bytes.Equal(result, input) {
				t.Fatal("unproved executable edit accepted")
			}
			if err != nil && result != nil {
				t.Fatal("partial output on refusal")
			}
		})
	}
}
