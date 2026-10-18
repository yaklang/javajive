package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Both callers remain binary. The enclosing receiver has no source name, while
// every ordinary constructor argument must retain its identity, evaluation
// order and abrupt completion. Equal erasures and a competing String overload
// make dropping or guessing an argument observable independently of recompiling.
const anonymousMemberArgumentsFixture = `interface MemberArgsAction{Object get();}
class MemberArgsEffects{
 static int events,calls,wrong,fault;static Object published;
 static final RuntimeException failure=new IllegalArgumentException("same exception");
 static Object ref(int step,Object value){events=events*10+step;if(fault==step)throw failure;return value;}
 static long wide(long value){events=events*10+2;if(fault==2)throw failure;return value^0x5a5a5a5a5a5a5a5aL;}
}
abstract class MemberArgsRoot{
 abstract Object identity();abstract MemberArgsAction make(Object first,long salt,Object last);
 class Entry{
  final MemberArgsRoot root;final Object first,last,token;final long word;
  Entry(Object first,long word,Object last){root=MemberArgsRoot.this;this.first=first;this.word=word;this.last=last;token=root.identity();MemberArgsEffects.events=MemberArgsEffects.events*10+4;MemberArgsEffects.calls++;MemberArgsEffects.published=this;if(MemberArgsEffects.fault==4)throw MemberArgsEffects.failure;}
 }
 static MemberArgsRoot create(final Object token){return new MemberArgsRoot(){
  Object identity(){return token;}
  class Element extends Entry{
   Element(Object first,long word,Object last){super(first,word,last);}
   Element(String first,long word,String last){super(first,word,last);MemberArgsEffects.wrong++;}
  }
  MemberArgsAction make(final Object first,final long salt,final Object last){return new MemberArgsAction(){public Object get(){return new Element(MemberArgsEffects.ref(1,first),MemberArgsEffects.wide(salt),MemberArgsEffects.ref(3,last));}};}
 };}
}
class MemberArgsDriver{
 public static void main(String[]args)throws Exception{
  boolean producers=true,nested=false;int rows=0;Object shared=new Object();
  for(Object token:new Object[]{null,shared})for(Object first:new Object[]{null,shared,new String("equal")})for(Object last:new Object[]{null,shared,new String("equal")})for(long salt:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
   MemberArgsRoot namespace=MemberArgsRoot.create(token);MemberArgsAction action=namespace.make(first,salt,last);
   long expected=producers?java.math.BigInteger.valueOf(salt).xor(new java.math.BigInteger("5a5a5a5a5a5a5a5a",16)).longValue():salt;
   for(int fault:producers?new int[]{0,1,2,3,4}:new int[]{0,4}){
    Object previous=null;
    for(int repeat=0;repeat<2;repeat++){
     MemberArgsEffects.events=0;MemberArgsEffects.calls=0;MemberArgsEffects.wrong=0;MemberArgsEffects.published=null;MemberArgsEffects.fault=fault;
     Object returned=null;
     try{returned=action.get();if(fault!=0)throw new AssertionError("missing original failure");}
     catch(RuntimeException failure){if(fault==0||failure!=MemberArgsEffects.failure)throw new AssertionError("exception identity",failure);}
     boolean allocated=fault==0||fault==4;int events=producers?(fault==1?1:fault==2?12:fault==3?123:1234):nested&&fault==0?44:4;
     int calls=allocated?(nested&&fault==0?2:1):0;
     if(MemberArgsEffects.events!=events||MemberArgsEffects.calls!=calls||MemberArgsEffects.wrong!=0)throw new AssertionError("argument order/constructor overload");
     if(!allocated){if(MemberArgsEffects.published!=null)throw new AssertionError("allocation after failed producer");}
     else{
      MemberArgsRoot.Entry entry=(MemberArgsRoot.Entry)MemberArgsEffects.published;
      if(entry==null||entry==previous||entry.root!=namespace||entry.last!=last||entry.word!=expected||entry.token!=token||(fault==0&&returned!=entry))throw new AssertionError("capture/argument identity/publication/arithmetic");
      if(nested&&fault==0){MemberArgsRoot.Entry inner=(MemberArgsRoot.Entry)entry.first;if(inner==entry||inner.first!=first||inner.last!=last||inner.word!=expected||inner.root!=namespace||inner.token!=token)throw new AssertionError("same-owner nested NEW identity");}
      else if(entry.first!=first)throw new AssertionError("first argument identity");
      if(entry.getClass().getDeclaringClass()!=namespace.getClass()||!entry.getClass().isMemberClass()||!entry.getClass().getSimpleName().equals("Element"))throw new AssertionError("original member binding");
      previous=entry;
     }
     rows++;
    }
   }
  }
  System.out.println(rows+":anonymous:member:arguments:identity:effects");
 }
}`

