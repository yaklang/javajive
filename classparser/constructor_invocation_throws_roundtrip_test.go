package javaclassparser

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const constructorInvocationThrowsFixture = `
interface CtorThrowMarker {}
class CtorThrowMarkerFailure extends java.io.IOException implements CtorThrowMarker {}
class CtorThrowState {
 static String trace;
 static final java.io.IOException io=new java.io.IOException("same-io");
 static final java.io.IOException other=new java.io.IOException("other-io");
 static final RuntimeException runtime=new RuntimeException("same-runtime");
 static <T> T opaque(T value){return value;}
 static final java.io.FileNotFoundException file=new java.io.FileNotFoundException("same-file");
 static final CtorThrowMarkerFailure marker=new CtorThrowMarkerFailure();
 @SuppressWarnings("unchecked") static <T extends Throwable> void effect(boolean fail,Throwable value)throws T{
  trace+="B";if(fail)throw(T)value;trace+="E";
 }
}
class CtorThrowLooseBase {<E extends Throwable>CtorThrowLooseBase(boolean fail,Throwable value)throws E{CtorThrowState.<E>effect(fail,value);}}
class CtorThrowLoose extends CtorThrowLooseBase {CtorThrowLoose(boolean fail,Throwable value){super(fail,value);}}
class CtorThrowExceptionBase {<E extends Exception>CtorThrowExceptionBase(boolean fail,Exception value)throws E{CtorThrowState.<E>effect(fail,value);}}
class CtorThrowException extends CtorThrowExceptionBase {CtorThrowException(boolean fail,Exception value){super(fail,value);}}
class CtorThrowArgumentBase {<E extends Throwable>CtorThrowArgumentBase(boolean fail,E value)throws E{CtorThrowState.<E>effect(fail,value);}}
class CtorThrowArgument extends CtorThrowArgumentBase {CtorThrowArgument(boolean fail,java.io.IOException value)throws java.io.IOException{super(fail,value);}}
class CtorThrowNarrow extends CtorThrowArgumentBase {CtorThrowNarrow(boolean fail,java.io.FileNotFoundException value)throws java.io.IOException{super(fail,value);}}
class CtorThrowPairBase {<E extends Throwable>CtorThrowPairBase(boolean fail,E left,E right)throws E{CtorThrowState.<E>effect(fail,left);}}
class CtorThrowPair extends CtorThrowPairBase {CtorThrowPair(boolean fail,java.io.IOException left,java.io.FileNotFoundException right)throws java.io.IOException{super(fail,left,right);}}
class CtorThrowIntersectionBase {<E extends Exception & CtorThrowMarker>CtorThrowIntersectionBase(boolean fail,E value)throws E{CtorThrowState.<E>effect(fail,value);}}
class CtorThrowIntersection extends CtorThrowIntersectionBase {CtorThrowIntersection(boolean fail,CtorThrowMarkerFailure value)throws java.io.IOException{super(fail,value);}}
class CtorThrowShadowBase<E extends java.io.IOException> {<E extends Throwable>CtorThrowShadowBase(boolean fail,Throwable value)throws E{CtorThrowState.<E>effect(fail,value);}}
class CtorThrowShadow extends CtorThrowShadowBase<java.io.IOException> {CtorThrowShadow(boolean fail,Throwable value){super(fail,value);}}
class CtorThrowOverloadBase {
 <E extends Throwable>CtorThrowOverloadBase(boolean fail,Throwable value)throws E{CtorThrowState.<E>effect(fail,value);}
 CtorThrowOverloadBase(int tag,Throwable value)throws java.io.IOException{CtorThrowState.trace+="R";throw CtorThrowState.io;}
 CtorThrowOverloadBase(Object tag,Throwable value,int extra)throws java.io.IOException{CtorThrowState.trace+="Q";throw CtorThrowState.io;}
}
class CtorThrowOverload extends CtorThrowOverloadBase {CtorThrowOverload(boolean fail,Throwable value){super(fail,value);}}
class CtorThrowCheckedBase {<E extends java.io.IOException>CtorThrowCheckedBase(boolean fail,Throwable value)throws E{CtorThrowState.<E>effect(fail,value);}}
class CtorThrowChecked extends CtorThrowCheckedBase {CtorThrowChecked(boolean fail,Throwable value)throws java.io.IOException{super(fail,value);}}
class CtorThrowOrdinaryBase {CtorThrowOrdinaryBase(boolean fail,Throwable value)throws java.io.IOException{CtorThrowState.<java.io.IOException>effect(fail,value);}}
class CtorThrowOrdinary extends CtorThrowOrdinaryBase {CtorThrowOrdinary(boolean fail,Throwable value)throws java.io.IOException{super(fail,value);}}
class CtorThrowMethodShadow<E extends java.io.IOException> extends CtorThrowLooseBase {<E extends Throwable>CtorThrowMethodShadow(boolean fail,Throwable value){super(fail,value);}}
class CtorThrowCast extends CtorThrowArgumentBase {CtorThrowCast(boolean fail,java.io.IOException value)throws Throwable{super(fail,(Throwable)value);}}
class CtorThrowConditional extends CtorThrowArgumentBase {CtorThrowConditional(boolean fail,boolean pick,java.io.IOException left,java.io.IOException right)throws Throwable{super(fail,pick?left:right);}}
class CtorThrowOpaqueBroad extends CtorThrowArgumentBase {CtorThrowOpaqueBroad(boolean fail,Throwable value)throws Throwable{super(fail,CtorThrowState.opaque(value));}}
class CtorThrowMixedIO extends CtorThrowArgumentBase {<X extends Exception>CtorThrowMixedIO(boolean fail,java.io.IOException value)throws X,java.io.IOException{super(fail,value);}}
class CtorThrowMixedOrdinary extends CtorThrowOrdinaryBase {<X extends Exception>CtorThrowMixedOrdinary(boolean fail,java.io.IOException value)throws X,java.io.IOException{super(fail,value);}}
class CtorThrowExceptionArgumentBase {<E extends Exception>CtorThrowExceptionArgumentBase(boolean fail,E value)throws E{CtorThrowState.<E>effect(fail,value);}}
class CtorThrowOpaqueException extends CtorThrowExceptionArgumentBase {<X extends java.io.FileNotFoundException>CtorThrowOpaqueException(boolean fail,java.io.IOException value)throws X,Exception{super(fail,CtorThrowState.opaque(value));}}
class CtorThrowRivalBase {<E extends Exception>CtorThrowRivalBase(boolean fail,E value)throws E{CtorThrowState.<E>effect(fail,value);}CtorThrowRivalBase(boolean fail,Object value)throws java.sql.SQLException{CtorThrowState.trace+="R";throw new java.sql.SQLException();}}
class CtorThrowRivalMixed extends CtorThrowRivalBase {<X extends java.io.FileNotFoundException>CtorThrowRivalMixed(boolean fail,java.io.IOException value)throws X,Exception{super(fail,value);}}
class CtorThrowOpaquePlain extends CtorThrowExceptionArgumentBase {<X extends Object>CtorThrowOpaquePlain(boolean fail,java.io.IOException value)throws Exception{super(fail,CtorThrowState.opaque(value));}}
class CtorThrowDriver {
 public static void main(String[] args)throws Exception{
  Class<?>[] types={CtorThrowLoose.class,CtorThrowException.class,CtorThrowArgument.class,CtorThrowNarrow.class,CtorThrowPair.class,CtorThrowIntersection.class,CtorThrowShadow.class,CtorThrowOverload.class,CtorThrowChecked.class,CtorThrowOrdinary.class,CtorThrowMethodShadow.class,CtorThrowCast.class,CtorThrowConditional.class,CtorThrowOpaqueBroad.class,CtorThrowMixedIO.class,CtorThrowMixedOrdinary.class,CtorThrowOpaqueException.class,CtorThrowRivalMixed.class,CtorThrowOpaquePlain.class};
  int rows=0;
  for(int kind=0;kind<types.length;kind++)for(int scenario=0;scenario<((kind==0||kind==1||kind==6||kind==7||kind==10||kind==13)?3:2);scenario++){
   boolean fail=scenario!=0;
   CtorThrowState.trace="";Throwable expected=scenario==2?CtorThrowState.runtime:kind==3?CtorThrowState.file:kind==5?CtorThrowState.marker:kind==12&&fail?CtorThrowState.other:CtorThrowState.io;
   try{
    switch(kind){
     case 0:new CtorThrowLoose(fail,expected);break;
     case 1:new CtorThrowException(fail,(Exception)expected);break;
     case 2:new CtorThrowArgument(fail,(java.io.IOException)expected);break;
     case 3:new CtorThrowNarrow(fail,(java.io.FileNotFoundException)expected);break;
     case 4:new CtorThrowPair(fail,(java.io.IOException)expected,CtorThrowState.file);break;
     case 5:new CtorThrowIntersection(fail,(CtorThrowMarkerFailure)expected);break;
     case 6:new CtorThrowShadow(fail,expected);break;
     case 7:new CtorThrowOverload(fail,expected);break;
     case 8:new CtorThrowChecked(fail,expected);break;
     case 9:new CtorThrowOrdinary(fail,expected);break;
     case 10:new CtorThrowMethodShadow<java.io.IOException>(fail,expected);break;
     case 11:new CtorThrowCast(fail,(java.io.IOException)expected);break;
     case 12:new CtorThrowConditional(fail,!fail,CtorThrowState.io,CtorThrowState.other);break;
     case 13:new CtorThrowOpaqueBroad(fail,expected);break;
     case 14:new CtorThrowMixedIO(fail,(java.io.IOException)expected);break;
     case 15:new CtorThrowMixedOrdinary(fail,(java.io.IOException)expected);break;
     case 16:new CtorThrowOpaqueException(fail,(java.io.IOException)expected);break;
     case 17:new CtorThrowRivalMixed(fail,(java.io.IOException)expected);break;
     case 18:new CtorThrowOpaquePlain(fail,(java.io.IOException)expected);break;
    }
    if(fail||!CtorThrowState.trace.equals("BE"))throw new AssertionError("normal/order/overload");
   }catch(Throwable failure){if(!fail||failure!=expected||!CtorThrowState.trace.equals("B"))throw new AssertionError("failure identity/order/overload",failure);}
   java.lang.reflect.Constructor<?>[] ctors=types[kind].getDeclaredConstructors();
   Class<?>[] exceptions=ctors[0].getExceptionTypes();
   boolean checked=kind==2||kind==3||kind==4||kind==5||kind==8||kind==9;
   boolean broad=kind==11||kind==12||kind==13;
   if(kind>=14&&kind<=17){Class<?> first=kind<=15?Exception.class:java.io.FileNotFoundException.class,second=kind<=15?java.io.IOException.class:Exception.class;if(ctors.length!=1||exceptions.length!=2||exceptions[0]!=first||exceptions[1]!=second||!(ctors[0].getGenericExceptionTypes()[0] instanceof java.lang.reflect.TypeVariable))throw new AssertionError("mixed formal/concrete ABI");}
   else if(kind==18){if(ctors.length!=1||exceptions.length!=1||exceptions[0]!=Exception.class||ctors[0].getGenericExceptionTypes()[0]!=Exception.class)throw new AssertionError("concrete throws omitted from Signature");}
   else if(ctors.length!=1||exceptions.length!=((checked||broad)?1:0)||checked&&exceptions[0]!=java.io.IOException.class||broad&&exceptions[0]!=Throwable.class)throw new AssertionError("original constructor Exceptions ABI "+types[kind].getName()+":"+java.util.Arrays.toString(exceptions));
   rows++;
  }
  System.out.println(rows+":generic-ctor:same:order:overload:abi");
 }
}`

