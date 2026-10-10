package javaclassparser

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/internal/filesys"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMethodLocalEnclosingOrdinalUsesOriginalInstanceOwnerChain(t *testing.T) {
	for _, members := range []string{
		`class Level{class Inner{Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}}}`,
		`static class Level{class Inner{Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}}}`,
		`static class Level{static class Inner{Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}}}`,
		`class Level{class Middle{class Inner{Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}}}}`,
	} {
		t.Run(members, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, "class NestedFactOwner{"+members+"}", "none")
			z := nativeArchive(t, files)
			t.Cleanup(func() { z.Close() })
			root, err := Parse(files["NestedFactOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil || len(p.methodLocals) != 1 {
				t.Fatal("actual verified nested owner/local plan")
			}
			for _, local := range p.methodLocals {
				got, ok := nativeMethodLocalEnclosingField(p, local.owner.owner, nil)
				if !ok || got != local.constructor.enclosingField {
					t.Fatalf("field=%s known=%v actual=%s", got, ok, local.constructor.enclosingField)
				}
			}
		})
	}
}

// A small constant pool can repeatedly reference a very long owner. Edge count
// alone understates the storage/hash work of materialized owner/field keys.
// Exercise the production archive index with real javac field references and a
// consistently renamed owner; nothing from this metadata fixture is executed.
func TestNativeMemberArchiveFieldKeysChargeMaterializedOwnerPrefixes(t *testing.T) {
	var fields, expression strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&fields, "long f%d;", i)
		if i > 0 {
			expression.WriteByte('+')
		}
		fmt.Fprintf(&expression, "s.f%d", i)
	}
	files := nativeCompileDebugClasses(t, "class KeyStorage{"+fields.String()+"}class KeyReader{static long read(KeyStorage s){return "+expression.String()+";}}", "none")
	longOwner := strings.Repeat("LongOwner", 512)
	renamed := map[string][]byte{}
	for path, raw := range files {
		obj, err := Parse(append([]byte(nil), raw...))
		if err != nil {
			t.Fatal(err)
		}
		for _, constant := range obj.ConstantPool {
			if text, ok := constant.(*ConstantUtf8Info); ok {
				text.Value = strings.ReplaceAll(text.Value, "KeyStorage", longOwner)
			}
		}
		renamed[strings.ReplaceAll(path, "KeyStorage", longOwner)] = obj.Bytes()
	}
	for _, variant := range []string{"original short owner", "long owner within budget", "long owner allocation budget", "long owner work budget"} {
		t.Run(variant, func(t *testing.T) {
			inputs := renamed
			if variant == "original short owner" {
				inputs = files
			}
			z := nativeArchive(t, inputs)
			t.Cleanup(func() { z.Close() })
			limits := workbudget.Limits{MaxOutputBytes: 128 << 10}
			switch variant {
			case "long owner allocation budget":
				limits.MaxOutputBytes = 32 << 10
			case "long owner work budget":
				limits.MaxRequestWork = 180000
			}
			work := workbudget.New(nil, limits)
			z.archive = &archiveFSState{budget: filesys.WrapWorkBudget(work, filesys.ArchiveLimits{}), ctx: context.Background(), targetRelease: 8}
			index := z.originalMemberIndex()
			want := variant == "original short owner" || variant == "long owner within budget"
			if index.valid != want {
				t.Fatalf("valid=%v want=%v workErr=%v", index.valid, want, work.Check())
			}
			if want {
				owner := "KeyStorage"
				if variant != "original short owner" {
					owner = longOwner
				}
				for i := 0; i < 80; i++ {
					if !index.captureUsers[nativeMemberCaptureIndexKey(owner, fmt.Sprintf("f%d", i))]["KeyReader"] {
						t.Fatal("original physical field user omitted")
					}
				}
			} else if work.Check() == nil {
				t.Fatal("index must retain the canonical budget failure")
			}
		})
	}
}

