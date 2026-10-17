package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// An update is both a local read and a write. Its target must remain connected
// to the original load's enumerable binding through declaration/slot renaming.
func TestNativeTypedUpdatesRetainLocalDeclarationAndBinding(t *testing.T) {
	var methods, oracle, checks strings.Builder
	rows := 0
	for _, typ := range nativeTypedPostCases {
		n := typ.name
		methods.WriteString(strings.ReplaceAll(`TYPE identityTYPE(TYPE x){return x;}TYPE[] runTYPE(boolean branch,TYPE seed){if(branch){Object[] scratch=new Object[]{"reused slot"};if(scratch.length!=1)throw new AssertionError();return new TYPE[]{seed,seed,seed};}else{TYPE cursor=seed;TYPE first=identityTYPE(cursor++);TYPE second=identityTYPE(cursor++);return new TYPE[]{first,second,cursor};}}TYPE[] prefixTYPE(TYPE seed){TYPE cursor=seed;TYPE first=identityTYPE(++cursor);TYPE second=identityTYPE(++cursor);return new TYPE[]{first,second,cursor};}`, "TYPE", n))
		oracle.WriteString(strings.NewReplacer("TYPE", n, "NEXT", typ.next, "EQUAL", typ.equal).Replace(`static TYPE nextTYPE(TYPE x,int delta){return NEXT;}static boolean eqTYPE(TYPE got,TYPE next){return EQUAL;}`))
		checks.WriteString(strings.NewReplacer("TYPE", n, "INPUTS", typ.inputs).Replace(`for(TYPE x:new TYPE[]{INPUTS}){TYPE next=nextTYPE(x,1),twice=nextTYPE(next,1);TYPE[] a=c.runTYPE(false,x);if(a.length!=3||!eqTYPE(a[0],x)||!eqTYPE(a[1],next)||!eqTYPE(a[2],twice))throw new AssertionError("local target/binding");a=c.runTYPE(true,x);if(!eqTYPE(a[0],x)||!eqTYPE(a[1],x)||!eqTYPE(a[2],x))throw new AssertionError("other binding");a=c.prefixTYPE(x);if(a.length!=3||!eqTYPE(a[0],next)||!eqTYPE(a[1],twice)||!eqTYPE(a[2],twice))throw new AssertionError("prefix local target/binding");rows++;}`))
		rows += strings.Count(typ.inputs, ",") + 1
	}
	methods.WriteString(`long read(long index){return index;}long loop(boolean invalid,long seed,int count){if(invalid){Object[] scratch=new Object[]{seed,count};if(scratch.length!=2)throw new AssertionError();return seed;}else{long cursor=seed;long end=cursor+count;long total=0;while(cursor<end){long first=read(cursor);if(first%3!=0){cursor++;total+=first;}else{break;}}OUT:while(cursor<end){long first=read(cursor++);if(first%2==0){total+=first;while(cursor<end){long second=read(cursor);if(second%2!=0)continue OUT;cursor++;total+=second;}}else{total+=first;if(cursor<end){total+=read(cursor++);continue;}}}return total;}}`)
	checks.WriteString(`for(long seed=-3;seed<4;seed++){for(int count=0;count<7;count++){long expected=0;for(int i=0;i<count;i++)expected+=seed+i;if(c.loop(false,seed,count)!=expected||c.loop(true,seed,count)!=seed)throw new AssertionError("loop update binding");rows++;}}`)
	rows += 49
	source := fmt.Sprintf(`class LocalUpdateOwner {class Child{%s}Child make(){return new Child();}}class LocalUpdateDriver{%s public static void main(String[]args){LocalUpdateOwner.Child c=new LocalUpdateOwner().make();int rows=0;%s System.out.println("local:binding:"+rows);}}`, methods.String(), oracle.String(), checks.String())
	want := fmt.Sprintf("local:binding:%d\n", rows)
	for _, native := range []bool{false, true} {
		if native && os.Getenv("JAVA8_JAVAC") == "" {
			t.Fatal("JAVA8_JAVAC required")
		}
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			if native {
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
					return localGetterCompileJavac8(t, map[string]string{"LocalUpdateOwner.java": source}, debug)
				}, NativeJavac8, os.Getenv("JAVA8_JAVAC"), []string{"LocalUpdateOwner"}, "LocalUpdateDriver", want, nil, nativeLexicalExactSignatures)
			} else {
				testNativePrivateSetterCompiledFixture(t, "LocalUpdateOwner", "LocalUpdateDriver", want, func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, nativeLexicalExactSignatures)
			}
		})
	}
}
