package javaclassparser

import (
	"fmt"
	"testing"
)

func TestAdversarialRawInheritedFieldWriteHierarchy(t *testing.T) {
	for _, row := range []struct{ name, formals, parent, mid, baseField, typ, erased, values, pollution string }{
		{"generic_chain", "<E>", "p.Mid<E>", `package p;public class Mid<U> extends Base<U>{}`, "T", "E", "Object", `new Object[]{null,new Object(),new StringBuilder("identity")}`, `new Object()`},
		{"raw_generic_middle", "", "p.Mid", `package p;public class Mid<U> extends Base<U>{}`, "T", "Object", "Object", `new Object[]{null,new Object(),new StringBuilder("identity")}`, `new Object()`},
		{"nongeneric_raw_middle", "", "p.Mid", `package p;public class Mid extends Base{}`, "T", "Object", "Object", `new Object[]{null,new Object(),new StringBuilder("identity")}`, `new Object()`},
		{"array_chain", "<E>", "p.Mid<E>", `package p;public class Mid<U> extends Base<U>{}`, "T[]", "E[]", "Object[]", `new Object[]{null,new Object[0],new Object[]{new Object()}}`, `new Object[]{new Object()}`},
		{"list_chain", "<E>", "p.Mid<E>", `package p;public class Mid<U> extends Base<U>{}`, "java.util.List<T>", "java.util.List<E>", "java.util.List", `new Object[]{null,new java.util.ArrayList(),java.util.Arrays.asList(new Object())}`, `java.util.Arrays.asList(new Object())`},
		{"map_chain", "<E>", "p.Mid<E>", `package p;public class Mid<U> extends Base<U>{}`, "java.util.Map<String,T>", "java.util.Map<String,E>", "java.util.Map", `new Object[]{null,new java.util.HashMap(),java.util.Collections.singletonMap("key",new Object())}`, `java.util.Collections.singletonMap("key",new Object())`},
	} {
		t.Run(row.name, func(t *testing.T) {
			owner := "package q;public class FieldOwner" + row.formals + " extends " + row.parent + "{public class Writer{public " + row.typ + " put(" + row.typ + " next){return payload=next;}}}"
			driver := fmt.Sprintf(`package q;public class FieldDriver{public static void main(String[]args)throws Exception{FieldOwner owner=new FieldOwner();FieldOwner.Writer writer=owner.new Writer();if(FieldOwner.Writer.class.getDeclaringClass()!=FieldOwner.class)throw new AssertionError("source binding");java.lang.reflect.Method put=FieldOwner.Writer.class.getDeclaredMethod("put",%s.class);java.lang.reflect.Method access=FieldOwner.class.getDeclaredMethod("access$002",FieldOwner.class,%s.class);access.setAccessible(true);if(!access.isSynthetic()||access.getReturnType()!=%s.class)throw new AssertionError("erased original accessor ABI");int rows=0;for(Object value:%s){Object pollution=%s;if(access.invoke(null,owner,pollution)!=pollution||((p.Base)owner).read()!=pollution)throw new AssertionError("erased store/result must not narrow");Object got=put.invoke(writer,new Object[]{value});if(got!=value||((p.Base)owner).read()!=value)throw new AssertionError("instantiated writer identity");rows++;}System.out.println(rows+":raw:hierarchy:storage:identity");}}`, row.erased, row.erased, row.erased, row.values, row.pollution)
			sources := map[string]string{"p/Base.java": "package p;public class Base<T>{protected " + row.baseField + " payload;public " + row.baseField + " read(){return payload;}}", "p/Mid.java": row.mid, "q/FieldOwner.java": owner, "q/FieldDriver.java": driver}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/FieldOwner", "q.FieldDriver", "3:raw:hierarchy:storage:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
		})
	}
}

func TestNativeRawFieldSourceHierarchyBoundaries(t *testing.T) {
	for _, row := range []struct {
		name, formals, parent, mid string
		want                       bool
	}{
		{"generic_chain", "<E>", "p.Mid<E>", `package p;public class Mid<U> extends Base<U>{}`, true},
		{"nongeneric_breaks_raw_chain", "<E>", "p.Mid", `package p;public class Mid extends Base<String>{}`, false},
		{"fixed_chain", "", "p.Mid<String>", `package p;public class Mid<U> extends Base<U>{}`, false},
		{"raw_generic_middle", "", "p.Mid", `package p;public class Mid<U> extends Base<U>{}`, true},
		{"nongeneric_raw_middle", "", "p.Mid", `package p;public class Mid extends Base{}`, true},
	} {
		for _, use := range []nativeInheritedFieldUse{nativeInheritedFieldErasedRead, nativeInheritedFieldRawWrite} {
			t.Run(fmt.Sprintf("%s/use%d", row.name, use), func(t *testing.T) {
				files := nativeCompileSourceReleaseClasses(t, map[string]string{"p/Base.java": `package p;public class Base<T>{protected T payload;}`, "p/Mid.java": row.mid, "q/FieldOwner.java": "package q;public class FieldOwner" + row.formals + " extends " + row.parent + "{}"}, "none", "8")
				objects := map[string]*ClassObject{}
				for n, b := range files {
					o, e := Parse(b)
					if e != nil {
						t.Fatal(e)
					}
					objects[n[:len(n)-6]] = o
				}
				root, owner := objects["q/FieldOwner"], objects["p/Base"]
				resolve := func(n string) (*ClassObject, bool) { o, ok := objects[n]; return o, ok }
				generic, known := nativeMemberInheritedFieldSourceSignature(root, owner, owner.Fields[0], "Ljava/lang/Object;", use, resolve, nil)
				if known != (row.want || use == nativeInheritedFieldErasedRead) || known && !generic {
					t.Fatalf("generic=%v known=%v want=%v", generic, known, row.want)
				}
			})
		}
	}
}
