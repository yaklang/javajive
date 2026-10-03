package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"sync"
	"testing"
)

const nativeMemberProofFixture = `class NativeArchiveOwner {class Child {Child(){this(7);}Child(long n){}Object owner(){return NativeArchiveOwner.this;}}Child make(long n){return new Child(n);}}`

func TestNativeMemberProofRequiresOriginalCaptureAndConstructorGraph(t *testing.T) {
	files := nativeCompileClasses(t, nativeMemberProofFixture)
	for _, scenario := range []string{"original", "major53", "static", "duplicate ownership", "missing ownership", "wrong enclosing type", "capture flags", "duplicate capture", "class annotation", "method type annotation", "missing code", "duplicate code", "duplicate constructor", "wrong capture receiver", "wrong capture parameter", "small stack", "small locals", "handler before delegate", "this cycle", "mutated enclosing parameter", "extra capture store", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["NativeArchiveOwner$Child.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			var ctor, chain *MemberInfo
			var code, chainCode *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := obj.getUtf8(m.NameIndex)
				d, _ := obj.getUtf8(m.DescriptorIndex)
				if n != "<init>" {
					continue
				}
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						if strings.Contains(d, "J") {
							ctor = m
							code = c
						} else {
							chain = m
							chainCode = c
						}
					}
				}
			}
			if ctor == nil || chain == nil || code == nil || chainCode == nil {
				t.Fatal("fixture lacks constructor witnesses")
			}
			var table *InnerClassesAttribute
			for _, a := range obj.Attributes {
				if v, ok := a.(*InnerClassesAttribute); ok {
					table = v
				}
			}
			if table == nil || len(table.Classes) != 1 {
				t.Fatal("fixture ownership")
			}
			var work *workbudget.Budget
			switch scenario {
			case "major53":
				obj.MajorVersion = 53
			case "static":
				table.Classes[0].InnerClassAccessFlags |= 8
			case "duplicate ownership":
				table.Classes = append(table.Classes, table.Classes[0])
			case "missing ownership":
				table.Classes = nil
			case "wrong enclosing type":
				obj.Fields[0].DescriptorIndex = ctor.DescriptorIndex
			case "capture flags":
				obj.Fields[0].AccessFlags &^= 0x1000
			case "duplicate capture":
				obj.Fields = append(obj.Fields, obj.Fields[0])
			case "class annotation":
				obj.Attributes = append(obj.Attributes, &RuntimeVisibleAnnotationsAttribute{})
			case "method type annotation":
				ctor.Attributes = append(ctor.Attributes, &RuntimeVisibleTypeAnnotationsAttribute{})
			case "missing code":
				ctor.Attributes = nil
			case "duplicate code":
				ctor.Attributes = append(ctor.Attributes, code)
			case "duplicate constructor":
				obj.Methods = append(obj.Methods, ctor)
			case "wrong capture receiver":
				code.Code[0] = byte(core.OP_ALOAD_1)
			case "wrong capture parameter":
				code.Code[1] = byte(core.OP_ALOAD_0)
			case "small stack":
				code.MaxStack = 1
			case "small locals":
				code.MaxLocals = 1
			case "handler before delegate":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: 0}}
			case "this cycle":
				for _, v := range obj.ConstantPool {
					if m, ok := v.(*ConstantMethodrefInfo); ok {
						owner, k := sourceBridgeClassName(obj, m.ClassIndex)
						if k && owner == obj.GetClassName() {
							nt := obj.ConstantPool[m.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
							nt.DescriptorIndex = chain.DescriptorIndex
						}
					}
				}
			case "mutated enclosing parameter":
				code.Code = append(code.Code[:len(code.Code)-1], byte(core.OP_ACONST_NULL), byte(core.OP_ASTORE_1), byte(core.OP_RETURN))
			case "extra capture store":
				copyCode := append([]byte(nil), code.Code...)
				code.Code = append(copyCode[:len(copyCode)-1], copyCode[:5]...)
				code.Code = append(code.Code, byte(core.OP_RETURN))
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberProof(obj, work) != nil; got != (scenario == "original") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

func TestNativeMemberArchiveRequiresCompleteRepresentableFamily(t *testing.T) {
	for _, row := range []struct {
		name, source string
		accept       bool
	}{
		{"direct", nativeMemberProofFixture, true},
		{"qualified", `class NativeArchiveOwner {class Child{Child(long n){}}}class MemberUser{static Object make(NativeArchiveOwner o,long n){return o.new Child(n);}}`, true},
		{"generic child", `class NativeArchiveOwner{class Child<T>{Child(T n){}}Child<String> make(){return new Child<String>("x");}}`, true},
		{"deeper owner", `class NativeArchiveOwner{class Child{class Deep{}Child(){}}Child make(){return new Child();}}`, false},
		{"mixed static owner", `class NativeArchiveOwner{static class Static{}class Child{Child(){}}Child make(){return new Child();}}`, true},
		{"mixed anonymous owner", `class NativeArchiveOwner{class Child{Child(){}}Object make(){return new Child();}Object other(){return new Object(){};}}`, false},
		{"member superclass", `class NativeArchiveOwner{class Base{}class Child extends Base{}Object make(){return new Child();}}`, false},
		{"foreign subclass implicit owner", `class NativeArchiveOwner{class Child{Child(){}}Child make(){return new Child();}}class Sub extends NativeArchiveOwner.Child{Sub(NativeArchiveOwner o){o.super();}}`, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			z := nativeArchive(t, nativeCompileClasses(t, row.source))
			src, e := z.ReadFile("NativeArchiveOwner$Child.class")
			if e != nil {
				t.Fatal(e)
			}
			if got := strings.Contains(string(src), "original member body owned by"); got != row.accept {
				t.Fatalf("owned=%v\n%s", got, src)
			}
		})
	}
}

