package javaclassparser

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeEnumParameterStorageUsesOriginalTypedCellAndMethodIdentity(t *testing.T) {
	source := `public class StorageOwner{public static int run(StorageMode first,StorageMode other){first=other;return first.ordinal();}public static int wide(long padding,StorageMode first,StorageMode other){first=other;return first.ordinal();}public static int nullable(StorageMode first){first=null;return first.ordinal();}}enum StorageMode{A,B,C}class StorageDriver{public static void main(String[]a){System.out.println(StorageOwner.run(StorageMode.A,StorageMode.B));}}`
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"StorageOwner.java": source}, "none", "8")
	for _, variant := range []string{"original", "null writer", "wide preceding parameter", "primitive slot reuse", "two-word slot overlap", "foreign method", "foreign code", "duplicate method", "duplicate code", "missing code", "wrong PC", "wrong opcode", "wrong parameter slot", "wrong descriptor", "receiver", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			object, err := Parse(files["StorageOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			name, slot := "run", 0
			if variant == "null writer" {
				name = "nullable"
			}
			if variant == "wide preceding parameter" || variant == "two-word slot overlap" {
				name, slot = "wide", 2
			}
			var method *MemberInfo
			var code *CodeAttribute
			for _, candidate := range object.Methods {
				n, _ := sourceBridgeUTF8(object, candidate.NameIndex)
				if n == name {
					method = candidate
					for _, attribute := range candidate.Attributes {
						if c, ok := attribute.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if method == nil || code == nil {
				t.Fatal("original method")
			}
			if variant == "primitive slot reuse" || variant == "two-word slot overlap" {
				originalLength := len(code.Code)
				if code.Code[len(code.Code)-1] != core.OP_IRETURN {
					t.Fatal("expected return packet")
				}
				code.Code = append(code.Code[:len(code.Code)-1:len(code.Code)-1], core.OP_POP)
				if variant == "primitive slot reuse" {
					code.Code = append(code.Code, core.OP_ICONST_1, core.OP_ISTORE_0, core.OP_ILOAD_0, core.OP_IRETURN)
				} else {
					code.Code = append(code.Code, core.OP_DCONST_0, core.OP_DSTORE_1, core.OP_ICONST_1, core.OP_IRETURN)
					code.MaxStack = 2
				}
				code.AttrLen += uint32(len(code.Code) - originalLength)
				// Execute the authored valid slot-reuse class first. Refusal must
				// concern source storage closure, not a malformed verifier input.
				original := t.TempDir()
				for n, raw := range files {
					if n == "StorageOwner.class" {
						raw = object.Bytes()
					}
					if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
				_, java := t04Tools(t)
				if got := t04RunJava(t, java, original, "StorageDriver"); got != "1\n" {
					t.Fatal(got)
				}
			}
			decoder := core.NewDecompiler(code.Code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			var read *nativeEnumSelectorProducer
			for _, op := range constructorMotionOps(decoder) {
				if constructorMotionLoad(op, "LStorageMode;") && core.GetRetrieveIdx(op) == slot {
					read = &nativeEnumSelectorProducer{pc: int(op.CurrentOffset), opcode: op.Instr.OpCode, slot: slot, result: "LStorageMode;"}
				}
			}
			if read == nil {
				t.Fatal("original changed parameter read")
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign method":
				copy := *method
				method = &copy
			case "foreign code":
				copy := *code
				code = &copy
			case "duplicate method":
				object.Methods = append(object.Methods, method)
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "missing code":
				method.Attributes = nil
			case "wrong PC":
				read.pc++
			case "wrong opcode":
				read.opcode = core.OP_ILOAD_0
			case "wrong parameter slot":
				read.slot++
			case "wrong descriptor":
				read.result = "Ljava/lang/Object;"
			case "receiver":
				method.AccessFlags &^= 8
			case "work":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
				_ = work.CheckAlloc(2)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			d := NewClassObjectDumper(object)
			d.Work = work
			proof := d.nativeEnumParameterStorageProof(method, code, read)
			want := variant == "original" || variant == "null writer" || variant == "wide preceding parameter"
			if (proof != nil) != want {
				t.Fatalf("typed parameter cell=%v want %v", proof != nil, want)
			}
			if proof != nil && (proof.slot != slot || proof.descriptor != "LStorageMode;" || len(proof.stores) != 1 || len(proof.markers) != 1) {
				t.Fatal("wrong original storage evidence")
			}
		})
	}
}
