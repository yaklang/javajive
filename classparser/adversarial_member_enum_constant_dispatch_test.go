package javaclassparser

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestAdversarialMemberEnumConstantDispatchPreservesEnclosure(t *testing.T) {
	for _, root := range []string{"DispatchModeOwner", "RenamedModeOwner"} {
		for _, deep := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deep=%v", root, deep), func(t *testing.T) {
				source := `public class DispatchModeOwner {
 public interface Operation {long apply(long value);}
 public enum Mode implements Operation {
  ADD {public long apply(long value){return value+1;}},
  NEGATE {public long apply(long value){return -value;}};
 }
 public class View {
  private final Mode mode;
  public View(Mode mode){this.mode=mode;}
  public long run(long value){return mode.apply(value);}
 }
 public View make(Mode mode){return new View(mode);}
}
class ConstantDispatchDriver {
 public static void main(String[]args){
  int rows=0;
  DispatchModeOwner root=new DispatchModeOwner();
  if(DispatchModeOwner.Mode.class.getDeclaringClass()!=DispatchModeOwner.class)throw new AssertionError("named enum ownership");
  for(DispatchModeOwner.Mode mode:DispatchModeOwner.Mode.values()){
   if(mode.getDeclaringClass()!=DispatchModeOwner.Mode.class||mode.getClass().getEnclosingClass()!=DispatchModeOwner.Mode.class)throw new AssertionError("constant subclass ownership");
   if(DispatchModeOwner.Mode.valueOf(mode.name())!=mode)throw new AssertionError("factory identity");
   DispatchModeOwner.View view=root.make(mode);
   if(view.getClass().getDeclaringClass()!=DispatchModeOwner.class)throw new AssertionError("view ownership");
   for(long value:new long[]{Long.MIN_VALUE,Long.MIN_VALUE+1,-1,0,1,Long.MAX_VALUE-1,Long.MAX_VALUE}){
    java.math.BigInteger n=java.math.BigInteger.valueOf(value);
    long expected=(mode==DispatchModeOwner.Mode.ADD?n.add(java.math.BigInteger.ONE):n.negate()).longValue();
    if(mode.apply(value)!=expected||view.run(value)!=expected)throw new AssertionError("virtual dispatch/overflow");rows++;
   }
  }
  DispatchModeOwner.Mode[] values=DispatchModeOwner.Mode.values();values[0]=null;
  if(DispatchModeOwner.Mode.values()[0]!=DispatchModeOwner.Mode.ADD)throw new AssertionError("factory defensive copy");
  try{root.make(null).run(7);throw new AssertionError("null receiver");}catch(NullPointerException expected){}
  System.out.println(rows+":enum:constant:virtual:ownership:overflow:null");
 }
}`
				source = strings.ReplaceAll(source, "DispatchModeOwner", root)
				if deep {
					start := strings.Index(source, " public enum Mode")
					end := strings.Index(source, " public class View")
					source = source[:start] + " public static class Scope {" + source[start:end] + " }\n" + source[end:]
					source = strings.ReplaceAll(source, "private final Mode", "private final Scope.Mode")
					source = strings.ReplaceAll(source, "View(Mode", "View(Scope.Mode")
					source = strings.ReplaceAll(source, "make(Mode", "make(Scope.Mode")
					source = strings.ReplaceAll(source, root+".Mode", root+".Scope.Mode")
					source = strings.Replace(source, "getDeclaringClass()!="+root+".class)throw new AssertionError(\"named enum ownership\")", "getDeclaringClass()!="+root+".Scope.class)throw new AssertionError(\"named enum ownership\")", 1)
				}
				testNativePrivateSetterCompiledFixture(t, root, "ConstantDispatchDriver", "14:enum:constant:virtual:ownership:overflow:null\n", func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, debug, "8")
				}, nativeEnumConstantConstructorPacketOracle)
			})
		}
	}
}

