package javaclassparser

import (
	"testing"
)

func TestNativeConstructorRuntimeClassQueryPreservesCaptureBoundary(t *testing.T) {
	files := nativeCompileClasses(t, `class QueryBase{Class<?> kind;QueryBase(){kind=getClass();kind.getDeclaredMethods();}} final class QueryChild extends QueryBase{Object capture;QueryChild(){super();}}
 class QueryOpen extends QueryBase{Object capture;QueryOpen(){super();}}
 class QueryReadBase{Object capture,seen;Class<?> kind;QueryReadBase(){kind=getClass();seen=capture;}} final class QueryReadChild extends QueryReadBase{QueryReadChild(){super();}}
 class QueryEscapeBase{static Object escaped;QueryEscapeBase(){Class<?> kind=getClass();escaped=this;}} final class QueryEscapeChild extends QueryEscapeBase{Object capture;QueryEscapeChild(){super();}}
 class QuerySelfBase{Object self;Class<?> kind;QuerySelfBase(){self=this;kind=getClass();}} final class QuerySelfChild extends QuerySelfBase{Object capture;QuerySelfChild(){super();}}
 class QueryAliasBase{static Object escaped;Object self;Class<?> kind;QueryAliasBase(){self=this;kind=getClass();escaped=self;}} final class QueryAliasChild extends QueryAliasBase{Object capture;QueryAliasChild(){super();}}`)
	resolve := func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
	for _, tc := range []struct {
		child, base, storage string
		want                 bool
	}{{"QueryChild", "QueryBase", "QueryChild", true}, {"QueryOpen", "QueryBase", "QueryOpen", false}, {"QueryReadChild", "QueryReadBase", "QueryReadBase", false}, {"QueryEscapeChild", "QueryEscapeBase", "QueryEscapeChild", false}, {"QuerySelfChild", "QuerySelfBase", "QuerySelfChild", true}, {"QueryAliasChild", "QueryAliasBase", "QueryAliasChild", false}} {
		t.Run(tc.child, func(t *testing.T) {
			o, e := Parse(append([]byte(nil), files[tc.child+".class"]...))
			if e != nil {
				t.Fatal(e)
			}
			d := NewClassObjectDumper(o)
			d.foldSiblingResolver = resolve
			d.options.TargetSourceVersion = 8
			got := d.constructorCaptureChainDoesNotObserve(tc.base, "()V", map[string]bool{tc.storage + "\x00capture\x00Ljava/lang/Object;": true})
			if got != tc.want {
				t.Fatalf("query movement %v want%v", got, tc.want)
			}
		})
	}
}
