package javaclassparser

import (
	"context"
	"fmt"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousUnusedEnclosingNeedsCompleteNonserializableAncestry(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"UnusedOwner.java": `class UnusedOwner{Object value=new Object(){};}`}, "none", "21")
	for _, variant := range []string{"original", "missing provider", "missing parent", "incomplete parents", "wrong identity", "serializable", "cycle", "budget", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(files["UnusedOwner$1.class"])
			if err != nil {
				t.Fatal(err)
			}
			declarations := map[string]callbinding.Class{
				"UnusedOwner$1":    {Name: "UnusedOwner$1", Parents: []string{"java/lang/Object"}, ParentsComplete: true},
				"java/lang/Object": {Name: "java/lang/Object", ParentsComplete: true},
			}
			provider := callbinding.Provider(func(name string) (callbinding.Class, bool) { d, ok := declarations[name]; return d, ok })
			var work *workbudget.Budget
			switch variant {
			case "missing provider":
				provider = nil
			case "missing parent":
				delete(declarations, "java/lang/Object")
			case "incomplete parents", "wrong identity", "serializable", "cycle":
				d := declarations["UnusedOwner$1"]
				if variant == "incomplete parents" {
					d.ParentsComplete = false
				} else if variant == "wrong identity" {
					d.Name = "Foreign"
				} else if variant == "serializable" {
					d.Parents = []string{"java/io/Serializable"}
				} else {
					d.Parents = []string{"UnusedOwner$1"}
				}
				declarations["UnusedOwner$1"] = d
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousNonserializable(obj, provider, work); got != (variant == "original") {
				t.Fatal("unknown or serializable ancestry granted unused-field compiler role", got)
			}
		})
	}
}

func TestNativeModernSourceNestPreservesNamespaceAndReciprocalChecks(t *testing.T) {
	for _, release := range []string{"11", "18", "21"} {
		files := nativeCompileSourceReleaseClasses(t, map[string]string{"UnusedOwner.java": `class UnusedOwner{Object value=new Object(){};}`}, "none", release)
		for _, variant := range []string{"original", "preview", "future version", "missing nest", "one-way nest", "dynamic CP", "module CP", "typed nil", "budget"} {
			t.Run(release+"/"+variant, func(t *testing.T) {
				root, err := Parse(append([]byte(nil), files["UnusedOwner.class"]...))
				if err != nil {
					t.Fatal(err)
				}
				copies := map[string][]byte{}
				for name, raw := range files {
					copies[name] = raw
				}
				d := NewClassObjectDumper(root)
				d.options.TargetSourceVersion = int(root.MajorVersion) - 44
				d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := copies[name+".class"]; return raw, ok }
				switch variant {
				case "preview":
					root.MinorVersion = 65535
				case "future version":
					root.MajorVersion = 66
				case "missing nest":
					modernNestTestRemoveAttribute(root, "NestMembers")
				case "one-way nest":
					child, _ := Parse(append([]byte(nil), files["UnusedOwner$1.class"]...))
					modernNestTestRemoveAttribute(child, "NestHost")
					copies["UnusedOwner$1.class"] = child.Bytes()
				case "dynamic CP":
					root.ConstantPool = append(root.ConstantPool, &ConstantDynamicInfo{})
				case "module CP":
					root.ConstantPool = append(root.ConstantPool, &ConstantModuleInfo{})
				case "typed nil":
					root.ConstantPool = append(root.ConstantPool, (*ConstantClassInfo)(nil))
				case "budget":
					d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				}
				objects, known := d.nativeModernNestOriginalScope()
				if known != (variant == "original") || known && !nativeModernNestSourceScopeClosed(objects, objects, nil) {
					t.Fatal("source-profile choice granted unproved reciprocal nest", known)
				}
			})
		}
	}
}

