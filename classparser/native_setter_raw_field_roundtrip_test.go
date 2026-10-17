package javaclassparser

import (
	"fmt"
	"testing"
)

func TestAdversarialRawInheritedFieldWrite(t *testing.T) {
	for _, row := range []struct{ name, formals, super, typ, erased, values string }{
		{"formal", "<E>", "E", "E", "Object", `new Object[]{null,new Object(),new StringBuilder("identity")}`},
		{"bounded", "<E extends Number>", "E", "E", "Number", `new Object[]{null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE)}`},
		{"array", "<E>", "E[]", "E[]", "Object[]", `new Object[]{null,new Object[0],new Object[]{new Object()}}`},
	} {
		t.Run(row.name, func(t *testing.T) {
			owner := "package q;public class FieldOwner" + row.formals + " extends p.Base<" + row.super + ">{public class Writer{public " + row.typ + " put(" + row.typ + " next){return payload=next;}}}"
			driver := fmt.Sprintf(`package q;public class FieldDriver{public static void main(String[]args)throws Exception{FieldOwner owner=new FieldOwner();FieldOwner.Writer writer=owner.new Writer();if(FieldOwner.Writer.class.getDeclaringClass()!=FieldOwner.class)throw new AssertionError("source binding");java.lang.reflect.Method put=FieldOwner.Writer.class.getDeclaredMethod("put",%s.class);java.lang.reflect.Method access=FieldOwner.class.getDeclaredMethod("access$002",FieldOwner.class,Object.class);access.setAccessible(true);if(!access.isSynthetic()||access.getReturnType()!=Object.class)throw new AssertionError("erased original accessor ABI");int rows=0;for(Object value:%s){Object pollution=new Object();if(access.invoke(null,owner,pollution)!=pollution||((p.Base)owner).read()!=pollution)throw new AssertionError("erased store/result must not narrow");Object got=put.invoke(writer,new Object[]{value});if(got!=value||((p.Base)owner).read()!=value)throw new AssertionError("instantiated writer identity");rows++;}System.out.println(rows+":class-formal:erased:store:source:identity");}}`, row.erased, row.values)
			sources := map[string]string{"p/Base.java": `package p;public class Base<T>{protected T payload;public T read(){return payload;}}`, "q/FieldOwner.java": owner, "q/FieldDriver.java": driver}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/FieldOwner", "q.FieldDriver", "3:class-formal:erased:store:source:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
		})
	}
}
