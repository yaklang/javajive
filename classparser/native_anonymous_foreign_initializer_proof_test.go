package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAnonymousInitializerForeignReadRequiresDeclaration(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousForeignFieldInitializerFixture)
	for _, variant := range []string{"original", "missing", "identity", "static", "private", "synthetic", "constant", "descriptor", "duplicate", "wrong receiver type", "no receiver type", "budget"} {
		t.Run(variant, func(t *testing.T) {
			obj, e := Parse(append([]byte(nil), files["ForeignInitHolder.class"]...))
			if e != nil {
				t.Fatal(e)
			}
			var declaration *MemberInfo
			for _, field := range obj.Fields {
				n, _ := sourceBridgeUTF8(obj, field.NameIndex)
				if n == "value" {
					declaration = field
				}
			}
			if declaration == nil {
				t.Fatal("actual field")
			}
			owner := "ForeignInitHolder"
			receiver := values.NewJavaRef(nil, nil, types.NewJavaClass(owner))
			switch variant {
			case "identity":
				obj.ThisClass = obj.SuperClass
			case "static":
				declaration.AccessFlags |= 8
			case "private":
				declaration.AccessFlags |= 2
			case "synthetic":
				declaration.AccessFlags |= 0x1000
			case "constant":
				declaration.Attributes = append(declaration.Attributes, &ConstantValueAttribute{})
			case "descriptor":
				declaration.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("Ljava/lang/String;"))
			case "duplicate":
				obj.Fields = append(obj.Fields, declaration)
			case "wrong receiver type":
				receiver.ResetVarType(types.NewJavaClass("DifferentHolder"))
			case "no receiver type":
				receiver.ResetVarType(nil)
			}
			resolve := func(name string) (*ClassObject, bool) { return obj, variant != "missing" && name == owner }
			var work *workbudget.Budget
			if variant == "budget" {
				work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			}
			got := nativeAnonymousInitializerForeignField(&values.JavaClassMember{Name: owner, Member: "value", Description: "Ljava/lang/Object;"}, receiver, resolve, work)
			if got != (variant == "original") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

// The JVM original selects a superclass field through an upcast, although the
// receiver descriptor denotes the subclass. A same-spelled subclass field has
// a different value. Do not emit an uncast source access and silently rebind it.
func TestNativeAnonymousInitializerForeignReadRefusesHiddenFieldRebinding(t *testing.T) {
	f := strings.Replace(nativeAnonymousForeignFieldInitializerFixture, `class ForeignInitHolder {`, `class ForeignInitBase {Object value;}class ForeignInitHolder extends ForeignInitBase {`, 1)
	f = strings.Replace(f, `this.value=value;`, `super.value=value;this.value=new Object();`, 1)
	f = strings.Replace(f, `holder.next.value`, `((ForeignInitBase)holder.next).value`, 1)
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, f, debug)
			out := t.TempDir()
			for name, raw := range files {
				if e := os.WriteFile(filepath.Join(out, name), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if got := t04RunJava(t, java, out, "ForeignInitDriver"); got != "6:foreign:field:identity:volatile:order:null\n" {
				t.Fatalf("original hidden field identity=%q", got)
			}
			obj, e := Parse(files["ForeignInitOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			// The anonymous plan has not yet rendered its initializer. Check the
			// committed source family, which runs the complete field-selection proof.
			archive := nativeArchive(t, files)
			defer archive.Close()
			entry := archive.nativeMemberEntry(obj)
			if entry != nil && entry.family != nil {
				t.Fatal("hidden subclass field rebound in source")
			}
		})
	}
}
