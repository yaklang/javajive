package javaclassparser

import (
	"strings"
	"testing"
)

func TestNativeAccessorAncestorSourceChoicesStaySeparate(t *testing.T) {
	for _, variant := range []string{"direct ancestor", "transitive ancestor", "unrelated declaration", "different erased result", "retained invocation"} {
		t.Run(variant, func(t *testing.T) {
			root := `package q;public class Root extends Holder.Parent{public static class Builder{public Object read(){return Root.value;}}}`
			ss := map[string]string{"p/Base.java": `package p;public class Base{protected static Object value;}`, "q/Holder.java": `package q;public class Holder{public static class Parent extends p.Base{public Parent(){class Local{};new Local();}public static class BuilderBase{public Object read(){return Parent.value;}}}}`}
			if variant == "transitive ancestor" {
				ss["q/Middle.java"] = `package q;public class Middle extends Holder.Parent{}`
				root = strings.Replace(root, "extends Holder.Parent{", "extends Middle{", 1)
			}
			if variant == "unrelated declaration" {
				root = strings.Replace(root, "extends Holder.Parent{", "extends p.Base{", 1)
			}
			if variant == "different erased result" {
				root = strings.Replace(root, "extends Holder.Parent{", "extends Holder.Parent{private static String value;", 1)
			}
			ss["q/Root.java"] = root
			files := nativeCompileSourceReleaseClasses(t, ss, "none", "8")
			z := nativeArchive(t, files)
			defer z.Close()
			originalRoot, e := Parse(files["q/Root.class"])
			if e != nil {
				t.Fatal(e)
			}
			prepared := z.prepareNativeMemberFamilyUnpublished(originalRoot, snapshotJDECEnv())
			if prepared == nil || len(prepared.family.getters) != 1 {
				t.Fatal("original accessor family missing")
			}
			if variant == "retained invocation" {
				prepared.family.retainedAccessors = prepared.family.getters
				prepared.family.getters = nil
			}
			want := variant == "unrelated declaration" || variant == "different erased result" || variant == "retained invocation"
			got := z.nativeMemberAccessorAncestorFlagsClosed(prepared, false)
			if got != want {
				t.Fatalf("ancestor source flag closure=%v want=%v", got, want)
			}
		})
	}
}