func TestAdversarialAnonymousMemberArgumentsPreserveLexicalReceiver(t *testing.T) {
	modern, _ := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("actual javac8 required")
	}
	for _, compiler := range []struct {
		name, javac string
		profile     SourceCompilerProfile
	}{{"native8", native, NativeJavac8}, {"modern8", modern, ModernJavac}} {
		for _, owner := range []string{"MemberArgsRoot", "RenamedMemberScope"} {
			for _, layout := range []string{"parameters", "effectful", "nested", "long-arguments"} {
				t.Run(compiler.name+"/"+owner+"/"+layout, func(t *testing.T) {
					source := strings.ReplaceAll(anonymousMemberArgumentsFixture, "MemberArgsRoot", owner)
					want := "900:anonymous:member:arguments:identity:effects\n"
					if layout != "effectful" {
						source = strings.Replace(source, "MemberArgsEffects.ref(1,first),MemberArgsEffects.wide(salt),MemberArgsEffects.ref(3,last)", "first,salt,last", 1)
						source = strings.Replace(source, "boolean producers=true", "boolean producers=false", 1)
						want = "360:anonymous:member:arguments:identity:effects\n"
					}
					if layout == "nested" {
						source = strings.Replace(source, "new Element(first,salt,last)", "new Element(new Element(first,salt,last),salt,last)", 1)
						source = strings.Replace(source, "nested=false", "nested=true", 1)
					}
					if layout == "long-arguments" {
						// Each mask appears an even number of times. XOR's inverse
						// law supplies an oracle independent of this long evaluation
						// chain, including the sign bit and category-2 local slots.
						body := "long carry=salt;" + strings.Repeat("carry^=0x8000000000000001L;carry^=0x400000001L;", 48)
						source = strings.Replace(source, "return new Element(first,salt,last)", body+"return new Element(first,carry,last)", 1)
					}
					compile := func(debug string) map[string][]byte {
						var files map[string][]byte
						if compiler.profile == NativeJavac8 {
							files = nativePrivateEnumCompile(t, source, owner, debug)
						} else {
							files = nativeCompileDebugClasses(t, source, debug)
						}
						if layout == "long-arguments" {
							child, err := Parse(files[owner+"$1$1.class"])
							if err != nil {
								t.Fatal(err)
							}
							long := false
							for _, method := range child.Methods {
								name, _ := sourceBridgeUTF8(child, method.NameIndex)
								for _, attribute := range method.Attributes {
									if code, ok := attribute.(*CodeAttribute); ok && name == "get" && len(code.Code) > 512 {
										long = true
									}
								}
							}
							if !long {
								t.Fatal("original argument evaluation did not cross the old packet cutoff")
							}
						}
						return files
					}
					testNativeIndependentCompilerFamilyFixture(t, compile, compiler.profile, compiler.javac, []string{owner}, "MemberArgsDriver", want, nil, nativeLexicalExactSignatures)
				})
			}
		}
	}
}