func TestNativeAnonymousUnusedEnclosingParameterIsNeverReadOrWritten(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"UnusedOwner.java": `class UnusedOwner{Object value=new Object(){};}`}, "none", "21")
	for _, variant := range []string{"original", "no capability", "wrong owner descriptor", "read word", "write word", "extra unused word", "wrong flags", "budget"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["UnusedOwner$1.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			var constructor *MemberInfo
			var code *CodeAttribute
			for _, method := range obj.Methods {
				if name, _ := sourceBridgeUTF8(obj, method.NameIndex); name == "<init>" {
					constructor = method
					for _, attr := range method.Attributes {
						if body, ok := attr.(*CodeAttribute); ok {
							code = body
						}
					}
				}
			}
			if constructor == nil || code == nil || len(code.Code) != 5 || code.Code[4] != 0xb1 {
				t.Fatal("actual unused-enclosing constructor packet missing")
			}
			allow, owner := true, "UnusedOwner"
			var work *workbudget.Budget
			switch variant {
			case "no capability":
				allow = false
			case "wrong owner descriptor":
				owner = "Foreign"
			case "read word":
				code.Code = append(append([]byte{}, code.Code[:4]...), 0x2b, 0x57, 0xb1) // aload1; pop; return
			case "write word":
				code.Code = append(append([]byte{}, code.Code[:4]...), 0x01, 0x4c, 0xb1) // null; astore1; return
			case "extra unused word":
				cp := NewConstantPoolWithConstant(&obj.ConstantPool)
				constructor.DescriptorIndex = uint16(cp.AddUtf8Info("(LUnusedOwner;I)V"))
			case "wrong flags":
				obj.AccessFlags |= 0x0010
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			packet := nativeAnonymousConstructorWithRoles(obj, owner, "", "", work, nil, nil, nil, 0x0020, allow, nil)
			if (packet != nil) != (variant == "original") || packet != nil && !packet.unusedEnclosing {
				t.Fatal("unused role ignored original word data flow", packet)
			}
		})
	}
}

// The independent javac originals establish the field-emission truth table.
// A source-version choice cannot silently erase an existing observable field.
func TestNativeAnonymousCompilerProfileRetainsEnclosingDeclarationAcrossTargets(t *testing.T) {
	for _, release := range []string{"8", "18", "21"} {
		for _, serializable := range []bool{false, true} {
			for _, readsOuter := range []bool{false, true} {
				ancestry, expression := "", "17"
				if serializable {
					ancestry = " implements java.io.Serializable"
				}
				if readsOuter {
					expression = "CrossOwner.this.word"
				}
				files := nativeCompileSourceReleaseClasses(t, map[string]string{
					"CrossOwner.java": `class CrossOwner{long word=31;CrossParent value=new CrossParent(){long get(){return ` + expression + `;}};}class CrossParent` + ancestry + `{long get(){return 0;}}`,
				}, "none", release)
				root, _ := Parse(files["CrossOwner.class"])
				child, _ := Parse(files["CrossOwner$1.class"])
				for _, target := range []int{8, 21} {
					t.Run(fmt.Sprintf("original%s/serial=%t/read=%t/target%d", release, serializable, readsOuter, target), func(t *testing.T) {
						originalField := release == "8" || serializable || readsOuter
						fieldPresent := false
						for _, field := range child.Fields {
							name, _ := sourceBridgeUTF8(child, field.NameIndex)
							fieldPresent = fieldPresent || name == "this$0"
						}
						if fieldPresent != originalField {
							t.Fatal("actual independent compiler oracle differs", fieldPresent, originalField)
						}
						d := NewClassObjectDumper(root)
						d.options.TargetSourceVersion = target
						d.options.SourceCompiler = ModernJavac
						d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return raw, ok }
						packet := d.nativeAnonymousConstructorForCompiler(child, "CrossOwner", "", "", nil, nil, d.buildInvocationMetadata(), nil)
						outputField := target < 18 || serializable || readsOuter
						if (packet != nil) != (originalField == outputField) {
							t.Fatalf("selected compiler changes original enclosing field: present=%t expected=%t packet=%v", originalField, outputField, packet)
						}
					})
				}
			}
		}
	}
}