func TestNativeMemberArchiveKeepsPolicyPhysicalEntryAndConcurrentIdentity(t *testing.T) {
	base := nativeCompileClasses(t, strings.Replace(nativeMemberProofFixture, "this(7)", "this(7)", 1))
	version := nativeCompileClasses(t, strings.Replace(nativeMemberProofFixture, "this(7)", "this(13)", 1))
	files := map[string][]byte{}
	for n, b := range base {
		files[n] = b
		files[strings.TrimSuffix(n, ".class")+".raw"] = b
	}
	for n, b := range version {
		files["META-INF/versions/9/"+n] = b
	}
	files["META-INF/MANIFEST.MF"] = []byte("Manifest-Version: 1.0\nMulti-Release: true\n\n")
	for _, first := range []string{"NativeArchiveOwner$Child.class", "NativeArchiveOwner.raw", "META-INF/versions/9/NativeArchiveOwner.class"} {
		t.Run(first, func(t *testing.T) {
			z := nativeArchive(t, files)
			if _, e := z.ReadFile(first); e != nil {
				t.Fatal(e)
			}
			var wg sync.WaitGroup
			outputs := make(chan string, 6)
			for i := 0; i < 6; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					s, e := z.ReadFile("NativeArchiveOwner.class")
					if e != nil {
						outputs <- e.Error()
					} else {
						outputs <- string(s)
					}
				}()
			}
			wg.Wait()
			close(outputs)
			for src := range outputs {
				if !strings.Contains(src, "class Child") || !strings.Contains(src, "this(7") || strings.Contains(src, DecompileStubMarker) {
					t.Fatalf("base ownership: %s", src)
				}
			}
			for _, n := range []string{"NativeArchiveOwner$Child.raw"} {
				src, e := z.ReadFile(n)
				if e != nil || strings.Contains(string(src), "original member body owned by") {
					t.Fatalf("physical alias %s: %v %s", n, e, src)
				}
			}
			versionSource, e := z.ReadFile("META-INF/versions/9/NativeArchiveOwner.class")
			if e != nil || !strings.Contains(string(versionSource), "class Child") || !strings.Contains(string(versionSource), "this(13") || strings.Contains(string(versionSource), "this(7") {
				t.Fatalf("version member identity %v %s", e, versionSource)
			}
			versionChild, e := z.ReadFile("META-INF/versions/9/NativeArchiveOwner$Child.class")
			if e != nil || !strings.Contains(string(versionChild), "original member body owned by") {
				t.Fatalf("version child ownership %v %s", e, versionChild)
			}
			t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			src, e := z.ReadFile("NativeArchiveOwner$Child.class")
			if e != nil || strings.Contains(string(src), "original member body owned by") {
				t.Fatalf("policy alias %v %s", e, src)
			}
		})
	}
}