func TestNativeMethodLocalHeadersCannotExportLocalGenericOrThrowsTypes(t *testing.T) {
	files := nativeCompileDebugClasses(t, `@interface ScopeMark{Class<?> value();}class NestedHeaderOwner{java.util.List<?> field;java.util.List<?> header(){return null;}Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}class Level{java.util.List<?> field;java.util.List<?> header(){return null;}Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}}}`, "none")
	for _, variant := range []string{"original", "root field signature", "member field signature", "member method signature", "member class signature", "member throws", "member superclass", "member interface", "member class annotation", "member field annotation", "member method annotation"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			t.Cleanup(func() { z.Close() })
			root, err := Parse(files["NestedHeaderOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			index := z.originalMemberIndex()
			if p == nil || !index.valid {
				t.Fatal("original owner/header proof")
			}
			owner := p.lexicalObjects["NestedHeaderOwner$Level"]
			binary := "NestedHeaderOwner$Level$1Entry"
			var classIndex uint16
			for i, constant := range owner.ConstantPool {
				if _, ok := constant.(*ConstantClassInfo); ok {
					if name, ok := sourceBridgeClassName(owner, uint16(i+1)); ok && name == binary {
						classIndex = uint16(i + 1)
					}
				}
			}
			if classIndex == 0 {
				t.Fatal("original local class identity in owner CP")
			}
			if variant == "root field signature" {
				owner = root
			}
			sig := func(text string) *SignatureAttribute {
				owner.ConstantPool = append(owner.ConstantPool, &ConstantUtf8Info{Value: text})
				return &SignatureAttribute{SignatureIndex: uint16(len(owner.ConstantPool))}
			}
			replaceSignature := func(attrs []AttributeInfo, text string) []AttributeInfo {
				kept := []AttributeInfo{}
				for _, a := range attrs {
					if _, ok := a.(*SignatureAttribute); !ok {
						kept = append(kept, a)
					}
				}
				return append(kept, sig(text))
			}
			switch variant {
			case "root field signature", "member field signature":
				owner.Fields[0].Attributes = replaceSignature(owner.Fields[0].Attributes, "Ljava/util/List<L"+binary+";>;")
			case "member method signature":
				for _, m := range owner.Methods {
					if n, _ := sourceBridgeUTF8(owner, m.NameIndex); n == "header" {
						m.Attributes = replaceSignature(m.Attributes, "()Ljava/util/List<L"+binary+";>;")
					}
				}
			case "member class signature":
				owner.Attributes = replaceSignature(owner.Attributes, "<X:L"+binary+";>Ljava/lang/Object;")
			case "member throws":
				for _, m := range owner.Methods {
					if n, _ := sourceBridgeUTF8(owner, m.NameIndex); n == "make" {
						m.Attributes = append(m.Attributes, &ExceptionsAttribute{ExceptionIndexTable: []uint16{classIndex}})
					}
				}
			case "member superclass":
				owner.SuperClass = classIndex
			case "member interface":
				owner.Interfaces = append(owner.Interfaces, classIndex)
			case "member class annotation", "member field annotation", "member method annotation":
				annotation := &RuntimeVisibleAnnotationsAttribute{Annotations: []*AnnotationAttribute{{TypeName: "LScopeMark;", ElementValuePairs: []*ElementValuePairAttribute{{Name: "value", Tag: 'c', Value: "L" + binary + ";"}}}}}
				if variant == "member class annotation" {
					owner.Attributes = append(owner.Attributes, annotation)
				} else if variant == "member field annotation" {
					owner.Fields[0].Attributes = append(owner.Fields[0].Attributes, annotation)
				} else {
					for _, m := range owner.Methods {
						if name, _ := sourceBridgeUTF8(owner, m.NameIndex); name == "header" {
							m.Attributes = append(m.Attributes, annotation)
						}
					}
				}
			}
			if got := z.nativeMethodLocalArchiveClosed(p, index); got != (variant == "original") {
				t.Fatalf("local escaped via declaration header: closed=%v", got)
			}
		})
	}
}

