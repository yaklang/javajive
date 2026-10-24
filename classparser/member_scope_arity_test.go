package javaclassparser

import (
	"reflect"
	"strings"
	"testing"
)

func TestMemberArityRequiresOriginalStaticScopeCuts(t *testing.T) {
	for _, root := range []string{"ArityRoot", "Arity$Literal"} {
		source := strings.ReplaceAll(`class ArityRoot<T extends Number>{
 class Open {class Leaf {}}
 static class Cut {class Middle {class Leaf {}}}
 static class Generic<V extends CharSequence>{class Middle {class Leaf {}}}
 class Shadow<T extends CharSequence>{class Leaf {}}
}`, "ArityRoot", root)
		files := nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, "none", "8")
		for _, row := range []struct {
			suffix string
			want   []string
		}{{"$Open$Leaf", []string{"T"}}, {"$Cut$Middle$Leaf", nil}, {"$Generic$Middle$Leaf", []string{"V"}}, {"$Shadow$Leaf", []string{"T"}}} {
			t.Run(root+row.suffix, func(t *testing.T) {
				d := NewClassObjectDumper(mustParseLowering(t, files[root+row.suffix+".class"]))
				d.foldSiblingResolver = func(name string) ([]byte, bool) { b, k := files[name+".class"]; return b, k }
				if got := d.enclosingFormalTypeParamsForArity(); !reflect.DeepEqual(got, row.want) {
					t.Fatalf("got %v want %v", got, row.want)
				}
			})
		}
	}
}
