package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The factory's method variable and the consumer's class variable have
// independent binders. Reusing their spelling is not a proof of equality.
// Exercise a custom interface as well as the JDK declaration, with recursive
// contravariant bounds and unchanged binary callers of the erased bridges.
const factoryReceiverBindingFixture = `
interface ReceiverOrder<N>{int compareKey(N other);}
interface ReceiverCmp<N>{int compare(N left,N right);}
class ReceiverState{
 static String trace="";
 static final RuntimeException failure=new IllegalStateException("same");
 static final RuntimeException nullRight=new IllegalArgumentException("right");
 static int mode;
}
class ReceiverFactory{
 static{ReceiverState.trace+="I";}
 public static <X extends ReceiverOrder<? super X>>ReceiverCmp<X> natural(){
  ReceiverState.trace+="F";
  if(ReceiverState.mode==1)throw ReceiverState.failure;
  if(ReceiverState.mode==2)return null;
  return(left,right)->left.compareKey(right);
 }
}
class ReceiverBase implements ReceiverOrder<ReceiverBase>{
 final int key;
 ReceiverBase(int key){this.key=key;}
 public int compareKey(ReceiverBase other){
  ReceiverState.trace+="C";
  if(key==17)throw ReceiverState.failure;
  if(other==null)throw ReceiverState.nullRight;
  long diff=(long)key-(long)other.key;
  return diff<0?-1:diff>0?1:0;
 }
}
class ReceiverLeaf extends ReceiverBase{ReceiverLeaf(int key){super(key);}}
class ReceiverPolluted implements ReceiverOrder<Object>{
 public int compareKey(Object other){ReceiverState.trace+="P";return other==this?37:-29;}
}
class FactoryCarrier<T extends ReceiverOrder<? super T>>implements ReceiverCmp<T>{
 public int compare(T left,T right){return ReceiverFactory.<T>natural().compare(left,right);}
}
class ReceiverDriver{
 @SuppressWarnings({"rawtypes","unchecked"})public static void main(String[]args){
  FactoryCarrier<ReceiverLeaf> typed=new FactoryCarrier<ReceiverLeaf>();
  ReceiverCmp raw=typed;
  int rows=0;
  Throwable early=null;ReceiverState.trace="";
  try{raw.compare(new Object(),new ReceiverLeaf(0));}catch(Throwable e){early=e;}
  if(!(early instanceof ClassCastException)||!ReceiverState.trace.equals(""))throw new AssertionError("bridge must fail before factory initialization",early);rows++;
  for(int left:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})
   for(int right:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){
    ReceiverState.trace="";
    int actual=typed.compare(new ReceiverLeaf(left),new ReceiverLeaf(right));
    long delta=(long)left-(long)right;int expected=delta<0?-1:delta>0?1:0;
    if(actual!=expected||!ReceiverState.trace.equals(rows==1?"IFC":"FC"))throw new AssertionError("comparison/overflow/initialization/order");rows++;
   }
  ReceiverPolluted polluted=new ReceiverPolluted();ReceiverState.trace="";
  if(raw.compare(polluted,polluted)!=37||!ReceiverState.trace.equals("FP"))throw new AssertionError("invented concrete type argument");rows++;
  Object[][] pairs={
   {null,new ReceiverLeaf(0)},{new ReceiverLeaf(0),null},
   {new ReceiverLeaf(17),new ReceiverLeaf(0)},
   {new Object(),new ReceiverLeaf(0)},{new ReceiverLeaf(0),new Object()}
  };
  for(int i=0;i<pairs.length;i++){
   ReceiverState.trace="";Throwable actual=null;
   try{raw.compare(pairs[i][0],pairs[i][1]);}catch(Throwable e){actual=e;}
   if(i==0?!(actual instanceof NullPointerException):i==1?actual!=ReceiverState.nullRight:i==2?actual!=ReceiverState.failure:!(actual instanceof ClassCastException))throw new AssertionError("cause identity/bridge boundary",actual);
   if(!ReceiverState.trace.equals(i==0?"F":i<3?"FC":""))throw new AssertionError("bridge/factory/callback priority");rows++;
  }
  for(int mode:new int[]{1,2}){
   ReceiverState.mode=mode;ReceiverState.trace="";Throwable actual=null;
   try{typed.compare(new ReceiverLeaf(0),new ReceiverLeaf(0));}catch(Throwable e){actual=e;}
   if(mode==1?actual!=ReceiverState.failure:!(actual instanceof NullPointerException))throw new AssertionError("factory failure/null receiver",actual);
   if(!ReceiverState.trace.equals("F"))throw new AssertionError("factory evaluated more than once");rows++;
  }
  ReceiverState.mode=0;
  System.out.println(rows+":recursive:contravariant:bridge:identity");
 }
}`

