package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The physical classfile owner uses slashes and the expression IR uses dots.
// Both must denote the same original class at a proved unqualified lexical
// call. The retained binary driver also checks anonymous/enclosing identity,
// delayed field reads, distinct allocations, and the original call exception.
const packagedLexicalReceiverFixture = `
interface LexicalView {LexicalRead nested();void replace(Object value);}
interface LexicalRead {Object read();Object owner();}
class LexicalEffects {
 static final RuntimeException error=new IllegalStateException("original");
 static int calls;
 static boolean fail;
}
class LexicalOwner {
 LexicalView make(final Object token) {
  return new LexicalView() {
   Object current=token;
   Object identity() {
    LexicalEffects.calls++;
    if(LexicalEffects.fail)throw LexicalEffects.error;
    return this;
   }
   public void replace(Object value) {current=value;}
   public LexicalRead nested() {
    return new LexicalRead() {
     public Object read() {return current;}
     public Object owner() {return identity();}
    };
   }
  };
 }
}
class LexicalDriver {
 static void checkOwner(LexicalRead read,LexicalView original,boolean fail) {
  int before=LexicalEffects.calls;
  try {
   Object got=read.owner();
   if(fail||got!=original)throw new AssertionError("lexical receiver identity");
  }catch(RuntimeException error) {
   if(!fail||error!=LexicalEffects.error)throw new AssertionError("original call exception",error);
  }
  if(LexicalEffects.calls!=before+1)throw new AssertionError("call once");
 }
 public static void main(String[] args) {
  Object shared=new Object();int rows=0;
  for(Object token:new Object[]{null,shared,new String("token")})for(Object next:new Object[]{null,shared,new String("next")}) {
   LexicalView outer=new LexicalOwner().make(token);
   LexicalRead first=outer.nested(),second=outer.nested();
   if(first==second||!first.getClass().isAnonymousClass()||first.getClass().getEnclosingClass()!=outer.getClass()||!first.getClass().getEnclosingMethod().getName().equals("nested"))throw new AssertionError("original lexical/anonymous scope");
   if(first.read()!=token||second.read()!=token)throw new AssertionError("initial identity");
   outer.replace(next);
   if(first.read()!=next||second.read()!=next)throw new AssertionError("delayed field identity");
   for(boolean fail:new boolean[]{false,true}) {
    LexicalEffects.fail=fail;
    checkOwner(first,outer,fail);checkOwner(second,outer,fail);rows++;
   }
  }
  System.out.println(rows+":lexical:receiver:identity:delayed:exceptions");
 }
}`

func TestAdversarialPackagedLexicalReceiverPreservesOriginalBinding(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, pkg := range []string{"", "lexical.binding"} {
			namespace := pkg
			if namespace == "" {
				namespace = "unnamed"
			}
			for _, simple := range []string{"LexicalOwner", "DifferentLexicalOwner"} {
				t.Run(compiler.name+"/"+namespace+"/"+simple, func(t *testing.T) {
					source := strings.ReplaceAll(packagedLexicalReceiverFixture, "LexicalOwner", simple)
					owner, driver := simple, "LexicalDriver"
					if pkg != "" {
						source = "package " + pkg + ";\n" + source
						owner = strings.ReplaceAll(pkg, ".", "/") + "/" + simple
						driver = pkg + "." + driver
					}
					compile := func(debug string) map[string][]byte {
						if compiler.profile == NativeJavac8 {
							return localGetterCompileJavac8(t, map[string]string{simple + ".java": source}, debug)
						}
						return nativeCompileDebugClasses(t, source, debug)
					}
					testNativeIndependentCompilerFamilyFixture(t, compile, compiler.profile, compiler.javac, []string{owner}, driver, "18:lexical:receiver:identity:delayed:exceptions\n", nil, nativeLexicalExactSignatures)
				})
			}
		}
	}
}
