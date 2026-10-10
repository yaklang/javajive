package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorWideningHierarchyEvidenceAndBudget(t *testing.T) {
	for _, variant := range []string{"subtype", "missing", "wrong identity", "incomplete", "cycle", "too many edges", "too deep"} {
		t.Run(variant, func(t *testing.T) {
			calls := 0
			provider := func(name string) (callbinding.Class, bool) {
				calls++
				class := callbinding.Class{Name: name, ParentsComplete: true}
				if name == "Actual" {
					class.Parents = []string{"Formal"}
				}
				switch variant {
				case "missing":
					return callbinding.Class{}, false
				case "wrong identity":
					class.Name = "Unrelated"
				case "incomplete":
					class.ParentsComplete = false
				case "cycle":
					if name == "Actual" {
						class.Parents = []string{"Other"}
					} else {
						class.Parents = []string{"Actual"}
					}
				case "too many edges":
					for i := 0; i < 65; i++ {
						class.Parents = append(class.Parents, fmt.Sprintf("Other%d", i))
					}
				case "too deep":
					if name == "Actual" {
						class.Parents = []string{"Step0"}
					} else if strings.HasPrefix(name, "Step") {
						var index int
						fmt.Sscanf(name, "Step%d", &index)
						if index < 70 {
							class.Parents = []string{fmt.Sprintf("Step%d", index+1)}
						} else {
							class.Parents = []string{"Formal"}
						}
					}
				}
				return class, true
			}
			q := newConstructorWideningQuery(provider)
			want := variant == "subtype"
			if got := q.assignable("LActual;", "LFormal;"); got != want {
				t.Fatalf("assignable=%v want=%v", got, want)
			}
			if calls > 64 {
				t.Fatalf("hierarchy exceeded retained budget: %d", calls)
			}
			if want {
				for i := 0; i < 255; i++ {
					if !q.assignable("LActual;", "LFormal;") {
						t.Fatal("repeated operand lost original proof")
					}
				}
				if calls != 1 {
					t.Fatalf("shared query repeated provider: %d", calls)
				}
			}
		})
	}
	for _, tc := range []struct {
		actual, formal string
		want           bool
	}{
		{"Ljava/lang/String;", "Ljava/lang/Object;", true}, {"null", "[Ljava/lang/Object;", true},
		{"[[I", "[Ljava/lang/Object;", true}, {"[I", "[J", false}, {"[I", "[Ljava/lang/Object;", false},
		{"[Ljava/lang/Object;", "[Ljava/lang/String;", false}, {"I", "J", false}, {"I", "Z", false}, {"Z", "I", false},
	} {
		t.Run(tc.actual+"/"+tc.formal, func(t *testing.T) {
			if got := newConstructorWideningQuery(nil).assignable(tc.actual, tc.formal); got != tc.want {
				t.Fatalf("assignable=%v want=%v", got, tc.want)
			}
		})
	}
	// Freeze the observed parent edge. An unrelated later provider lookup must
	// not mutate the evidence already bound to a different constructor operand.
	parents := []string{"Formal"}
	q := newConstructorWideningQuery(func(name string) (callbinding.Class, bool) {
		if name == "Actual" {
			return callbinding.Class{Name: name, ParentsComplete: true, Parents: parents}, true
		}
		parents[0] = "Unrelated"
		return callbinding.Class{Name: name, ParentsComplete: true}, true
	})
	if !q.assignable("LActual;", "LFormal;") {
		t.Fatal("missing original edge")
	}
	_ = q.assignable("LOther;", "LAbsent;")
	if !q.assignable("LActual;", "LFormal;") {
		t.Fatal("provider changed an earlier bound operand")
	}
}

func TestAdversarialConstructorWideningOriginalCastOperandEvidence(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "CastBoundaryOwner.java")
	if err := os.WriteFile(file, []byte(`class CastBoundaryParent {CastBoundaryParent(Object[] x){}}class CastBoundaryOwner {class Member extends CastBoundaryParent {Member(String[] x){super((Object[])x);}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	raw := readClassBytes(t, dir, "CastBoundaryOwner$Member")
	for _, variant := range []string{"original", "cast consumes THIS", "cast without operand", "wrong cast constant kind", "nil cast constant", "wrong invocation constant kind", "truncated cast", "primitive operand", "work budget"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			var code *CodeAttribute
			var descriptor string
			for _, m := range obj.Methods {
				name, _ := obj.getUtf8(m.NameIndex)
				if name == "<init>" {
					descriptor, _ = obj.getUtf8(m.DescriptorIndex)
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("missing original constructor")
			}
			params, _, err := callbinding.Descriptor(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			if len(ops) < 7 || ops[2].Instr.OpCode != core.OP_PUTFIELD || ops[5].Instr.OpCode != core.OP_CHECKCAST {
				t.Fatal("missing original cast and early capture")
			}
			cast := ops[5]
			cp := core.Convert2bytesToInt(cast.Data)
			switch variant {
			case "cast consumes THIS":
				load := *ops[3].Instr
				ops[4].Instr = &load
			case "primitive operand":
				load := *ops[3].Instr
				load.OpCode = core.OP_ICONST_0
				ops[4].Instr = &load
			case "cast without operand":
				ops = append(ops[:4], ops[5:]...)
			case "wrong cast constant kind":
				obj.ConstantPool[cp-1] = &ConstantUtf8Info{Value: "[Ljava/lang/Object;"}
			case "nil cast constant":
				obj.ConstantPool[cp-1] = (*ConstantClassInfo)(nil)
			case "wrong invocation constant kind":
				index := core.Convert2bytesToInt(ops[6].Data)
				obj.ConstantPool[index-1] = &ConstantUtf8Info{Value: "<init>"}
			case "truncated cast":
				cast.Data = []byte{0}
			case "work budget":
				copies := make([]*core.OpCode, 513)
				for i := range copies {
					copies[i] = ops[4]
				}
				ops = append(append(append([]*core.OpCode{}, ops[:4]...), copies...), ops[5:]...)
			}
			next, call := constructorMotionDelegation(obj, ops, 3, params, constructorParameterSlots(params), nil)
			if got := next != 0 && call != nil; got != (variant == "original") {
				t.Fatalf("delegation proof=%v next=%d call=%#v", got, next, call)
			}
		})
	}
}
