package javaclassparser

import (
	"context"
	"fmt"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

// More than one machine word of independent lexical members is not ambiguous
// ownership. Original getters must still regenerate with the same symbol ordinal.
func nativeWideMemberLayoutSources(root string, count int) map[string]string {
	var owner strings.Builder
	fmt.Fprintf(&owner, "public class %s{", root)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&owner, "private int f%d;", i)
	}
	fmt.Fprintf(&owner, "public %s(){", root)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&owner, "f%d=%d;", i, i)
	}
	owner.WriteString("}")
	for i := count - 1; i >= 0; i-- {
		fmt.Fprintf(&owner, "public static class Reader%03d{public Reader%03d(){} public int read(%s o){return o.f%d;}}", i, i, root, i)
	}
	owner.WriteString("}")
	driver := fmt.Sprintf(`class WideLayoutDriver{public static void main(String[]a)throws Exception{%s o=new %s();int sum=0;for(int i=0;i<%d;i++){Class<?> c=Class.forName("%s$Reader"+String.format("%%03d",i));if(c.getDeclaringClass()!=%s.class)throw new AssertionError("owner");Object r=c.getDeclaredConstructor().newInstance();Object value=c.getDeclaredMethod("read",%s.class).invoke(r,o);if(!value.equals(i))throw new AssertionError("wrong original field registration");sum+=((Integer)value).intValue();}if(sum!=%d)throw new AssertionError("independent arithmetic oracle");System.out.println("%d:wide:lexical:original:registration:identity");}}`, root, root, count, root, root, root, count*(count-1)/2, count)
	return map[string]string{root + ".java": owner.String(), "WideLayoutDriver.java": driver}
}
func TestAdversarialWideMemberRegistrationRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeWideMemberLayoutSources("WideLayoutOwner", 70), "WideLayoutOwner", "WideLayoutDriver", "70:wide:lexical:original:registration:identity\n")
}
func TestAdversarialWideMemberRegistrationRenamedRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeWideMemberLayoutSources("ChangedLayoutOwner", 67), "ChangedLayoutOwner", "WideLayoutDriver", "67:wide:lexical:original:registration:identity\n")
}

// Exercise the repair searches independently of the full renderer. The oracle
// is the first-use order of original physical accessor ordinals, not solver state.
func TestNativeWideAccessorLayoutKeepsEveryOriginalRegistration(t *testing.T) {
	for _, count := range []int{63, 64, 65, 70, 129} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, nativeWideMemberLayoutSources("WideLayoutOwner", count), "none", "8")
			z := nativeArchive(t, files)
			defer z.Close()
			obj, e := Parse(files["WideLayoutOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			p := z.nativeMemberReader(obj).planNativeMemberFamily()
			if p == nil || len(p.children) != count || len(p.getters) != count {
				t.Fatal("complete original member forest")
			}
			objects, known := nativeMemberDependencyObjects(obj, p, nil)
			if !known || len(objects) != count+1 {
				t.Fatal("dependency phase lost a proved lexical node")
			}
			key := fmt.Sprintf("%s$Reader%03d", p.owner, count-1)
			member := p.children[key]
			originalObject := member.object
			member.object = obj
			if _, known := nativeMemberDependencyObjects(obj, p, nil); known {
				t.Fatal("foreign declaration licensed by wider graph")
			}
			member.object = originalObject

			members := make([]string, count)
			owners := make([]string, count)
			for _, g := range p.getters {
				i := g.ordinal / 100
				// Reverse input order; an independent ordinal oracle requires 0..69.
				class := fmt.Sprintf("Reader%03d", count-1-i)
				members[count-1-i] = fmt.Sprintf("class %s{Object read(){return null;/*jdec-owned-getter:%d:%s:%s*/}}", class, g.ordinal, g.owner, g.field)
				owners[count-1-i] = p.owner + "$" + class
			}
			source := "class WideLayoutOwner{Object before=new Object();Object after=new Object();}"
			for _, tree := range []bool{false, true} {
				if tree {
					p.registrationLayouts = map[string]*nativeMemberRegistrationScope{}
					// A real sealed child scope engages the nested continuation solver.
					owner := owners[0]
					p.registrationLayouts[owner] = &nativeMemberRegistrationScope{owner: owner, source: members[0], declaration: members[0]}
				}
				result, known := nativeMemberRegistrationLayout(p, source, members, nil, owners)
				if !known {
					t.Fatalf("tree=%v: original order repair refused", tree)
				}
				position := -1
				for ordinal := 0; ordinal < count; ordinal++ {
					marker := fmt.Sprintf("/*jdec-owned-getter:%d:", ordinal*100)
					at := strings.Index(result, marker)
					if at <= position || strings.Count(result, marker) != 1 {
						t.Fatalf("tree=%v ordinal=%d missing/aliased/reordered", tree, ordinal)
					}
					position = at
				}
				if strings.Index(result, "Object before=new Object();") > strings.Index(result, "Object after=new Object();") {
					t.Fatal("fixed initializer order changed")
				}
				for _, limit := range []string{"memory", "work", "canceled"} {
					var work *workbudget.Budget
					switch limit {
					case "memory":
						work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					case "work":
						work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					if output, known := nativeMemberRegistrationLayout(p, source, members, work, owners); known || output != "" {
						t.Fatalf("tree=%v %s: exhausted request published source", tree, limit)
					}
				}
			}
		})
	}
}
