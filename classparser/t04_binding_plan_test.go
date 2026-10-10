package javaclassparser

import (
	"strings"
	"testing"
)

// javac and the JVM are independent oracles: exact descriptor binding must
// preserve the Object override (3), even when the receiver is narrowed to Child.
func TestBindingPlanVirtualAndInterfaceDispatch(t *testing.T) {
	for _, iface := range []bool{false, true} {
		name := "virtual"
		base := `public class BindBase { public int pick(Object x){return 1;} }`
		childHead := "extends BindBase"
		if iface {
			name = "interface"
			base = `public interface BindBase { int pick(Object x); }`
			childHead = "implements BindBase"
		}
		t.Run(name, func(t *testing.T) {
			original, classes := t04CompileRun(t, "8", "BindMain", map[string]string{
				"BindBase.java":  base,
				"BindChild.java": `public class BindChild ` + childHead + ` { public int pick(Object x){return 3;} public int pick(String x){return 2;} }`,
				"BindMain.java":  `public class BindMain { static int n; static BindChild recv(){n++;return new BindChild();} public static void main(String[] a){ BindBase b=recv(); Object x="x";System.out.println(b.pick(x));System.out.println(n); } }`,
			})
			if strings.TrimSpace(original) != "3\n1" {
				t.Fatalf("fixture oracle: %q", original)
			}
			t04RoundTripModes(t, "8", "BindMain", original, classes, func(t *testing.T, src string) {})
		})
	}
}

func TestInvocationMetadataMissingAncestorIsNotUnique(t *testing.T) {
	_, classes := t04CompileRun(t, "8", "MetaMain", map[string]string{
		"MetaBase.java":  `public class MetaBase { public int pick(Object x){return 1;} }`,
		"MetaChild.java": `public class MetaChild extends MetaBase {}`,
		"MetaMain.java":  `public class MetaMain { public static void main(String[] a){System.out.println(new MetaChild().pick("x"));} }`,
	})
	result, err := DecompileWithOptions(classes["MetaMain"], DecompileOptions{Mode: Precision, Resolve: func(n string) ([]byte, bool) {
		b, ok := classes[n]
		if n == "MetaBase" {
			return nil, false
		}
		return b, ok
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status == "complete" {
		t.Fatalf("incomplete hierarchy claimed complete: %s", result.Source)
	}
}
