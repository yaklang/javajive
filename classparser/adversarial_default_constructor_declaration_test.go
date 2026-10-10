package javaclassparser

import (
	"strings"
	"testing"
)

// Body equivalence alone cannot license eliminating an explicit constructor.
// The original, unchanged caller independently reflects its declaration.
func TestAdversarialDefaultConstructorKeepsOriginalDeclaration(t *testing.T) {
	cases := []struct{ name, declaration, check string }{
		{"plain", "public DeclaredCtorMetadata(){}", `if(c.getExceptionTypes().length!=0||c.getTypeParameters().length!=0||c.getAnnotation(CtorMark.class)!=null)throw new AssertionError("plain default declaration");`},
		{"throws", "public DeclaredCtorMetadata()throws java.io.IOException{}", `if(c.getExceptionTypes().length!=1||c.getExceptionTypes()[0]!=java.io.IOException.class)throw new AssertionError("original constructor throws declaration");`},
		{"annotation", `@CtorMark("original") public DeclaredCtorMetadata(){}`, `if(c.getAnnotation(CtorMark.class)==null||!c.getAnnotation(CtorMark.class).value().equals("original"))throw new AssertionError("original constructor annotation");`},
		{"generic", "public <T extends CharSequence> DeclaredCtorMetadata(){}", `if(c.getTypeParameters().length!=1||!c.getTypeParameters()[0].getName().equals("T")||c.getTypeParameters()[0].getBounds().length!=1||c.getTypeParameters()[0].getBounds()[0]!=CharSequence.class)throw new AssertionError("original constructor generic declaration");`},
	}
	for _, root := range []string{"DeclaredCtorMetadata", "RenamedCtorDeclaration"} {
		for _, row := range cases {
			t.Run(root+"/"+row.name, func(t *testing.T) {
				source := `@java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME) @java.lang.annotation.Target(java.lang.annotation.ElementType.CONSTRUCTOR) @interface CtorMark{String value();}public class DeclaredCtorMetadata{` + row.declaration + `}
class ConstructorDeclarationDriver{public static void main(String[]args)throws Exception{java.lang.reflect.Constructor<?>c=DeclaredCtorMetadata.class.getDeclaredConstructor();if(c.getModifiers()!=java.lang.reflect.Modifier.PUBLIC||c.getParameterTypes().length!=0||c.newInstance().getClass()!=DeclaredCtorMetadata.class)throw new AssertionError("original constructor API");` + row.check + `System.out.println("constructor:declaration");}}`
				source = strings.ReplaceAll(source, "DeclaredCtorMetadata", root)
				testNativePrivateSetterSourceFixture(t, map[string]string{root + ".java": source}, root, "ConstructorDeclarationDriver", "constructor:declaration\n")
			})
		}
	}
}
