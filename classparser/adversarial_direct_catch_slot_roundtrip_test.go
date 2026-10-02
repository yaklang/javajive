package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestAdversarialSavedRetryDirectCatchSlotRoundTrip(t *testing.T) {
	t.Parallel()
	javac, java := t04Tools(t)
	for _, debug := range []string{"-g", "-g:none"} {
		original := t.TempDir()
		file := filepath.Join(original, "SavedRetryExceptionReview.java")
		if err := os.WriteFile(file, []byte(reviewedSavedRetryFixtureSource), 0644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, file).CombinedOutput(); err != nil {
			t.Fatalf("original compile: %v\n%s", err, out)
		}
		want := t04RunJava(t, java, original, "SavedRetryExceptionReview")
		classFile := filepath.Join(original, "SavedRetryExceptionReview.class")
		raw, err := os.ReadFile(classFile)
		if err != nil {
			t.Fatal(err)
		}
		object, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		changed := 0
		for _, method := range object.Methods {
			name, _ := object.getUtf8(method.NameIndex)
			if name != "decode" {
				continue
			}
			for _, attribute := range method.Attributes {
				code, ok := attribute.(*CodeAttribute)
				if !ok {
					continue
				}
				parser := core.NewDecompiler(code.Code, nil)
				if err := parser.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				ops := parser.Opcodes()
				if len(code.ExceptionTable) != 4 {
					t.Fatal("fixture no longer has two initial catches and one multicatch retry")
				}
				// javac saves through a catch temporary. The equally valid ECJ
				// shape stores directly to the outer local at handler entry.
				// Change only ASTORE/ALOAD/ASTORE, retaining every PC, branch,
				// exception range and frame. Verify and execute the result below.
				for _, entry := range code.ExceptionTable[:2] {
					for i, op := range ops {
						if op.CurrentOffset != entry.HandlerPc {
							continue
						}
						if i+2 >= len(ops) {
							t.Fatal("truncated catch fixture")
						}
						load, store := ops[i+1], ops[i+2]
						catchSlot, savedSlot := core.GetStoreIdx(op), core.GetStoreIdx(store)
						if catchSlot < 0 || savedSlot < 0 || core.GetRetrieveIdx(load) != catchSlot || op.Instr.OpCode != core.OP_ASTORE {
							t.Fatalf("fixture catch no longer has checked store/load/store shape: PC=%d", op.CurrentOffset)
						}
						code.Code[int(op.CurrentOffset)+1] = byte(savedSlot)
						for _, dead := range []*core.OpCode{load, store} {
							for j := int(dead.CurrentOffset); j < int(dead.CurrentOffset)+1+len(dead.Data); j++ {
								code.Code[j] = core.OP_NOP
							}
						}
						changed++
					}
				}
			}
		}
		if changed != 2 {
			t.Fatalf("expected two direct saved catch stores; got %d", changed)
		}
		raw = object.Bytes()
		if err := os.WriteFile(classFile, raw, 0644); err != nil {
			t.Fatal(err)
		}
		if got := t04RunJava(t, java, original, "SavedRetryExceptionReview"); got != want {
			t.Fatalf("verified direct-slot mutation changed original oracle: got %q want %q", got, want)
		}
		resolve := func(name string) ([]byte, bool) {
			b, err := os.ReadFile(filepath.Join(original, name+".class"))
			return b, err == nil
		}
		for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
			var source string
			if mode == "legacy" {
				source, err = DecompileWithResolver(raw, resolve)
			} else {
				var result DecompileResult
				result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, Resolve: resolve, TargetSourceVersion: 8})
				source = result.Source
			}
			if err != nil {
				t.Fatal(err)
			}
			rebuilt := t.TempDir()
			file := filepath.Join(rebuilt, "SavedRetryExceptionReview.java")
			if err := os.WriteFile(file, []byte(source), 0644); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", rebuilt, file).CombinedOutput(); err != nil {
				t.Fatalf("%s/%s rebuild: %v\n%s\n%s", mode, debug, err, out, source)
			}
			if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+original, "SavedRetryExceptionReview"); got != want {
				t.Fatalf("%s/%s: got %q want %q\n%s", mode, debug, got, want, source)
			}
		}
	}
}