func TestAdversarialMemberEnumConstantWideMixedAllocation(t *testing.T) {
	for _, root := range []string{"WideConstantScope", "OtherWideConstantScope"} {
		t.Run(root, func(t *testing.T) {
			source := `public class WideConstantScope {
 public enum Mode {
  IDENTITY(0,0.0),
  ADD(1,0.25) {public long apply(long value){return helper(value,step);}private long helper(long value,long n){return value+n;}},
  NEGATE(0,-0.0) {public long apply(long value){return -value;}};
  final long step;final double bias;
  Mode(long step,double bias){this.step=step;this.bias=bias;}
  public long apply(long value){return value;}
 }
 public class View {final Mode mode;public View(Mode mode){this.mode=mode;}public long run(long value){return mode.apply(value);}}
 public View make(Mode mode){return new View(mode);}
}
class WideConstantDriver {public static void main(String[]args){
 int rows=0;WideConstantScope root=new WideConstantScope();
 if(WideConstantScope.Mode.class.getDeclaringClass()!=WideConstantScope.class)throw new AssertionError("enum owner");
 for(WideConstantScope.Mode mode:WideConstantScope.Mode.values()){
  if(mode.getDeclaringClass()!=WideConstantScope.Mode.class||WideConstantScope.Mode.valueOf(mode.name())!=mode)throw new AssertionError("identity");
  if(mode!=WideConstantScope.Mode.IDENTITY&&mode.getClass().getEnclosingClass()!=WideConstantScope.Mode.class)throw new AssertionError("constant owner");
  long bits=Double.doubleToRawLongBits(mode.bias);long expectedBits=mode==WideConstantScope.Mode.IDENTITY?0L:mode==WideConstantScope.Mode.ADD?0x3fd0000000000000L:Long.MIN_VALUE;
  if(bits!=expectedBits||mode.step!=(mode==WideConstantScope.Mode.ADD?1L:0L))throw new AssertionError("wide constructor binding");
  for(long value:new long[]{Long.MIN_VALUE,Long.MIN_VALUE+1,-1,0,1,Long.MAX_VALUE-1,Long.MAX_VALUE}){
   java.math.BigInteger n=java.math.BigInteger.valueOf(value);
   long expected=(mode==WideConstantScope.Mode.IDENTITY?n:mode==WideConstantScope.Mode.ADD?n.add(java.math.BigInteger.ONE):n.negate()).longValue();
   if(mode.apply(value)!=expected||root.make(mode).run(value)!=expected)throw new AssertionError("mixed virtual dispatch");rows++;
  }
 }
 System.out.println(rows+":enum:mixed:wide:private:identity:overflow");
}}`
			source = strings.ReplaceAll(source, "WideConstantScope", root)
			testNativePrivateSetterCompiledFixture(t, root, "WideConstantDriver", "21:enum:mixed:wide:private:identity:overflow\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, debug, "8")
			}, nativeEnumConstantConstructorPacketOracle)
		})
	}
}

// This oracle reads original and regenerated classfiles independently of the
// production admission proof. Constant-specific constructors disappear from
// Java source, so compare their executable packet, capacity and all declaration
// rows in addition to the untouched driver's JVM behavior and ABI.
func nativeEnumConstantConstructorPacketOracle(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	a, e := Parse(original)
	if e != nil {
		t.Fatal(e)
	}
	b, e := Parse(rebuilt)
	if e != nil {
		t.Fatal(e)
	}
	nativeLexicalExactSignatures(t, name, original, rebuilt)
	methods := func(obj *ClassObject) []string {
		var rows []string
		for _, m := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, m.NameIndex)
			d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
			rows = append(rows, fmt.Sprintf("%s%s:%x", n, d, m.AccessFlags))
		}
		sort.Strings(rows)
		return rows
	}
	if !reflect.DeepEqual(methods(a), methods(b)) {
		t.Fatalf("original method and bridge inventory %s\n%v\n%v", name, methods(a), methods(b))
	}

	parameters := func(obj *ClassObject) []string {
		var rows []string
		for _, m := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, m.NameIndex)
			d, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
			for _, a := range m.Attributes {
				if a, ok := a.(*UnparsedAttribute); ok && a.Name == "MethodParameters" {
					if len(a.Info) < 1 || len(a.Info) != 1+4*int(a.Info[0]) {
						t.Fatal("malformed original parameters")
					}
					row := n + d + ":"
					for i := 1; i < len(a.Info); i += 4 {
						idx := uint16(a.Info[i])<<8 | uint16(a.Info[i+1])
						name := ""
						if idx != 0 {
							var ok bool
							name, ok = sourceBridgeUTF8(obj, idx)
							if !ok {
								t.Fatal("bad parameter name")
							}
						}
						row += fmt.Sprintf("%s/%x;", name, uint16(a.Info[i+2])<<8|uint16(a.Info[i+3]))
					}
					rows = append(rows, row)
				}
			}
		}
		sort.Strings(rows)
		return rows
	}
	if !reflect.DeepEqual(parameters(a), parameters(b)) {
		t.Fatalf("original reflection parameter flags/names %s\n%v\n%v", name, parameters(a), parameters(b))
	}

	if a.AccessFlags == 0x4030 && a.GetSupperClassName() != "java/lang/Enum" {
		selectCtor := func(obj *ClassObject) []string {
			rows := nativeEnumSwitchOriginalPacketShape(t, obj)
			var out []string
			keep := false
			for _, row := range rows {
				if strings.HasPrefix(row, "method:") {
					keep = strings.HasPrefix(row, "method:<init>")
				}
				if keep {
					out = append(out, row)
				}
			}
			return out
		}
		if !reflect.DeepEqual(selectCtor(a), selectCtor(b)) {
			t.Fatalf("constant constructor packet %s\n%v\n%v", name, selectCtor(a), selectCtor(b))
		}
	}
	if !reflect.DeepEqual(nativeNestedEnumMetadataShape(t, a), nativeNestedEnumMetadataShape(t, b)) {
		t.Fatalf("constant-family original metadata %s\n%v\n%v", name, nativeNestedEnumMetadataShape(t, a), nativeNestedEnumMetadataShape(t, b))
	}
}