func TestNativeMemberArchiveFieldUsersIncludeValueCapturesAndOrdinaryNames(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class FieldIndexOwner{Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}static class Storage{long value;}static long read(Storage s){return s.value;}}class FieldIndexUser{static long read(FieldIndexOwner.Storage s){return s.value;}}`, "none")
	for _, variant := range []string{"original", "retarget physical field reference"} {
		t.Run(variant, func(t *testing.T) {
			inputs := map[string][]byte{}
			for name, raw := range files {
				inputs[name] = raw
			}
			if variant != "original" {
				foreign, err := Parse(append([]byte(nil), files["FieldIndexUser.class"]...))
				if err != nil {
					t.Fatal(err)
				}
				for _, constant := range foreign.ConstantPool {
					if field, ok := constant.(*ConstantFieldrefInfo); ok {
						name, known := sourceBridgeClassName(foreign, field.ClassIndex)
						if !known || name != "FieldIndexOwner$Storage" {
							continue
						}
						cls := foreign.ConstantPool[field.ClassIndex-1].(*ConstantClassInfo)
						foreign.ConstantPool[cls.NameIndex-1].(*ConstantUtf8Info).Value = "FieldIndexOwner$1Entry"
						nt := foreign.ConstantPool[field.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
						foreign.ConstantPool[nt.NameIndex-1].(*ConstantUtf8Info).Value = "val$seed"
					}
				}
				inputs["FieldIndexUser.class"] = foreign.Bytes()
			}
			z := nativeArchive(t, inputs)
			t.Cleanup(func() { z.Close() })
			index := z.originalMemberIndex()
			if !index.valid {
				t.Fatal("actual original CP field reference index")
			}
			key := nativeMemberCaptureIndexKey("FieldIndexOwner$1Entry", "val$seed")
			if !index.captureUsers[key]["FieldIndexOwner$1Entry"] {
				t.Fatal("actual val$ field users were omitted")
			}
			ordinary := index.captureUsers[nativeMemberCaptureIndexKey("FieldIndexOwner$Storage", "value")]
			if !ordinary["FieldIndexOwner"] {
				t.Fatal("field identity depends on textual role prefix")
			}
			if variant == "original" {
				if !ordinary["FieldIndexUser"] || index.captureUsers[key]["FieldIndexUser"] {
					t.Fatal("physical original users changed")
				}
			} else if !index.captureUsers[key]["FieldIndexUser"] {
				t.Fatal("actual retargeted value capture reference omitted")
			}
			for field := range index.captureUsers {
				if !strings.Contains(field, "\x00") {
					t.Fatal("owner/field identity must be separated")
				}
			}
		})
	}
}

// Even equal method names/descriptors and allocation PCs do not license a
// constructor in another owner. Test original family metadata/bytecode first,
// then disturb each separately instead of fabricating a positive certificate.
func TestNativeMethodLocalAllocationScopeCannotBorrowSiblingOwner(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class NestedCallOwner{static Object make(){class Entry{}return new Entry();}static class Level{static Object make(){class Entry{}return new Entry();}}}`, "none")
	for _, variant := range []string{"original", "root method", "wrong method", "wrong descriptor", "preview only", "missing declaration", "wrong NEW", "wrong invocation", "wrong constructor descriptor"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			t.Cleanup(func() { z.Close() })
			root, err := Parse(files["NestedCallOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			prepared := z.prepareNativeMemberFamily(root, snapshotJDECEnv())
			if prepared == nil {
				t.Fatal("actual nested family admission")
			}
			entry := z.finishNativeMemberFamily(prepared, z.nativeMemberLookup, false)
			if entry == nil {
				t.Fatal("actual complete source")
			}
			p := entry.family
			local := p.methodLocals["NestedCallOwner$Level$1Entry"]
			if local == nil || local.source == "" || len(local.allocations) != 1 || len(local.constructor.captures) != 0 {
				t.Fatal("actual no-capture local source")
			}
			d := NewClassObjectDumper(p.lexicalObjects[local.owner.owner])
			d.nativeMemberRoot = p
			d.nativeSourceNamesReady = true
			d.FuncCtx = &class_context.ClassContext{ClassName: local.owner.owner, FunctionName: local.owner.method, CurrentMethodDesc: local.owner.descriptor}
			newPC, invokePC := 0, 0
			for pc, site := range local.allocations {
				newPC = pc
				invokePC = site.invokePC
			}
			descriptor := local.constructor.descriptor
			switch variant {
			case "root method":
				d.obj = p.lexicalObjects[p.owner]
				d.FuncCtx.ClassName = p.owner
			case "wrong method":
				d.FuncCtx.FunctionName = "other"
			case "wrong descriptor":
				d.FuncCtx.CurrentMethodDesc = "(J)Ljava/lang/Object;"
			case "preview only":
				d.nativeSourceNamesReady = false
			case "missing declaration":
				local.source = ""
			case "wrong NEW":
				newPC++
			case "wrong invocation":
				invokePC++
			case "wrong constructor descriptor":
				descriptor = "(J)V"
			}
			d.wireNativeMethodLocalSource()
			_, ok := d.FuncCtx.SourceMethodLocalAllocation(local.object.GetClassName(), descriptor, newPC, invokePC, nil)
			if ok != (variant == "original") {
				t.Fatalf("source allocation admitted=%v", ok)
			}
			if !ok && !p.failed {
				t.Fatal("rejected source scope must invalidate family transaction")
			}
		})
	}
}

