package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

const modernNestInputFixture = `class InputReceiptOwner {
 static class Child {
  static java.util.function.Supplier<String> make(int captured){return ()->"receipt:"+captured;}
  static int marker(){return 17;}
 }
 static java.util.function.Supplier<String> make(int captured){return Child.make(captured);}
}`

func modernNestInputChangedCode(t *testing.T, raw []byte) []byte {
	t.Helper()
	object, err := Parse(bytes.Clone(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name != "marker" {
			continue
		}
		for _, attr := range method.Attributes {
			code, ok := attr.(*CodeAttribute)
			if !ok || code == nil {
				continue
			}
			if !bytes.Equal(code.Code, []byte{core.OP_BIPUSH, 17, core.OP_IRETURN}) {
				t.Fatal("original executable marker did not match authored constant")
			}
			code.Code[1] = 18
			changed := object.Bytes()
			if len(changed) != len(raw) || bytes.Equal(changed, raw) {
				t.Fatal("expected a same-length original Code mutation")
			}
			return changed
		}
	}
	t.Fatal("original marker method missing")
	return nil
}

func TestNativeModernNestInputRequiresEntireOriginalInputAndPrivateBuffer(t *testing.T) {
	files := nativeCompileReleaseClasses(t, modernNestInputFixture, "none", "11")
	raw := files["InputReceiptOwner$Child.class"]
	changed := modernNestInputChangedCode(t, raw)
	for _, kind := range []string{"original", "equal fresh bytes", "same name changed Code", "different length", "wrong name", "no original object", "borrowed buffer changed"} {
		t.Run(kind, func(t *testing.T) {
			borrowed := bytes.Clone(raw)
			owned, input, ok := sealNativeModernNestInput(borrowed, nil)
			if !ok {
				t.Fatal("original input was refused")
			}
			object, err := Parse(owned)
			if err != nil {
				t.Fatal(err)
			}
			input.object = object
			name, current := "InputReceiptOwner$Child", borrowed
			switch kind {
			case "equal fresh bytes":
				current = bytes.Clone(raw)
			case "same name changed Code":
				current = changed
			case "different length":
				current = append(bytes.Clone(raw), 0)
			case "wrong name":
				name = "InputReceiptOwner"
			case "no original object":
				input.object = nil
			case "borrowed buffer changed":
				copy(borrowed, changed)
				if !bytes.Equal(owned, raw) {
					t.Fatal("resolver buffer mutation contaminated private original input")
				}
			}
			got, known := input.bind(name, current, nil)
			want := kind == "original" || kind == "equal fresh bytes"
			if known != want || want && got != object || !want && got != nil {
				t.Fatalf("original input permission=%v, expected %v", known, want)
			}
			if !bytes.Equal(owned, raw) {
				t.Fatal("binding changed the retained original executable input")
			}
		})
	}
}

func TestNativeModernNestInputChargesCopyHashAndCancellation(t *testing.T) {
	files := nativeCompileReleaseClasses(t, modernNestInputFixture, "none", "11")
	raw := files["InputReceiptOwner$Child.class"]
	for _, kind := range []string{"original", "short input", "copy work", "copy allocation", "copy canceled", "bind work", "bind canceled"} {
		t.Run(kind, func(t *testing.T) {
			var sealWork, bindWork *workbudget.Budget
			inputBytes := raw
			canceled := func() *workbudget.Budget {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return workbudget.New(ctx, workbudget.Limits{})
			}
			switch kind {
			case "short input":
				inputBytes = raw[:9]
			case "copy work":
				sealWork = workbudget.New(nil, workbudget.Limits{MaxGraphScans: int64(len(raw))})
			case "copy allocation":
				sealWork = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "copy canceled":
				sealWork = canceled()
			case "bind work":
				bindWork = workbudget.New(nil, workbudget.Limits{MaxGraphScans: int64(len(raw))})
			case "bind canceled":
				bindWork = canceled()
			}
			owned, input, sealed := sealNativeModernNestInput(inputBytes, sealWork)
			wantSeal := kind == "original" || kind == "bind work" || kind == "bind canceled"
			if sealed != wantSeal {
				t.Fatalf("seal=%v, expected %v", sealed, wantSeal)
			}
			if !sealed {
				if owned != nil || input.object != nil {
					t.Fatal("refused input published a partial original")
				}
				return
			}
			var err error
			input.object, err = Parse(owned)
			if err != nil {
				t.Fatal(err)
			}
			got, known := input.bind("InputReceiptOwner$Child", raw, bindWork)
			if known != (kind == "original") || !known && got != nil {
				t.Fatalf("bounded bind=%v", known)
			}
		})
	}
}

func TestNativeModernMemberFamilyRequiresStableResolverOriginalNamespace(t *testing.T) {
	files := nativeCompileReleaseClasses(t, modernNestInputFixture, "none", "11")
	childName := "InputReceiptOwner$Child"
	original := files[childName+".class"]
	changed := modernNestInputChangedCode(t, original)
	for _, kind := range []string{"equal fresh bytes", "same buffer", "changed Code on second read", "missing on second read", "wrong class on second read", "reused buffer changed", "extra byte on second read"} {
		t.Run(kind, func(t *testing.T) {
			root, err := Parse(bytes.Clone(files["InputReceiptOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			before := root.Bytes()
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 11
			reads := 0
			borrowed := bytes.Clone(original)
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				raw, found := files[name+".class"]
				if name != childName {
					return bytes.Clone(raw), found
				}
				reads++
				if reads >= 2 {
					switch kind {
					case "changed Code on second read":
						return bytes.Clone(changed), true
					case "missing on second read":
						return nil, false
					case "wrong class on second read":
						return bytes.Clone(files["InputReceiptOwner.class"]), true
					case "reused buffer changed":
						copy(borrowed, changed)
					case "extra byte on second read":
						return append(bytes.Clone(original), 0), true
					}
				}
				if kind == "same buffer" || kind == "reused buffer changed" {
					return borrowed, true
				}
				return bytes.Clone(raw), found
			}
			family := d.planNativeMemberFamily()
			if reads < 2 {
				t.Fatal("control did not reach both original nest and lexical member reads")
			}
			want := kind == "equal fresh bytes" || kind == "same buffer"
			if (family != nil) != want {
				t.Fatalf("stable original family=%v, expected %v", family != nil, want)
			}
			if family != nil {
				child := family.children[childName]
				if child == nil || child.object != family.modernNestObjects[childName] || child.object != family.lexicalObjects[childName] {
					t.Fatal("source and original permission borrowed distinct parsed declarations")
				}
			}
			if !bytes.Equal(before, root.Bytes()) {
				t.Fatal("family planning or refusal rewrote the original root")
			}
		})
	}
}
