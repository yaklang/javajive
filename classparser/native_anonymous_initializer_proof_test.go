package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAnonymousInitializerRequiresExactPostSuperPackets(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousInitializerFixture)
	for _, variant := range []string{"original", "foreign receiver", "foreign target", "captured store", "wrong descriptor", "wrong load width", "non-captured parameter", "bad literal bytes", "effectful RHS", "duplicate store", "duplicate final store", "handler", "small stack", "small locals", "ConstantValue", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["AnonymousInitOwner$1.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var code *CodeAttribute
			for _, m := range obj.Methods {
				name, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if name == "<init>" {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if code == nil {
				t.Fatal("original constructor")
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(d)
			start := -1
			for i, op := range ops {
				call := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL)
				if call != nil && call.Member == "<init>" && call.Name == obj.GetSupperClassName() {
					start = i + 1
					break
				}
			}
			if start < 0 {
				t.Fatal("original SUPER")
			}
			field := obj.Fields[0]
			for _, f := range obj.Fields {
				name, _ := sourceBridgeUTF8(obj, f.NameIndex)
				if name == "initial" {
					field = f
				}
			}
			switch variant {
			case "foreign receiver":
				code.Code[ops[start].CurrentOffset] = byte(core.OP_ALOAD_1)
			case "foreign target":
				refIndex := core.Convert2bytesToInt(ops[start+2].Data)
				obj.ConstantPool[refIndex-1].(*ConstantFieldrefInfo).ClassIndex = obj.SuperClass
			case "captured store":
				for _, f := range obj.Fields {
					name, _ := sourceBridgeUTF8(obj, f.NameIndex)
					if name == "val$wide" {
						refIndex := core.Convert2bytesToInt(ops[start+2].Data)
						ref := obj.ConstantPool[refIndex-1].(*ConstantFieldrefInfo)
						nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						nt.NameIndex = f.NameIndex
						nt.DescriptorIndex = f.DescriptorIndex
					}
				}
			case "wrong descriptor":
				refIndex := core.Convert2bytesToInt(ops[start+2].Data)
				ref := obj.ConstantPool[refIndex-1].(*ConstantFieldrefInfo)
				nt := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				obj.ConstantPool[nt.DescriptorIndex-1].(*ConstantUtf8Info).Value = "J"
			case "wrong load width":
				code.Code[ops[start+4].CurrentOffset] = byte(core.OP_FLOAD_3)
			case "non-captured parameter":
				code.Code[ops[start+4].CurrentOffset] = byte(core.OP_ALOAD_2)
			case "bad literal bytes":
				code.Code[ops[start+1].CurrentOffset] = byte(core.OP_LDC2_W)
			case "effectful RHS":
				code.Code[ops[start+1].CurrentOffset] = byte(core.OP_INVOKESTATIC)
			case "duplicate final store":
				field.AccessFlags |= 0x10
				fallthrough
			case "duplicate store":
				offset := int(ops[start].CurrentOffset)
				packet := append([]byte(nil), code.Code[offset:int(ops[start+2].CurrentOffset)+3]...)
				code.Code = append(code.Code[:len(code.Code)-1], append(packet, byte(core.OP_RETURN))...)
			case "handler":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: uint16(ops[start].CurrentOffset), EndPc: uint16(len(code.Code) - 1), HandlerPc: uint16(len(code.Code) - 1)})
			case "small stack":
				code.MaxStack = 1
			case "small locals":
				code.MaxLocals = 1
			case "ConstantValue":
				field.Attributes = append(field.Attributes, &ConstantValueAttribute{})
			}
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			child := nativeAnonymousConstructor(obj, "AnonymousInitOwner", "make(Ljava/lang/Object;Ljava/lang/Object;D)LAnonymousInitParent;", work)
			if variant == "duplicate store" {
				// The literal packet shortcut still requires unique fields. The
				// expression capability instead retains both mutable writes as
				// distinct original-PC events; it must not discard either store.
				if child == nil || child.expressionInitializer == nil || len(child.initializers) != 0 || len(child.expressionInitializer.stores) != 6 {
					t.Fatal("repeated mutable writes require complete ordered expression proof")
				}
				writes := 0
				for _, name := range child.expressionInitializer.stores {
					if name == "initial" {
						writes++
					}
				}
				if writes != 2 {
					t.Fatalf("original initial writes=%d", writes)
				}
				return
			}
			if (child != nil) != (variant == "original") {
				t.Fatalf("proof=%v", child != nil)
			}
			if child != nil {
				if len(child.initializers) != 5 {
					t.Fatal("original ordered stores")
				}
				for _, init := range child.initializers {
					if init.capture == "" {
						continue
					}
					bindings := map[string]string{init.capture: "ref"}
					if source, known := nativeAnonymousInitializerSource(child, bindings, &class_context.ClassContext{}, nil, nil); known {
						t.Fatalf("captured name collides with original own field: %s", source)
					}
					break
				}
			}
		})
	}
}

