package javaclassparser

import (
	"strings"
	"testing"
)

func publicAbstractRootBridgeFixture(kind string) string {
	fixture := strings.Replace(nativeRootPrivateConstructorFixture, "class RootBridgePacket {", "public abstract class RootBridgePacket {", 1)
	constructorTypes := "Object.class"
	extraCheck := ""
	switch kind {
	case "generic":
		fixture = strings.Replace(fixture, "class RootBridgePacket {", "class RootBridgePacket<T> {", 1)
		fixture = strings.Replace(fixture, "Member extends RootBridgePacket", "Member extends RootBridgePacket<Object>", 1)
		extraCheck = `if(RootBridgePacket.class.getTypeParameters().length!=1||RootBridgePacket.class.getTypeParameters()[0].getBounds()[0]!=Object.class)throw new AssertionError("generic declaration");`
	case "wide":
		fixture = strings.Replace(fixture, "final Object value;private RootBridgePacket(Object value)", "final long word;final Object value;private RootBridgePacket(long word,Object value)", 1)
		fixture = strings.Replace(fixture, "this.value=value;", "this.value=value;this.word=word;", 1)
		fixture = strings.Replace(fixture, "super(value);", "super(value==null?Long.MIN_VALUE:Long.MAX_VALUE,value);", 1)
		fixture = strings.Replace(fixture, "if(p.value!=v", "if(p.word!=(v==null?Long.MIN_VALUE:Long.MAX_VALUE)||p.value!=v", 1)
		constructorTypes = "long.class,Object.class"
	case "checked":
		fixture = strings.Replace(fixture, "private RootBridgePacket(Object value){", "private RootBridgePacket(Object value)throws java.io.IOException{", 1)
		fixture = strings.Replace(fixture, "Member(Object value){", "Member(Object value)throws java.io.IOException{", 1)
		fixture = strings.Replace(fixture, "make(Object value){", "make(Object value)throws java.io.IOException{", 1)
		extraCheck = `if(RootBridgePacket.class.getDeclaredConstructor(Object.class).getExceptionTypes().length!=1||RootBridgePacket.class.getDeclaredConstructor(Object.class).getExceptionTypes()[0]!=java.io.IOException.class)throw new AssertionError("checked declaration");`
	}
	declarationCheck := "Object token=new Object();if(!java.lang.reflect.Modifier.isAbstract(RootBridgePacket.class.getModifiers())||!java.lang.reflect.Modifier.isPublic(RootBridgePacket.class.getModifiers())||!java.lang.reflect.Modifier.isPrivate(RootBridgePacket.class.getDeclaredConstructor(" + constructorTypes + ").getModifiers()))throw new AssertionError(\"public abstract/private declaration\");" + extraCheck + "int rows=0;"
	fixture = strings.Replace(fixture, "Object token=new Object();int rows=0;", declarationCheck, 1)
	return strings.Replace(fixture, "public static void main(String[]args){", "public static void main(String[]args)throws Exception{", 1)
}

func TestAdversarialPublicAbstractRootKeepsPrivateConstructorBridgeRoundTrip(t *testing.T) {
	for _, kind := range []string{"reference", "generic", "wide", "checked"} {
		t.Run(kind, func(t *testing.T) {
			testSourceTargetReleaseFamilyFixture(t, publicAbstractRootBridgeFixture(kind), "RootBridgePacket", "RootBridgeDriver", "2:private-root:identity:order:owner\n", "8", []int{8})
		})
	}
}
