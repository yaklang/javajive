package javaclassparser

import (
	"fmt"
	"testing"
)

func TestAdversarialRawInheritedFieldReadAndWriteHierarchy(t *testing.T) {
	for _, row := range []struct{ name, formals, parent, mid, baseField, typ, erased, values, pollution string }{
		{"generic_chain", "<E>", "p.Mid<E>", `package p;public class Mid<U> extends Base<U>{}`, "T", "E", "Object", `new Object[]{null,new Object(),new StringBuilder("identity")}`, `new Object()`},
		{"raw_generic_middle", "", "p.Mid", `package p;public class Mid<U> extends Base<U>{}`, "T", "Object", "Object", `new Object[]{null,new Object(),new StringBuilder("identity")}`, `new Object()`},
		{"nongeneric_raw_middle", "", "p.Mid", `package p;public class Mid extends Base{}`, "T", "Object", "Object", `new Object[]{null,new Object(),new StringBuilder("identity")}`, `new Object()`},
		{"array_chain", "<E>", "p.Mid<E>", `package p;public class Mid<U> extends Base<U>{}`, "T[]", "E[]", "Object[]", `new Object[]{null,new Object[0],new Object[]{new Object()}}`, `new Object[]{new Object()}`},
		{"list_chain", "<E>", "p.Mid<E>", `package p;public class Mid<U> extends Base<U>{}`, "java.util.List<T>", "java.util.List<E>", "java.util.List", `new Object[]{null,new java.util.ArrayList(),java.util.Arrays.asList(new Object())}`, `java.util.Arrays.asList(new Object())`},
		{"map_chain", "<E>", "p.Mid<E>", `package p;public class Mid<U> extends Base<U>{}`, "java.util.Map<String,T>", "java.util.Map<String,E>", "java.util.Map", `new Object[]{null,new java.util.HashMap(),java.util.Collections.singletonMap("key",new Object())}`, `java.util.Collections.singletonMap("key",new Object())`},
	} {
		t.Run(row.name, func(t *testing.T) {
			owner := "package q;public class FieldOwner" + row.formals + " extends " + row.parent + "{public class Reader{public " + row.typ + " get(){return payload;}}public class Writer{public " + row.typ + " put(" + row.typ + " next){return payload=next;}}}"
			driver := fmt.Sprintf(`package q;public class FieldDriver{public static void main(String[]args)throws Exception{FieldOwner owner=new FieldOwner();FieldOwner.Writer writer=owner.new Writer();FieldOwner.Reader reader=owner.new Reader();if(FieldOwner.Writer.class.getDeclaringClass()!=FieldOwner.class||FieldOwner.Reader.class.getDeclaringClass()!=FieldOwner.class)throw new AssertionError("source binding");java.lang.reflect.Method put=FieldOwner.Writer.class.getDeclaredMethod("put",%s.class);Class<?> carrier=%s.class;java.lang.reflect.Method access=null,get=null;for(java.lang.reflect.Method method:FieldOwner.class.getDeclaredMethods()){Class<?>[]params=method.getParameterTypes();if(!method.getName().startsWith("access$")||!method.isSynthetic()||!java.lang.reflect.Modifier.isStatic(method.getModifiers())||method.getReturnType()!=carrier||params.length<1||params[0]!=FieldOwner.class)continue;if(params.length==1){if(get!=null)throw new AssertionError("duplicate read bridge");get=method;}if(params.length==2&&params[1]==carrier){if(access!=null)throw new AssertionError("duplicate write bridge");access=method;}}if(get==null||access==null)throw new AssertionError("missing original read/write bridge");access.setAccessible(true);if(!access.isSynthetic()||access.getReturnType()!=%s.class)throw new AssertionError("erased original accessor ABI");get.setAccessible(true);if(!get.isSynthetic())throw new AssertionError("erased getter ABI");int rows=0;for(Object value:%s){Object pollution=%s;if(access.invoke(null,owner,pollution)!=pollution||((p.Base)owner).read()!=pollution||get.invoke(null,owner)!=pollution)throw new AssertionError("erased store/result must not narrow");Object got=put.invoke(writer,new Object[]{value});if(got!=value||((p.Base)owner).read()!=value||reader.get()!=value)throw new AssertionError("instantiated writer identity");rows++;}System.out.println(rows+":raw:hierarchy:storage:identity");}}`, row.erased, row.erased, row.erased, row.values, row.pollution)
			sources := map[string]string{"p/Base.java": "package p;public class Base<T>{protected " + row.baseField + " payload;public " + row.baseField + " read(){return payload;}}", "p/Mid.java": row.mid, "q/FieldOwner.java": owner, "q/FieldDriver.java": driver}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/FieldOwner", "q.FieldDriver", "3:raw:hierarchy:storage:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
		})
	}
}