// Keep declaration metadata independent of the production inference result.
// Exceptions attribute absence, order, Signature and every original flag are ABI.
func constructorInvocationThrowsDeclarations(t *testing.T, raw []byte) string {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	type declaration struct {
		Kind, Name, Descriptor string
		Flags                  uint16
		Signatures             []string
		Exceptions             [][]string
	}
	rows := []declaration{{Kind: "class", Name: obj.GetClassName(), Descriptor: obj.GetSupperClassName(), Flags: obj.AccessFlags}}
	attrs := func(row *declaration, attributes []AttributeInfo) {
		for _, attribute := range attributes {
			switch a := attribute.(type) {
			case *SignatureAttribute:
				if a == nil {
					t.Fatal("nil original Signature")
				}
				s, err := obj.getUtf8(a.SignatureIndex)
				if err != nil {
					t.Fatal(err)
				}
				row.Signatures = append(row.Signatures, s)
			case *ExceptionsAttribute:
				if a == nil {
					t.Fatal("nil original Exceptions")
				}
				names := []string{}
				for _, index := range a.ExceptionIndexTable {
					constant, err := obj.getConstantInfo(index)
					if err != nil {
						t.Fatal(err)
					}
					c, ok := constant.(*ConstantClassInfo)
					if !ok || c == nil {
						t.Fatal("bad original exception class")
					}
					n, err := obj.getUtf8(c.NameIndex)
					if err != nil {
						t.Fatal(err)
					}
					names = append(names, n)
				}
				row.Exceptions = append(row.Exceptions, names)
			}
		}
	}
	attrs(&rows[0], obj.Attributes)
	for _, group := range []struct {
		kind    string
		members []*MemberInfo
	}{{"field", obj.Fields}, {"method", obj.Methods}} {
		for _, m := range group.members {
			n, err := obj.getUtf8(m.NameIndex)
			if err != nil {
				t.Fatal(err)
			}
			d, err := obj.getUtf8(m.DescriptorIndex)
			if err != nil {
				t.Fatal(err)
			}
			row := declaration{Kind: group.kind, Name: n, Descriptor: d, Flags: m.AccessFlags}
			attrs(&row, m.Attributes)
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		return a.Kind+a.Name+a.Descriptor < b.Kind+b.Name+b.Descriptor
	})
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestAdversarialConstructorInvocationGenericThrowsOriginalRoundTrip(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		t.Run(string(profile), func(t *testing.T) {
			javac := modern
			if profile == NativeJavac8 {
				javac = os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required for independent native8 metadata")
				}
				version, err := exec.Command(javac, "-version").CombinedOutput()
				if err != nil || !strings.Contains(string(version), "javac 1.8.") {
					t.Fatal("native8 profile needs actual javac8", err, string(version))
				}
			}
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					var files map[string][]byte
					if profile == NativeJavac8 {
						files = nativePrivateEnumCompile(t, constructorInvocationThrowsFixture, "CtorThrowLoose", debug)
					} else {
						files = nativeCompileDebugClasses(t, constructorInvocationThrowsFixture, debug)
					}
					before := map[string][]byte{}
					original := t.TempDir()
					for name, raw := range files {
						before[name] = bytes.Clone(raw)
						if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					want := "44:generic-ctor:same:order:overload:abi\n"
					if got := t04RunJava(t, java, original, "CtorThrowDriver"); got != want {
						t.Fatalf("independent original JVM oracle: %q", got)
					}
					t.Log("independent original JVM: " + strings.TrimSpace(want))
					resolve := func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }
					children := []string{"CtorThrowLoose", "CtorThrowException", "CtorThrowArgument", "CtorThrowNarrow", "CtorThrowPair", "CtorThrowIntersection", "CtorThrowShadow", "CtorThrowOverload", "CtorThrowChecked", "CtorThrowOrdinary", "CtorThrowMethodShadow", "CtorThrowCast", "CtorThrowConditional", "CtorThrowOpaqueBroad", "CtorThrowMixedIO", "CtorThrowMixedOrdinary", "CtorThrowOpaqueException", "CtorThrowRivalMixed", "CtorThrowOpaquePlain"}
					owned := map[string]bool{}
					for _, name := range children {
						owned[name+".class"] = true
					}
					for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(policy, func(t *testing.T) {
							env := map[string]string{}
							if policy == "no-source-rewrites" {
								env["JDEC_NO_SOURCE_REWRITES"] = "1"
							}
							if policy == "no-core-cleanups" {
								env["JDEC_NO_CORE_CLEANUPS"] = "1"
							}
							out := t.TempDir()
							for name, raw := range files {
								if !owned[name] {
									if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
										t.Fatal(err)
									}
								}
							}
							var paths []string
							for _, name := range children {
								result, err := DecompileWithOptions(bytes.Clone(files[name+".class"]), DecompileOptions{Mode: Precision, TargetSourceVersion: 8, SourceCompiler: profile, Resolve: resolve, EnvSnapshot: env})
								if err != nil || strings.Contains(result.Source, DecompileStubMarker) {
									t.Errorf("generic throws binding %s: %v\n%s", name, err, result.Source)
									continue
								}
								path := filepath.Join(out, name+".java")
								if err := os.WriteFile(path, []byte(result.Source), 0600); err != nil {
									t.Fatal(err)
								}
								paths = append(paths, path)
							}
							if t.Failed() {
								return
							}
							args := []string{"-proc:none", "--release", "8"}
							if profile == NativeJavac8 {
								args = []string{"-proc:none", "-source", "8", "-target", "8"}
							}
							args = append(args, "-g:"+debug, "-cp", out, "-d", out)
							args = append(args, paths...)
							if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
								t.Fatalf("all original target binaries excluded; same-compiler rebuild: %v\n%s", err, log)
							}
							if got := t04RunJava(t, java, out, "CtorThrowDriver"); got != want {
								t.Fatalf("rebuilt JVM: %q want %q", got, want)
							}
							t.Log("fully rebuilt targets JVM: " + strings.TrimSpace(want))
							for name := range owned {
								rebuilt, err := os.ReadFile(filepath.Join(out, name))
								if err != nil {
									t.Fatal(err)
								}
								if a, b := constructorInvocationThrowsDeclarations(t, files[name]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
									t.Fatalf("exact original declaration ABI %s:\n%s\n%s", name, a, b)
								}
							}
							for name, raw := range files {
								if !bytes.Equal(raw, before[name]) {
									t.Fatal("immutable original dependency changed", name)
								}
							}
						})
					}
				})
			}
		})
	}
}