func TestAdversarialMemberEnumConstantGenericVirtualBridge(t *testing.T) {
	for _, root := range []string{"GenericConstantScope", "RenamedGenericScope"} {
		t.Run(root, func(t *testing.T) {
			source := `public class GenericConstantScope {
 public enum Mode implements java.util.function.Function<Number,Number> {
  NARROW {public Integer apply(Number value){return Integer.valueOf(increment(value.intValue()));}private int increment(int value){return value+1;}},
  NEGATE {public Long apply(Number value){return Long.valueOf(-value.longValue());}};
 }
 public class View {final Mode mode;public View(Mode mode){this.mode=mode;}public Number run(Number value){return mode.apply(value);}}
 public View make(Mode mode){return new View(mode);}
}
class GenericConstantDriver {public static void main(String[]args){
 int rows=0;GenericConstantScope root=new GenericConstantScope();
 if(GenericConstantScope.Mode.class.getDeclaringClass()!=GenericConstantScope.class)throw new AssertionError("enum owner");
 for(GenericConstantScope.Mode mode:GenericConstantScope.Mode.values()){
  if(mode.getClass().getEnclosingClass()!=GenericConstantScope.Mode.class||mode.getDeclaringClass()!=GenericConstantScope.Mode.class)throw new AssertionError("constant owner");
  for(Number value:new Number[]{Long.valueOf(Long.MIN_VALUE),Long.valueOf(Long.MAX_VALUE),Long.valueOf(-1),Long.valueOf(0),Long.valueOf(1),Integer.valueOf(Integer.MIN_VALUE),Integer.valueOf(Integer.MAX_VALUE)}){
   java.math.BigInteger n=java.math.BigInteger.valueOf(mode==GenericConstantScope.Mode.NARROW?value.intValue():value.longValue());
   long expected=mode==GenericConstantScope.Mode.NARROW?n.add(java.math.BigInteger.ONE).intValue():n.negate().longValue();
   Number result=mode.apply(value),throughView=root.make(mode).run(value);
   if(result.longValue()!=expected||throughView.longValue()!=expected||result.getClass()!=(mode==GenericConstantScope.Mode.NARROW?Integer.class:Long.class))throw new AssertionError("generic virtual binding/overflow");rows++;
  }
  java.util.function.Function erased=mode;
  try{erased.apply("wrong");throw new AssertionError("missing bridge cast");}catch(ClassCastException expected){}
  try{erased.apply(null);throw new AssertionError("missing null failure");}catch(NullPointerException expected){}
  if(GenericConstantScope.Mode.valueOf(mode.name())!=mode)throw new AssertionError("identity");
 }
 System.out.println(rows+":enum:generic:covariant:bridge:overflow:cast:null");
}}`
			source = strings.ReplaceAll(source, "GenericConstantScope", root)
			testNativePrivateSetterCompiledFixture(t, root, "GenericConstantDriver", "14:enum:generic:covariant:bridge:overflow:cast:null\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, debug, "8")
			}, nativeEnumConstantConstructorPacketOracle)
		})
	}
}

func TestAdversarialMemberEnumConstantEmptyBodyRetainsIdentity(t *testing.T) {
	for _, root := range []string{"EmptyConstantScope", "RenamedEmptyScope"} {
		t.Run(root, func(t *testing.T) {
			source := `public class EmptyConstantScope {public enum Mode{PLAIN,EMPTY{},ADD{public long apply(long value){return value+1;}};public long apply(long value){return value;}}}
class EmptyConstantDriver{public static void main(String[]args){int rows=0;
 if(EmptyConstantScope.Mode.class.getDeclaringClass()!=EmptyConstantScope.class)throw new AssertionError("enum owner");
 for(EmptyConstantScope.Mode mode:EmptyConstantScope.Mode.values()){
  if(mode.getDeclaringClass()!=EmptyConstantScope.Mode.class||EmptyConstantScope.Mode.valueOf(mode.name())!=mode)throw new AssertionError("enum identity");
  if(mode==EmptyConstantScope.Mode.PLAIN){if(mode.getClass()!=EmptyConstantScope.Mode.class)throw new AssertionError("plain identity");}
  else if(mode.getClass()==EmptyConstantScope.Mode.class||mode.getClass().getEnclosingClass()!=EmptyConstantScope.Mode.class)throw new AssertionError("empty body class identity");
  for(long value:new long[]{Long.MIN_VALUE,Long.MIN_VALUE+1,-1,0,1,Long.MAX_VALUE-1,Long.MAX_VALUE}){
   java.math.BigInteger n=java.math.BigInteger.valueOf(value);long expected=(mode==EmptyConstantScope.Mode.ADD?n.add(java.math.BigInteger.ONE):n).longValue();
   if(mode.apply(value)!=expected)throw new AssertionError("dispatch/overflow");rows++;
  }
 }
 System.out.println(rows+":enum:empty:body:class:identity:overflow");}}
`
			source = strings.ReplaceAll(source, "EmptyConstantScope", root)
			testNativePrivateSetterCompiledFixture(t, root, "EmptyConstantDriver", "21:enum:empty:body:class:identity:overflow\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": source}, debug, "8")
			}, nativeEnumConstantConstructorPacketOracle)
		})
	}
}