func TestNativeAnonymousInitializerRefusesInheritedCaptureRebinding(t *testing.T) {
	// Adding a field to the original parent does not change the old JVM's
	// captured-parameter store. The same unqualified source spelling would read
	// that new parent field instead; rebuilding must refuse this family.
	fixture := strings.Replace(nativeAnonymousInitializerFixture, "final Object seed;", "Object reserve;final Object seed;", 1)
	files := nativeCompileClasses(t, fixture)
	parent, err := Parse(files["AnonymousInitParent.class"])
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	for _, field := range parent.Fields {
		name, known := sourceBridgeUTF8(parent, field.NameIndex)
		if known && name == "reserve" {
			parent.ConstantPool[field.NameIndex-1].(*ConstantUtf8Info).Value = "captured"
			changed = true
		}
	}
	if !changed {
		t.Fatal("original field")
	}
	files["AnonymousInitParent.class"] = parent.Bytes()
	_, java := t04Tools(t)
	original := t.TempDir()
	for n, raw := range files {
		if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "AnonymousInitDriver"); got != "6:anonymous:initializer:callback:identity:owner\n" {
		t.Fatalf("valid original JVM=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	source, err := z.ReadFile("AnonymousInitOwner.class")
	if err != nil || !strings.Contains(string(source), "new AnonymousInitOwner$1(") || strings.Contains(string(source), "new AnonymousInitParent(") {
		t.Fatalf("inherited field rebound a captured local instead of refusing: %s", source)
	}
}

func TestNativeAnonymousInitializerRequiresCompleteFieldGraph(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousInitializerFixture)
	objects := map[string]*ClassObject{}
	for name, raw := range files {
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		objects[strings.TrimSuffix(name, ".class")] = obj
	}
	root := objects["AnonymousInitOwner$1"]
	platform := NewClassObjectDumper(root).nativeAnnotationDeclarationResolver()
	for _, variant := range []string{"complete", "missing parent", "wrong identity", "cycle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			resolve := func(name string) (*ClassObject, bool) {
				if variant == "missing parent" && name == "AnonymousInitParent" {
					return nil, false
				}
				if variant == "wrong identity" && name == "AnonymousInitParent" {
					return root, true
				}
				if obj := objects[name]; obj != nil {
					if variant == "cycle" && name == "AnonymousInitParent" {
						copy := *obj
						copy.SuperClass = copy.ThisClass
						return &copy, true
					}
					return obj, true
				}
				return platform(name)
			}
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			names, known := nativeAnonymousInitializerFieldNames(root, resolve, work)
			if known != (variant == "complete") {
				t.Fatalf("field graph proved=%v", known)
			}
			if known && (!names["seed"] || !names["saved"]) {
				t.Fatal("original own/inherited fields absent")
			}
		})
	}
}

func TestNativeAnonymousInitializerRefusesParameterReadAfterCapturedMutation(t *testing.T) {
	// Original parent reflection can mutate the captured physical field while
	// SUPER is running. The original constructor instead reads its unchanged parameter. Source
	// lexical capture access rereads the changed field and must be refused.
	fixture := strings.Replace(nativeAnonymousInitializerFixture,
		"AnonymousInitEffects.trace+=\"P\"+first();",
		"AnonymousInitEffects.trace+=\"P\"+first();try{for(java.lang.reflect.Field f:getClass().getDeclaredFields()){if(f.isSynthetic()&&f.getType()==double.class){f.setAccessible(true);f.setDouble(this,42.0);}}}catch(ReflectiveOperationException e){throw new AssertionError(e);}", 1)
	files := nativeCompileClasses(t, fixture)
	obj, err := Parse(files["AnonymousInitOwner$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	childBefore := nativeAnonymousConstructor(obj, "AnonymousInitOwner", "make(Ljava/lang/Object;Ljava/lang/Object;D)LAnonymousInitParent;", nil)
	if childBefore == nil {
		t.Fatal("original captured field initializer")
	}
	params, _, err := callbinding.Descriptor(childBefore.descriptor)
	if err != nil {
		t.Fatal(err)
	}
	slots := constructorParameterSlots(params)
	slot := -1
	for local, index := range slots {
		if index == childBefore.fields["val$wide"] {
			slot = local
		}
	}
	if slot < 0 || slot > 255 {
		t.Fatal("original captured parameter slot")
	}
	changed := false
	for _, m := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if name != "<init>" {
			continue
		}
		for _, attr := range m.Attributes {
			code, ok := attr.(*CodeAttribute)
			if !ok {
				continue
			}
			d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := d.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(d)
			for i, op := range ops {
				store := constructorMotionMember(obj, op, core.OP_PUTFIELD)
				if store == nil || store.Member != "saved" || i < 1 {
					continue
				}
				previous := ops[i-1]
				fieldRead := constructorMotionMember(obj, previous, core.OP_GETFIELD)
				if fieldRead == nil || fieldRead.Member != "val$wide" || i < 2 || !constructorMotionLoad(ops[i-2], "Ljava/lang/Object;") || core.GetRetrieveIdx(ops[i-2]) != 0 {
					t.Fatal("original captured field initializer")
				}
				start, end := int(ops[i-2].CurrentOffset), int(op.CurrentOffset)
				replacement := []byte{byte(core.OP_DLOAD), byte(slot)}
				if slot <= 3 {
					replacement = []byte{byte(core.OP_DLOAD_0 + slot)}
				}
				body := append([]byte(nil), code.Code[:start]...)
				body = append(body, replacement...)
				body = append(body, code.Code[end:]...)
				code.Code = body

				// Straight-line original code has no branch/handler/stackmap; obsolete
				// optional line/local tables are removed rather than shifting bad ranges.
				code.Attributes = nil
				code.AttrLen = uint32(12 + len(code.Code))
				changed = true
				break
			}
		}
	}
	if !changed {
		t.Fatal("original initializer site")
	}
	files["AnonymousInitOwner$1.class"] = obj.Bytes()
	_, java := t04Tools(t)
	original := t.TempDir()
	for n, raw := range files {
		if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "AnonymousInitDriver"); got != "6:anonymous:initializer:callback:identity:owner\n" {
		t.Fatalf("valid original JVM=%q", got)
	}
	child := nativeAnonymousConstructor(obj, "AnonymousInitOwner", "make(Ljava/lang/Object;Ljava/lang/Object;D)LAnonymousInitParent;", nil)
	if child != nil {
		t.Fatal("original parameter read rebound to mutated physical capture")
	}
}
