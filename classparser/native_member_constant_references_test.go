package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeMemberConstantReferencesKeepSuperclassInitialization(t *testing.T) {
	const source = `class ConstantEffects{static String trace="";static int mark(){trace+="P";return 7;}}
class ConstantParent{static int marker=ConstantEffects.mark();ConstantParent(long n){}}
class ConstantOwner{class Child extends ConstantParent{static final int value=7;Child(){super(7);}Object owner(){return ConstantOwner.this;}}class Alias extends Child{Alias(){super();}}}
class ConstantProbe{static int read(){return ConstantOwner.Alias.value;}static Class<?>anchor(){return ConstantOwner.Alias.class;}}
class ConstantDriver{public static void main(String[]args){System.out.println(ConstantProbe.read()+":"+ConstantEffects.trace);}}`
	_, java := t04Tools(t)
	files := nativeCompileClasses(t, source)
	z := nativeArchive(t, files)
	defer z.Close()
	src, e := z.ReadFile("ConstantOwner$Child.class")
	if e != nil || !strings.Contains(string(src), "original member body owned by") {
		t.Fatalf("genuine constant family must be representable %v %s", e, src)
	}
	for _, alias := range []string{"ConstantOwner$Child", "ConstantOwner$Alias"} {
		t.Run(alias, func(t *testing.T) {
			object, e := Parse(files["ConstantProbe.class"])
			if e != nil {
				t.Fatal(e)
			}
			classIndex := object.ConstantPoolManager.AddNewClassInfo(alias)
			name := object.ConstantPoolManager.AddUtf8Info("value")
			descriptor := object.ConstantPoolManager.AddUtf8Info("I")
			nt := object.ConstantPoolManager.AppendConstantInfo(&ConstantNameAndTypeInfo{NameIndex: uint16(name), DescriptorIndex: uint16(descriptor)})
			ref := object.ConstantPoolManager.AppendConstantInfo(&ConstantFieldrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: uint16(classIndex), NameAndTypeIndex: uint16(nt)}})
			found := false
			for _, method := range object.Methods {
				n, _ := sourceBridgeUTF8(object, method.NameIndex)
				if n != "read" {
					continue
				}
				for _, attribute := range method.Attributes {
					if code, ok := attribute.(*CodeAttribute); ok {
						old := len(code.Code)
						code.Code = []byte{0xb2, byte(ref >> 8), byte(ref), 0xac}
						code.AttrLen = uint32(int(code.AttrLen) + len(code.Code) - old)
						found = true
					}
				}
			}
			if !found {
				t.Fatal("authored read")
			}
			mutated := map[string][]byte{}
			original := t.TempDir()
			for n, raw := range files {
				if n == "ConstantProbe.class" {
					raw = object.Bytes()
				}
				mutated[n] = raw
				if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if got := t04RunJava(t, java, original, "ConstantDriver"); got != "7:P\n" {
				t.Fatalf("GETSTATIC initialization oracle %q", got)
			}
			archive := nativeArchive(t, mutated)
			defer archive.Close()
			src, e := archive.ReadFile("ConstantOwner$Child.class")
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(string(src), "original member body owned by") {
				t.Fatalf("source constant would erase original superclass initialization via %s", alias)
			}
		})
	}
	for _, limit := range []string{"budget", "canceled"} {
		t.Run(limit, func(t *testing.T) {
			root, e := Parse(files["ConstantOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			reader := z.nativeMemberReader(root)
			family := reader.planNativeMemberFamily()
			if family == nil {
				t.Fatal("original family")
			}
			work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			if limit == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if z.nativeMemberStaticConstantsReferencesClosed(family, z.originalMemberIndex(), work) {
				t.Fatal("resource refusal lost")
			}
		})
	}
}
