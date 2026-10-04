package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAnonymousInitializerStaticReadRequiresOriginalDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousStaticReadFixture)
	for _, variant := range []string{"original", "missing", "identity", "instance", "private", "synthetic", "constant", "descriptor", "duplicate", "unsafe name", "budget"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["StaticReadHolder.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			var target *MemberInfo
			for _, f := range obj.Fields {
				n, _ := sourceBridgeUTF8(obj, f.NameIndex)
				if n == "value" {
					target = f
				}
			}
			if target == nil {
				t.Fatal("actual static declaration")
			}
			original := &values.JavaClassMember{Name: "StaticReadHolder", Member: "value", Description: "I"}
			switch variant {
			case "identity":
				obj.ThisClass = obj.SuperClass
			case "instance":
				target.AccessFlags &^= 8
			case "private":
				target.AccessFlags |= 2
			case "synthetic":
				target.AccessFlags |= 0x1000
			case "constant":
				target.Attributes = append(target.Attributes, &ConstantValueAttribute{})
			case "descriptor":
				target.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("J"))
			case "duplicate":
				obj.Fields = append(obj.Fields, target)
			case "unsafe name":
				original.Member = "class"
			}
			resolve := func(name string) (*ClassObject, bool) { return obj, variant != "missing" && name == "StaticReadHolder" }
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			if got := nativeAnonymousInitializerFieldDeclaration(original, true, resolve, work); got != (variant == "original") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}
func TestNativeAnonymousInitializerStaticReadRequiresOriginalPCAndMember(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousStaticReadFixture)
	obj, e := Parse(files["StaticReadOwner$1.class"])
	if e != nil {
		t.Fatal(e)
	}
	child := nativeAnonymousConstructor(obj, "StaticReadOwner", "make", nil)
	if child == nil || child.expressionInitializer == nil {
		t.Fatal("original constructor proof")
	}
	plan := child.expressionInitializer
	pc := -1
	for _, op := range plan.ops[plan.start:] {
		f := constructorMotionMember(obj, op, core.OP_GETSTATIC)
		if f != nil && f.Member == "value" {
			pc = int(op.CurrentOffset)
			break
		}
	}
	if pc < 0 {
		t.Fatal("actual read PC")
	}
	holder, e := Parse(files["StaticReadHolder.class"])
	if e != nil {
		t.Fatal(e)
	}
	resolve := func(n string) (*ClassObject, bool) { return holder, n == "StaticReadHolder" }
	for _, v := range []string{"original", "missing PC", "store PC", "wrong owner", "wrong name", "wrong descriptor"} {
		t.Run(v, func(t *testing.T) {
			field := &values.JavaClassMember{Name: "StaticReadHolder", Member: "value", Description: "I", OriginPC: pc, HasOriginPC: true}
			switch v {
			case "missing PC":
				field.HasOriginPC = false
			case "store PC":
				for p := range plan.stores {
					field.OriginPC = p
					break
				}
			case "wrong owner":
				field.Name = "OtherHolder"
			case "wrong name":
				field.Member = "token"
			case "wrong descriptor":
				field.Description = "J"
			}
			events := []int{}
			got := nativeAnonymousInitializerExpressionEvents(child, plan, field, &events, resolve, nil)
			if got != (v == "original") {
				t.Fatalf("accepted=%v", got)
			}
			if got && (len(events) != 1 || events[0] != pc) {
				t.Fatalf("original read effects=%v", events)
			}
		})
	}
}

// A valid original GETSTATIC can execute class initialization even when its
// field has ConstantValue metadata. Java constant-variable reads can omit that
// instruction, so the source capability must refuse that substitution.
func TestNativeAnonymousInitializerStaticConstantReadRefusesClassInitElision(t *testing.T) {
	const fixture = `class StaticConstantTrace {static String trace="";static int produce(){trace+="C";return 31;}}
class StaticConstantHolder {static final int value=StaticConstantTrace.produce();static final int poolLiteral=100009;}
abstract class StaticConstantParent {StaticConstantParent(){StaticConstantTrace.trace+="P";}abstract int number();}
class StaticConstantOwner {StaticConstantParent make(){return new StaticConstantParent(){int n=StaticConstantHolder.value;int number(){return n;}};}}
class StaticConstantDriver {public static void main(String[]args){StaticConstantTrace.trace="";StaticConstantParent p=new StaticConstantOwner().make();if(p.number()!=31||!StaticConstantTrace.trace.equals("PC"))throw new AssertionError("original GETSTATIC initialization");System.out.println("31:constant:original:classinit");}}
`
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			holder, e := Parse(append([]byte(nil), files["StaticConstantHolder.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			index := 0
			for i, c := range holder.ConstantPool {
				if n, ok := c.(*ConstantIntegerInfo); ok && n.Value == 100009 {
					index = i + 1
				}
			}
			if index == 0 {
				t.Fatal("original integer pool literal")
			}
			changed := 0
			for _, f := range holder.Fields {
				n, _ := sourceBridgeUTF8(holder, f.NameIndex)
				if n == "value" {
					f.Attributes = append(f.Attributes, &ConstantValueAttribute{AttrLen: 2, ConstantValueIndex: uint16(index)})
					changed++
				}
			}
			if changed != 1 {
				t.Fatal("one original static final field")
			}
			files["StaticConstantHolder.class"] = holder.Bytes()
			if _, e := Parse(files["StaticConstantHolder.class"]); e != nil {
				t.Fatal(e)
			}
			dir := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(dir, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if got := t04RunJava(t, java, dir, "StaticConstantDriver"); got != "31:constant:original:classinit\n" {
				t.Fatalf("valid original JVM=%q", got)
			}
			root, e := Parse(files["StaticConstantOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			entry := archive.nativeMemberEntry(root)
			if entry != nil && entry.family != nil {
				t.Fatal("constant-variable source skipped original GETSTATIC class initialization")
			}
		})
	}
}

func TestNativeAnonymousInitializerStaticReadRefusesValueNameRebinding(t *testing.T) {
	fixture := strings.Replace(nativeAnonymousStaticReadFixture, "int before=", "Object reserve;int before=", 1)
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			child, e := Parse(append([]byte(nil), files["StaticReadOwner$1.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			changed := 0
			for _, f := range child.Fields {
				n, _ := sourceBridgeUTF8(child, f.NameIndex)
				if n == "reserve" {
					f.NameIndex = uint16(child.ConstantPoolManager.AddUtf8Info("StaticReadHolder"))
					changed++
				}
			}
			if changed != 1 {
				t.Fatal("one unused original field")
			}
			files["StaticReadOwner$1.class"] = child.Bytes()
			dir := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(dir, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if got := t04RunJava(t, java, dir, "StaticReadDriver"); got != "4:static:clinit:volatile:order:identity\n" {
				t.Fatalf("valid original GETSTATIC unchanged=%q", got)
			}
			root, e := Parse(files["StaticReadOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			entry := archive.nativeMemberEntry(root)
			if entry != nil && entry.family != nil {
				t.Fatal("type-qualified static read rebound to original anonymous value field")
			}
		})
	}
}
