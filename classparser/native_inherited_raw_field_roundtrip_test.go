package javaclassparser

import (
	"fmt"
	"testing"
)

func TestAdversarialRawInheritedFieldReadAndWrite(t *testing.T) {
	for _, row := range []struct{ name, formals, super, typ, erased, values string }{
		{"formal", "<E>", "E", "E", "Object", `new Object[]{null,new Object(),new StringBuilder("identity")}`},
		{"bounded", "<E extends Number>", "E", "E", "Number", `new Object[]{null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE)}`},
		{"array", "<E>", "E[]", "E[]", "Object[]", `new Object[]{null,new Object[0],new Object[]{new Object()}}`},
	} {
		t.Run(row.name, func(t *testing.T) {
			owner := "package q;public class FieldOwner" + row.formals + " extends p.Base<" + row.super + ">{public class Reader{public " + row.typ + " get(){return payload;}}public class Writer{public " + row.typ + " put(" + row.typ + " next){return payload=next;}}}"
			driver := fmt.Sprintf(`package q;public class FieldDriver{public static void main(String[]args)throws Exception{FieldOwner owner=new FieldOwner();FieldOwner.Writer writer=owner.new Writer();FieldOwner.Reader reader=owner.new Reader();if(FieldOwner.Writer.class.getDeclaringClass()!=FieldOwner.class||FieldOwner.Reader.class.getDeclaringClass()!=FieldOwner.class)throw new AssertionError("source binding");java.lang.reflect.Method put=FieldOwner.Writer.class.getDeclaredMethod("put",%s.class);Class<?> carrier=Object.class;java.lang.reflect.Method access=null,get=null;for(java.lang.reflect.Method method:FieldOwner.class.getDeclaredMethods()){Class<?>[]params=method.getParameterTypes();if(!method.getName().startsWith("access$")||!method.isSynthetic()||!java.lang.reflect.Modifier.isStatic(method.getModifiers())||method.getReturnType()!=carrier||params.length<1||params[0]!=FieldOwner.class)continue;if(params.length==1){if(get!=null)throw new AssertionError("duplicate read bridge");get=method;}if(params.length==2&&params[1]==carrier){if(access!=null)throw new AssertionError("duplicate write bridge");access=method;}}if(get==null||access==null)throw new AssertionError("missing original read/write bridge");access.setAccessible(true);if(!access.isSynthetic()||access.getReturnType()!=Object.class)throw new AssertionError("erased original accessor ABI");get.setAccessible(true);if(!get.isSynthetic())throw new AssertionError("erased getter ABI");int rows=0;for(Object value:%s){Object pollution=new Object();if(access.invoke(null,owner,pollution)!=pollution||((p.Base)owner).read()!=pollution||get.invoke(null,owner)!=pollution)throw new AssertionError("erased store/result must not narrow");Object got=put.invoke(writer,new Object[]{value});if(got!=value||((p.Base)owner).read()!=value||reader.get()!=value)throw new AssertionError("instantiated writer identity");rows++;}System.out.println(rows+":class-formal:erased:store:source:identity");}}`, row.erased, row.values)
			sources := map[string]string{"p/Base.java": `package p;public class Base<T>{protected T payload;public T read(){return payload;}}`, "q/FieldOwner.java": owner, "q/FieldDriver.java": driver}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/FieldOwner", "q.FieldDriver", "3:class-formal:erased:store:source:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
		})
	}
}