func TestNativeMemberFamilyRefusesUnprojectedOriginalSymbolsAndNullCheck(t *testing.T) {
	source := `class NativeArchiveOwner{class Child{Child(long n){}}Child make(long n){return new Child(n);}}class MemberUser{static Object make(NativeArchiveOwner o,long n){return o.new Child(n);}}`
	base := nativeCompileClasses(t, source)
	for _, scenario := range []string{"original", "method handle", "external enclosing field", "missing null witness", "root duplicate row", "root mismatched flags", "child body unsupported", "missing child", "class identity"} {
		t.Run(scenario, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range base {
				files[n] = append([]byte(nil), b...)
			}
			path := "NativeArchiveOwner.class"
			if scenario == "external enclosing field" || scenario == "missing null witness" {
				path = "MemberUser.class"
			}
			if scenario == "child body unsupported" {
				path = "NativeArchiveOwner$Child.class"
			}
			obj, e := Parse(files[path])
			if e != nil {
				t.Fatal(e)
			}
			switch scenario {
			case "method handle":
				for i, v := range obj.ConstantPool {
					if m, ok := v.(*ConstantMethodrefInfo); ok {
						owner, k := sourceBridgeClassName(obj, m.ClassIndex)
						if k && owner == "NativeArchiveOwner$Child" {
							obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: uint16(i + 1)})
							break
						}
					}
				}
			case "external enclosing field":
				cp := NewConstantPoolWithConstant(&obj.ConstantPool)
				ref := cp.newMemberrefInfo("NativeArchiveOwner$Child", "this$0", "LNativeArchiveOwner;")
				cp.AppendConstantInfo(&ConstantFieldrefInfo{ConstantMemberrefInfo: *ref})
			case "missing null witness":
				for _, m := range obj.Methods {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							d := core.NewDecompiler(c.Code, func(int) values.JavaValue { return nil })
							if e := d.ParseOpcode(); e != nil {
								t.Fatal(e)
							}
							for _, op := range d.Opcodes() {
								if call := constructorMotionMember(obj, op, core.OP_INVOKESTATIC); call != nil && call.Name == "java/util/Objects" && call.Member == "requireNonNull" {
									pc := int(op.CurrentOffset)
									c.Code[pc] = byte(core.OP_NOP)
									c.Code[pc+1] = byte(core.OP_NOP)
									c.Code[pc+2] = byte(core.OP_NOP)
								}
							}
						}
					}
				}
			case "root duplicate row":
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						table.Classes = append(table.Classes, table.Classes[0])
						table.NumberOfClasses = uint16(len(table.Classes))
						table.AttrLen = uint32(2 + 8*len(table.Classes))
					}
				}
			case "root mismatched flags":
				for _, a := range obj.Attributes {
					if table, ok := a.(*InnerClassesAttribute); ok {
						table.Classes[0].InnerClassAccessFlags |= 8
					}
				}
			case "child body unsupported":
				for _, m := range obj.Methods {
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							c.Code[len(c.Code)-1] = byte(core.OP_JSR)
						}
					}
				}
			case "missing child":
				delete(files, "NativeArchiveOwner$Child.class")
			case "class identity":
				cp := NewConstantPoolWithConstant(&obj.ConstantPool)
				obj.ThisClass = uint16(cp.AddNewClassInfo("WrongArchiveOwner"))
			}
			if scenario != "missing child" {
				files[path] = obj.Bytes()
				if _, e := Parse(files[path]); e != nil {
					t.Fatalf("mutation unexpectedly invalidated class syntax: %v", e)
				}
			}
			z := nativeArchive(t, files)
			root, e := z.ReadFile("NativeArchiveOwner.class")
			if e != nil {
				t.Fatal(e)
			}
			if got := strings.Contains(string(root), "class Child"); got != (scenario == "original") {
				t.Fatalf("root owned=%v\n%s", got, root)
			}
			if scenario != "missing child" {
				child, e := z.ReadFile("NativeArchiveOwner$Child.class")
				if e != nil {
					t.Fatal(e)
				}
				if strings.Contains(string(child), "original member body owned by") != (scenario == "original") {
					t.Fatalf("unproved child suppressed: %s", child)
				}
			}
		})
	}
}

