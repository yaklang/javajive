package javaclassparser

import (
	"fmt"
	"testing"
)

func TestAdversarialRawInheritedFinalFieldRead(t *testing.T) {
	for _, row := range []struct {
		name, formals, super, typ, values string
		pollutionCast                     bool
	}{
		{"formal", "<E>", "E", "E", `new Object[]{null,new Object(),new StringBuilder("identity")}`, false},
		{"bounded", "<E extends Number>", "E", "E", `new Object[]{null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE)}`, true},
		{"array", "<E>", "E[]", "E[]", `new Object[]{null,new Object[0],new Object[]{new Object()}}`, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			owner := "package q;public class FieldOwner" + row.formals + " extends p.Base<" + row.super + ">{public class Reader{public " + row.typ + " get(){return payload;}}}"
			driver := fmt.Sprintf(`package q;public class FieldDriver{public static void main(String[]args)throws Exception{FieldOwner owner=new FieldOwner();FieldOwner.Reader reader=owner.new Reader();if(FieldOwner.Reader.class.getDeclaringClass()!=FieldOwner.class)throw new AssertionError("source binding");java.lang.reflect.Field field=p.Base.class.getDeclaredField("payload");field.setAccessible(true);if(!java.lang.reflect.Modifier.isFinal(field.getModifiers()))throw new AssertionError("final original storage");java.lang.reflect.Method access=FieldOwner.class.getDeclaredMethod("access$000",FieldOwner.class);access.setAccessible(true);if(!access.isSynthetic()||access.getReturnType()!=Object.class)throw new AssertionError("erased original getter ABI");java.lang.reflect.Method get=FieldOwner.Reader.class.getDeclaredMethod("get");Object pollution=new Object();field.set(owner,pollution);if(access.invoke(null,owner)!=pollution)throw new AssertionError("raw read must not narrow");boolean cast=false;try{Object value=get.invoke(reader);if(value!=pollution)throw new AssertionError("raw identity");}catch(java.lang.reflect.InvocationTargetException ex){if(!(ex.getCause() instanceof ClassCastException))throw ex;cast=true;}if(cast!=%t||field.get(owner)!=pollution)throw new AssertionError("original typed consumer cast/no write");try{access.invoke(null,new Object[]{null});throw new AssertionError("null receiver accepted");}catch(java.lang.reflect.InvocationTargetException ex){if(!(ex.getCause() instanceof NullPointerException))throw ex;}int rows=0;for(Object value:%s){field.set(owner,value);if(access.invoke(null,owner)!=value||get.invoke(reader)!=value||field.get(owner)!=value)throw new AssertionError("final field identity");rows++;}System.out.println(rows+":final:raw:typed:cast:identity");}}`, row.pollutionCast, row.values)
			sources := map[string]string{"p/Base.java": `package p;public class Base<T>{protected final T payload;public Base(){payload=null;}}`, "q/FieldOwner.java": owner, "q/FieldDriver.java": driver}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "q/FieldOwner", "q.FieldDriver", "3:final:raw:typed:cast:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"})
		})
	}
}
