package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func objectMethodWitnessFixture(shape string) (string, string, string) {
	owner, formals, valueType, inputType := "ObjectWitnessOwner", "B,T extends B", "T", "B"
	valuesSource := `null,"payload",Integer.valueOf(7),new java.lang.Object()`
	classesSource := `null,java.lang.Object.class,String.class,Integer.class,Number.class,int.class`
	rows := 240
	if shape == "chain" {
		formals = "B,U extends B,T extends U"
	}
	if shape == "forward" {
		formals = "T extends B,B"
	}
	if shape == "array" {
		valueType, inputType = "T[]", "B[]"
		valuesSource = `null,new java.lang.Object[0],new String[]{"payload"},new Integer[]{7},new java.lang.Object[]{new java.lang.Object()}`
		classesSource = `null,java.lang.Object[].class,String[].class,Integer[].class,int[].class`
		rows = 250
	}
	physical := "java.lang.Object"
	if shape == "array" {
		physical += "[]"
	}
	source := fmt.Sprintf(`
class WitnessEffects {
 static String trace="";static int fail;static long word;
 static final java.io.IOException failure=new java.io.IOException("original");
 static Class kind(Class c)throws java.io.IOException{trace+="K";if(fail==1)throw failure;return c;}
 static %s value(%s v)throws java.io.IOException{trace+="V";if(fail==2)throw failure;return v;}
 static long wide(long n)throws java.io.IOException{trace+="W";if(fail==3)throw failure;return n;}
}
class %s<A> {
 private static <%s>%s project(Class<%s> kind,%s input,long n)throws java.io.IOException{
  WitnessEffects.trace+="P";if(WitnessEffects.fail==4)throw WitnessEffects.failure;
  WitnessEffects.word=(n*3L)^17L;return kind.cast(input);
 }
 private static <%s>%s project(Class<%s> kind,String%s input,long n){throw new AssertionError("String overload");}
 interface Operation{%s apply(Class kind,%s input,long n)throws java.io.IOException;}
 static class Reader<Object extends Number> implements Operation {
  public %s apply(Class kind,%s input,long n)throws java.io.IOException{
   return project(WitnessEffects.kind(kind),WitnessEffects.value(input),WitnessEffects.wide(n));
  }
 }
 static Operation anonymous(){return new Operation(){public %s apply(Class kind,%s input,long n)throws java.io.IOException{
  return project(WitnessEffects.kind(kind),WitnessEffects.value(input),WitnessEffects.wide(n));
 }};}
}
class WitnessDriver {
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(%s.Operation op:new %s.Operation[]{new %s.Reader<Integer>(),%s.anonymous()})
   for(Class kind:new Class[]{%s})for(%s input:new %s[]{%s})
    for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
     WitnessEffects.fail=0;WitnessEffects.trace="";
     try{
      %s result=op.apply(kind,input,n);
      if(kind==null||input!=null&&!kind.isInstance(input)||result!=input)throw new AssertionError("cast/identity");
     }catch(NullPointerException failure){if(kind!=null)throw new AssertionError("null timing");}
      catch(ClassCastException failure){if(kind==null||input==null||kind.isInstance(input))throw new AssertionError("payload cast");}
     long expected=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(3)).xor(java.math.BigInteger.valueOf(17)).longValue();
     if(WitnessEffects.word!=expected||!WitnessEffects.trace.equals("KVWP"))throw new AssertionError("overflow/order");
     for(int fail:new int[]{1,2,3,4}){
      WitnessEffects.fail=fail;WitnessEffects.trace="";
      try{op.apply(kind,input,n);throw new AssertionError("missing failure");}
      catch(java.io.IOException failure){if(failure!=WitnessEffects.failure)throw new AssertionError("checked identity");}
      if(!WitnessEffects.trace.equals(fail==1?"K":fail==2?"KV":fail==3?"KVW":"KVWP"))throw new AssertionError("short circuit");
     }
     rows++;
    }
  System.out.println(rows+":dependent:bounds:identity:overload:overflow:order");
 }
}`, physical, physical, owner, formals, valueType, valueType, inputType,
		formals, valueType, valueType, map[bool]string{true: "[]", false: ""}[shape == "array"],
		physical, physical, physical, physical, physical, physical,
		owner, owner, owner, owner, classesSource, physical, physical, valuesSource, physical)
	if shape == "renamed" {
		source = strings.NewReplacer(owner, "DistinctWitnessScope", "project", "route", "Reader", "Worker").Replace(source)
		owner = "DistinctWitnessScope"
	}
	return owner, source, fmt.Sprintf("%d:dependent:bounds:identity:overload:overflow:order\n", rows)
}

func TestAdversarialDependentObjectMethodWitnessRoundTrip(t *testing.T) {
	for _, shape := range []string{"dependent", "chain", "forward", "array", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, source, want := objectMethodWitnessFixture(shape)
			for _, native := range []bool{false, true} {
				name := "modern"
				if native {
					name = "native8"
				}
				t.Run(name, func(t *testing.T) {
					javac, _ := t04Tools(t)
					profile := ModernJavac
					if native {
						profile, javac = NativeJavac8, os.Getenv("JAVA8_JAVAC")
					}
					compile := func(debug string) map[string][]byte {
						if native {
							return nativePrivateEnumCompile(t, source, owner, debug)
						}
						return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
					}
					testNativeIndependentCompilerFamilyFixture(t, compile, profile, javac, []string{owner}, "WitnessDriver", want, nil, nativeLexicalExactSignatures, objectWitnessNoAddedNarrowingCast)
				})
			}
		})
	}
}

func objectWitnessNoAddedNarrowingCast(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	if !strings.HasSuffix(name, "$Reader.class") && !strings.HasSuffix(name, "$Worker.class") && !strings.HasSuffix(name, "$1.class") {
		return
	}
	for _, raw := range [][]byte{original, rebuilt} {
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, m.NameIndex)
			if n != "apply" {
				continue
			}
			for _, a := range m.Attributes {
				if code, ok := a.(*CodeAttribute); ok {
					d := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if err := d.ParseOpcode(); err != nil {
						t.Fatal(err)
					}
					ops := constructorMotionOps(d)
					for i, op := range ops {
						if op.Instr.OpCode == core.OP_CHECKCAST {
							// javac8 retains an array cast even for an explicit cast
							// to this producer's identical JVM return type. Prove
							// identity from physical instructions, not generic erasure
							// or the renderer's expression type: such a cast cannot
							// reject any value the verified producer can return.
							if i == 0 || len(op.Data) != 2 {
								t.Fatal("unproved payload cast", name)
							}
							previous := ops[i-1]
							member := constructorMotionMember(obj, previous, previous.Instr.OpCode)
							if member == nil || previous.Instr.OpCode != core.OP_INVOKESTATIC && previous.Instr.OpCode != core.OP_INVOKEVIRTUAL && previous.Instr.OpCode != core.OP_INVOKEINTERFACE {
								t.Fatal("cast has no exact invocation result", name)
							}
							_, result, err := callbinding.Descriptor(member.Description)
							target, valid := sourceBridgeClassName(obj, uint16(core.Convert2bytesToInt(op.Data)))
							if !strings.HasPrefix(target, "[") {
								target = "L" + target + ";"
							}
							if err != nil || !valid || result != target {
								t.Fatal("explicit Object witnesses introduced a narrowing check", name, result, target)
							}
						}
					}
				}
			}
		}
	}
}