func TestNativeMemberDescriptorOnlyDependencyUsesAcceptedScope(t *testing.T) {
	// The first root's original named member metadata is omitted from all type
	// references to the second family. Runtime descriptors remain authoritative.
	source := `class NativeArchiveOwner<T extends java.util.List<OtherOwner.Child>>{OtherOwner.Child field;java.util.List<OtherOwner.Child> generic;class Child{Child(){}}Child make(){return new Child();}}class OtherOwner{class Child{Child(){}}Child make(){return new Child();}}`
	files := nativeCompileClasses(t, source)
	obj, e := Parse(append([]byte(nil), files["NativeArchiveOwner.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range obj.Attributes {
		if table, ok := a.(*InnerClassesAttribute); ok {
			kept := table.Classes[:0]
			for _, row := range table.Classes {
				n, k := sourceBridgeClassName(obj, row.InnerClassInfoIndex)
				if !k || n != "OtherOwner$Child" {
					kept = append(kept, row)
				}
			}
			table.Classes = kept
			table.NumberOfClasses = uint16(len(kept))
			table.AttrLen = uint32(2 + 8*len(kept))
		}
	}
	// Keep the original constant pool rather than modify binary metadata to make
	// a test pass. The independent dependency helper also proves the field-only
	// and nested-bound edges on a clone without any CONSTANT_Class entries.
	clone, e := Parse(append([]byte(nil), files["NativeArchiveOwner.class"]...))
	if e != nil {
		t.Fatal(e)
	}
	for i, c := range clone.ConstantPool {
		if _, ok := c.(*ConstantClassInfo); ok {
			clone.ConstantPool[i] = nil
		}
	}
	refs, k := nativeMemberDependencyNames(clone, nil)
	if !k {
		t.Fatal("descriptor dependency refused")
	}
	found := false
	for _, n := range refs {
		if n == "OtherOwner$Child" {
			found = true
		}
	}
	if !found {
		t.Fatal("generic/field dependency lost")
	}
	files["NativeArchiveOwner.class"] = obj.Bytes()
	z := nativeArchive(t, files)
	child, e := z.ReadFile("NativeArchiveOwner$Child.class")
	if e != nil || strings.Contains(string(child), "original member body owned by") {
		t.Fatalf("joint dependency incorrectly owned %v %s", e, child)
	}
	second, e := z.ReadFile("OtherOwner$Child.class")
	if e != nil || !strings.Contains(string(second), "original member body owned by") {
		t.Fatalf("independent second owner refused %v %s", e, second)
	}
	root, e := z.ReadFile("NativeArchiveOwner.class")
	if e != nil || strings.Contains(string(root), "OtherOwner$Child") || !strings.Contains(string(root), "OtherOwner.Child") {
		t.Fatalf("dangling flat field/Signature type %v %s", e, root)
	}
}

func TestNativeMemberStaticTypeVariablesRequireOriginalLexicalBinding(t *testing.T) {
	files := nativeCompileClasses(t, `class NativeStaticScopeOwner<T extends Number>{static class Box<U extends Number>{U field;Box(U v){field=v;}<V extends U> V echo(V v){return v;}static <W extends Number> W own(W v){return v;}}}`)
	for _, scenario := range []string{"original", "field outer variable", "class bound outer variable", "method bound outer variable", "static field class variable", "static method class variable", "method own shadow", "malformed signature", "duplicate signature", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(append([]byte(nil), files["NativeStaticScopeOwner$Box.class"]...))
			if err != nil {
				t.Fatal(err)
			}
			pool := NewConstantPoolWithConstant(&obj.ConstantPool)
			replace := func(attrs *[]AttributeInfo, sig string) {
				var kept []AttributeInfo
				for _, a := range *attrs {
					if _, ok := a.(*SignatureAttribute); !ok {
						kept = append(kept, a)
					}
				}
				*attrs = append(kept, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(pool.AddUtf8Info(sig))})
			}
			var echo, own *MemberInfo
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if n == "echo" {
					echo = m
				}
				if n == "own" {
					own = m
				}
			}
			if echo == nil || own == nil || len(obj.Fields) != 1 {
				t.Fatal("missing signature witnesses")
			}
			var work *workbudget.Budget
			switch scenario {
			case "field outer variable":
				replace(&obj.Fields[0].Attributes, "TT;")
			case "class bound outer variable":
				replace(&obj.Attributes, "<U:TT;>Ljava/lang/Object;")
			case "method bound outer variable":
				replace(&echo.Attributes, "<V:TT;>(TV;)TV;")
			case "static field class variable":
				obj.Fields[0].AccessFlags |= 8
			case "static method class variable":
				replace(&own.Attributes, "<W:TU;>(TW;)TW;")
			case "method own shadow":
				replace(&echo.Attributes, "<T:Ljava/lang/Number;>(TT;)TT;")
			case "malformed signature":
				replace(&echo.Attributes, "<V:TU;>(TV;)TV")
			case "duplicate signature":
				obj.Attributes = append(obj.Attributes, obj.Attributes[0])
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			want := scenario == "original" || scenario == "method own shadow"
			if got := nativeMemberProof(obj, work) != nil; got != want {
				t.Fatalf("accepted=%v want %v", got, want)
			}
		})
	}
}