func TestNativeMethodLocalArchiveClosureRequiresActualDeclaringOwner(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class NestedUseOwner{Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}class Level{Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}}}`, "none")
	for _, variant := range []string{"original", "root constructor user", "foreign constructor user", "foreign type user", "capture field user", "method handle", "cached local owner", "missing nested object"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			t.Cleanup(func() { z.Close() })
			root, err := Parse(files["NestedUseOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil {
				t.Fatal("actual family")
			}
			index := z.originalMemberIndex()
			binary := "NestedUseOwner$Level$1Entry"
			local := p.methodLocals[binary]
			if !index.valid || local == nil {
				t.Fatal("actual archive users and local")
			}
			switch variant {
			case "root constructor user":
				index.constructors[binary][p.owner] = true
			case "foreign constructor user":
				index.constructors[binary]["Foreign"] = true
			case "foreign type user":
				index.typeUsers[binary]["Foreign"] = true
			case "capture field user":
				for field := range local.constructor.captures {
					index.captureUsers[nativeMemberCaptureIndexKey(binary, field)][p.owner] = true
				}
			case "method handle":
				index.handles[binary] = true
			case "cached local owner":
				local.owner.owner = p.owner
			case "missing nested object":
				p.children[local.owner.owner].object = nil
			}
			if got := z.nativeMethodLocalArchiveClosed(p, index); got != (variant == "original") {
				t.Fatalf("closed=%v", got)
			}
		})
	}
}

func TestNativeMethodLocalEnclosingOrdinalRefusesCachedRoleOrOwnerChanges(t *testing.T) {
	files := nativeCompileDebugClasses(t, `class NestedFactOwner{static class Level{class Inner{Object make(final long seed){class Entry{long get(){return seed;}}return new Entry();}}}}`, "none")
	for _, variant := range []string{"original", "nil family", "missing root", "missing child", "missing object", "wrong object", "cached owner", "cached name", "cached flags", "cached static", "missing lexical owner", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			t.Cleanup(func() { z.Close() })
			root, err := Parse(files["NestedFactOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			p := z.nativeMemberReader(root).planNativeMemberFamily()
			if p == nil || len(p.methodLocals) != 1 {
				t.Fatal("actual nested original plan")
			}
			name := "NestedFactOwner$Level$Inner"
			child := p.children[name]
			var work *workbudget.Budget
			switch variant {
			case "nil family":
				p = nil
			case "missing root":
				delete(p.lexicalObjects, p.owner)
			case "missing child":
				delete(p.children, name)
			case "missing object":
				child.object = nil
			case "wrong object":
				child.object = p.lexicalObjects[p.owner]
			case "cached owner":
				child.owner = p.owner
			case "cached name":
				child.name = "Other"
			case "cached flags":
				child.flags ^= 1
			case "cached static":
				child.static = !child.static
			case "missing lexical owner":
				delete(p.lexicalObjects, name)
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			_, ok := nativeMethodLocalEnclosingField(p, name, work)
			if ok != (variant == "original") {
				t.Fatalf("admitted=%v", ok)
			}
		})
	}
}