func TestAdversarialGenericFactoryReceiverKeepsIndependentBinders(t *testing.T) {
	modern, java := t04Tools(t)
	for _, shape := range []string{"custom", "jdk"} {
		t.Run(shape, func(t *testing.T) {
			fixture := factoryReceiverBindingFixture
			if shape == "jdk" {
				fixture = strings.Replace(fixture, "interface ReceiverOrder<N>{int compareKey(N other);}", "", 1)
				fixture = strings.Replace(fixture, "interface ReceiverCmp<N>{int compare(N left,N right);}", "", 1)
				fixture = strings.NewReplacer("ReceiverOrder", "Comparable", "ReceiverCmp", "java.util.Comparator", "compareKey", "compareTo").Replace(fixture)
				fixture = strings.Replace(fixture, " public int compare(T left,T right){return ReceiverFactory.<T>natural().compare(left,right);}", " public int compare(T left,T right){return ReceiverFactory.<T>natural().compare(left,right);}\n public int builtin(T left,T right){return java.util.Comparator.<T>naturalOrder().compare(left,right);}", 1)
				fixture = strings.Replace(fixture, "  ReceiverState.mode=0;", `  ReceiverState.mode=0;ReceiverState.trace="";
  if(typed.builtin(new ReceiverLeaf(Integer.MIN_VALUE),new ReceiverLeaf(Integer.MAX_VALUE))!=-1||!ReceiverState.trace.equals("C"))throw new AssertionError("JDK factory inference");
	  rows++;ReceiverState.trace="";Throwable builtinNull=null;try{typed.builtin(null,new ReceiverLeaf(0));}catch(Throwable e){builtinNull=e;}
	  if(!(builtinNull instanceof NullPointerException)||!ReceiverState.trace.equals(""))throw new AssertionError("JDK null receiver");rows++;`, 1)
			}
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(string(profile), func(t *testing.T) {
					javac := modern
					if profile == NativeJavac8 {
						javac = os.Getenv("JAVA8_JAVAC")
						if javac == "" {
							t.Skip("JAVA8_JAVAC required for the independent compiler")
						}
						if version, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(version), "javac 1.8.") {
							t.Fatalf("native8 needs actual javac8: %v / %s", err, version)
						}
					}
					for _, owner := range []string{"FactoryCarrier", "AlternateFactory"} {
						t.Run(owner, func(t *testing.T) {
							source := strings.ReplaceAll(fixture, "FactoryCarrier", owner)
							for _, debug := range []string{"none", "source,lines,vars"} {
								t.Run(debug, func(t *testing.T) {
									var files map[string][]byte
									if profile == NativeJavac8 {
										files = nativePrivateEnumCompile(t, source, owner, debug)
									} else {
										files = nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
									}
									write := func(dir, name string, data []byte) {
										t.Helper()
										if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
											t.Fatal(err)
										}
									}
									original := t.TempDir()
									for name, raw := range files {
										write(original, name, raw)
									}
									want := "34:recursive:contravariant:bridge:identity\n"
									if shape == "jdk" {
										want = "36:recursive:contravariant:bridge:identity\n"
									}
									if got := t04RunJava(t, java, original, "ReceiverDriver"); got != want {
										t.Fatalf("independent original: %q want %q", got, want)
									}
									t.Log("independent original factory-receiver JVM: " + strings.TrimSpace(want))
									for _, mode := range []DecompileMode{Precision, Compatibility} {
										t.Run(string(mode), func(t *testing.T) {
											result, err := DecompileWithOptions(bytes.Clone(files[owner+".class"]), DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { data, known := files[name+".class"]; return bytes.Clone(data), known }})
											if err != nil || strings.Contains(result.Source, DecompileStubMarker) || len(result.StubMethods) != 0 {
												t.Fatalf("source status=%s: %v diagnostics=%+v\n%s", result.Status, err, result.Diagnostics, result.Source)
											}
											for _, diagnostic := range result.Diagnostics {
												if diagnostic.Code != "legacy_source_recovery_disabled" {
													t.Fatalf("unexpected binding/recovery diagnostic: %+v\n%s", diagnostic, result.Source)
												}
											}
											if len(result.RulesApplied) != 0 {
												t.Fatalf("the fixture used an unproved source rewrite: %+v", result.RulesApplied)
											}
											// The existing ledger does not certify an argument-adapting
											// generic bridge. Preserve that explicit unknown rather than
											// changing product status merely to make this fixture green.
											// Only that exact original bridge may be unproved; the full
											// regenerated declaration ABI and runtime are checked below.
											unknownBridges := 0
											for _, member := range result.Members {
												if member.State == "preserved" || member.State == "regenerated" {
													continue
												}
												if member.State != "unsupported" || member.Kind != "method" || member.Name != "compare" || member.Descriptor != "(Ljava/lang/Object;Ljava/lang/Object;)I" {
													t.Fatalf("unrelated reconstruction failure: %+v", member)
												}
												unknownBridges++
											}
											if unknownBridges != 1 || result.Status != "unsupported" {
												t.Fatalf("the independent bridge ledger gap changed: status=%s unknown=%d", result.Status, unknownBridges)
											}
											out := t.TempDir()
											for name, raw := range files {
												if name != owner+".class" {
													write(out, name, raw)
												}
											}
											write(out, owner+".java", []byte(result.Source))
											args := []string{"-proc:none", "--release", "8"}
											if profile == NativeJavac8 {
												args = []string{"-proc:none", "-source", "8", "-target", "8"}
											}
											args = append(args, "-g:"+debug, "-cp", out, "-d", out, filepath.Join(out, owner+".java"))
											if log, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
												t.Fatalf("rebuild without original target: %v\n%s\n%s", err, log, result.Source)
											}
											if got := t04RunJava(t, java, out, "ReceiverDriver"); got != want {
												t.Fatalf("unchanged binary caller: %q want %q\n%s", got, want, result.Source)
											}
											rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
											if err != nil {
												t.Fatal(err)
											}
											if a, b := constructorInvocationThrowsDeclarations(t, files[owner+".class"]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
												t.Fatalf("declaration inventory changed:\n%s\n%s", a, b)
											}
											nativeCovariantBridgeABI(t, owner, files[owner+".class"], rebuilt)
											if a, b := factoryReceiverLowering(t, files[owner+".class"]), factoryReceiverLowering(t, rebuilt); a != b {
												t.Fatalf("raw source views changed original checks, calls or evaluation order:\n%s\n%s", a, b)
											}
											t.Log("fully rebuilt factory-receiver JVM: " + strings.TrimSpace(want))
										})
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

// These authored methods are straight-line forwarders. Normalize CP indexes,
// then compare every instruction, invoke kind/owner/descriptor and CHECKCAST.
// This is independent of the renderer: descriptor-identity source casts must
// compile away, including inside the regenerated argument-adapting bridge.
func factoryReceiverLowering(t *testing.T, raw []byte) string {
	t.Helper()
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	rows := []string{}
	for _, method := range obj.Methods {
		name, _ := sourceBridgeUTF8(obj, method.NameIndex)
		desc, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
		for _, attribute := range method.Attributes {
			code, ok := attribute.(*CodeAttribute)
			if !ok {
				continue
			}
			if len(code.ExceptionTable) != 0 {
				t.Fatal("unexpected authored forwarder handler")
			}
			parts := []string{fmt.Sprintf("%s%s stack=%d locals=%d", name, desc, code.MaxStack, code.MaxLocals)}
			for offset := 0; offset < len(code.Code); {
				op := code.Code[offset]
				offset++
				switch {
				case op >= 0x2a && op <= 0x2d || op == 0xac || op == 0xb1:
					parts = append(parts, fmt.Sprintf("%02x", op))
				case op == 0xc0 || op >= 0xb6 && op <= 0xb9:
					if len(code.Code)-offset < 2 {
						t.Fatal("truncated authored symbolic instruction")
					}
					index := binary.BigEndian.Uint16(code.Code[offset : offset+2])
					offset += 2
					if op == 0xc0 {
						owner, known := sourceBridgeClassName(obj, index)
						if !known {
							t.Fatal("unknown authored cast")
						}
						parts = append(parts, "checkcast:"+owner)
						continue
					}
					if index == 0 || int(index) > len(obj.ConstantPool) {
						t.Fatal("bad authored method reference")
					}
					var ref ConstantMemberrefInfo
					tag := "method"
					switch entry := obj.ConstantPool[index-1].(type) {
					case *ConstantMethodrefInfo:
						ref = entry.ConstantMemberrefInfo
					case *ConstantInterfaceMethodrefInfo:
						ref, tag = entry.ConstantMemberrefInfo, "interface"
					default:
						t.Fatal("unexpected authored member reference")
					}
					owner, known := sourceBridgeClassName(obj, ref.ClassIndex)
					if !known || ref.NameAndTypeIndex == 0 || int(ref.NameAndTypeIndex) > len(obj.ConstantPool) {
						t.Fatal("bad authored invoke owner")
					}
					nt, ok := obj.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					if !ok {
						t.Fatal("bad authored method name/type")
					}
					n, _ := sourceBridgeUTF8(obj, nt.NameIndex)
					d, _ := sourceBridgeUTF8(obj, nt.DescriptorIndex)
					parts = append(parts, fmt.Sprintf("%02x:%s:%s.%s%s", op, tag, owner, n, d))
					if op == 0xb9 {
						if len(code.Code)-offset < 2 {
							t.Fatal("truncated authored interface operands")
						}
						parts = append(parts, fmt.Sprintf("interface-operands:%x", code.Code[offset:offset+2]))
						offset += 2
					}
				default:
					t.Fatalf("unexpected authored forwarder opcode %x", op)
				}
			}
			rows = append(rows, strings.Join(parts, " "))
		}
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}