func TestNativeMemberOwnGenericScopeRequiresEnclosingDeclarations(t *testing.T) {
	files := nativeCompileClasses(t, `class NativeGenericProofOwner<T extends Number>{class Child<U extends T>{U field;Child(U n){field=n;}<V extends U> V echo(V n){return n;}}}`)
	for _, scenario := range []string{"original", "missing owner", "wrong owner", "missing owner signature", "duplicate owner signature", "free class bound", "free field variable", "free method bound", "constructor signature includes outer", "duplicate constructor signature", "method own shadow", "budget"} {
		t.Run(scenario, func(t *testing.T) {
			owner, e := Parse(append([]byte(nil), files["NativeGenericProofOwner.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			child, e := Parse(append([]byte(nil), files["NativeGenericProofOwner$Child.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			pool := NewConstantPoolWithConstant(&child.ConstantPool)
			replace := func(attrs *[]AttributeInfo, sig string) {
				var kept []AttributeInfo
				for _, a := range *attrs {
					if _, ok := a.(*SignatureAttribute); !ok {
						kept = append(kept, a)
					}
				}
				*attrs = append(kept, &SignatureAttribute{SignatureIndex: uint16(pool.AddUtf8Info(sig))})
			}
			var ctor, echo *MemberInfo
			var field *MemberInfo
			for _, m := range child.Methods {
				n, _ := sourceBridgeUTF8(child, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
				if n == "echo" {
					echo = m
				}
			}
			for _, f := range child.Fields {
				n, _ := sourceBridgeUTF8(child, f.NameIndex)
				if n == "field" {
					field = f
				}
			}
			if ctor == nil || echo == nil || field == nil {
				t.Fatal("missing witnesses")
			}
			var work *workbudget.Budget
			switch scenario {
			case "missing owner":
				owner = nil
			case "wrong owner":
				owner = child
			case "missing owner signature":
				owner.Attributes = nil
			case "duplicate owner signature":
				for _, a := range owner.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						owner.Attributes = append(owner.Attributes, a)
						break
					}
				}
			case "free class bound":
				replace(&child.Attributes, "<U:TX;>Ljava/lang/Object;")
			case "free field variable":
				replace(&field.Attributes, "TX;")
			case "free method bound":
				replace(&echo.Attributes, "<V:TX;>(TV;)TV;")
			case "constructor signature includes outer":
				replace(&ctor.Attributes, "(LNativeGenericProofOwner<TT;>;TU;)V")
			case "duplicate constructor signature":
				for _, a := range ctor.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						ctor.Attributes = append(ctor.Attributes, a)
						break
					}
				}
			case "method own shadow":
				replace(&echo.Attributes, "<T:Ljava/lang/Number;>(TT;)TT;")
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			want := scenario == "original" || scenario == "method own shadow"
			if got := nativeMemberProofWithOwner(child, owner, work) != nil; got != want {
				t.Fatalf("accepted=%v want %v", got, want)
			}
		})
	}
}
