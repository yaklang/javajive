package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// An array input is not a result-identity certificate, even for <T>T f(T).
// The unchecked producer deliberately returns arrays with a different component
// type. The actual original bytecode fixes each consumption check; reconstruction
// must neither add a narrower check nor discard an existing one.
const arrayProducerResultFixture = `import java.util.*;
class ArrayResultState{
 static String trace="";static int builds;static Object[] packet;static boolean fail;
 static final RuntimeException fault=new IllegalStateException("same");
 static void begin(){trace="";builds=0;}
 static void produce(String step){trace+=step;if(++builds!=1)throw new AssertionError("producer repeated");if(fail)throw fault;}
}
public class ArrayResultOwner extends ArrayResultBase<ArrayResultOwner>{
 public final List<String> stored;public final boolean enabled;
 public ArrayResultOwner(){stored=null;enabled=false;}
 public ArrayResultOwner(String... input){this(true,Arrays.asList(genericFactory(input)));}
 public ArrayResultOwner(boolean enabled,String... input){this(enabled,Arrays.asList(genericFactory(input)));}
 private ArrayResultOwner(List<String> values){stored=values;enabled=false;}
 private ArrayResultOwner(boolean enabled,List<String> values){stored=values;this.enabled=enabled;}
 public ArrayResultOwner accept(String... input){return accept(convertToValues(input));}
 private static Object[] convertToValues(String[] input){ArrayResultState.produce("P");return ArrayResultState.packet;}
 public static String checked(String... input){return choose((String[])convertToValues(input));}
 public static String choose(Object... input){ArrayResultState.trace+="O";return "object";}
 public static String choose(String... input){ArrayResultState.trace+="S";return "string";}
 public static <T>T genericFactory(T input){ArrayResultState.produce("G");return(T)ArrayResultState.packet;}
 public static List<String> inferred(String[] input){return Arrays.asList(genericFactory(input));}
 public static String data(){return "assertj (String[])(convertToPayload(payload))";}
}
class ArrayResultLeaf extends ArrayResultOwner{
 public ArrayResultOwner accept(Object... input){ArrayResultState.trace+="X";if(input!=ArrayResultState.packet)throw new AssertionError("virtual array identity");return this;}
}
class ArrayResultDriver{
 public static void main(String[] args){int rows=0;
  for(Object[] packet:new Object[][]{null,new Object[]{Long.valueOf(7)},new String[]{"same"},new Object[0]})for(boolean fail:new boolean[]{false,true}){
   ArrayResultState.packet=packet;ArrayResultState.fail=fail;
   for(boolean leaf:new boolean[]{false,true}){
    ArrayResultState.begin();ArrayResultOwner receiver=leaf?new ArrayResultLeaf():new ArrayResultOwner();ArrayResultOwner result=null;Throwable actual=null;
    try{result=receiver.accept("input");}catch(Throwable e){actual=e;}
    if(fail?actual!=ArrayResultState.fault:actual!=null||result!=receiver)throw new AssertionError("selected inherited Object[] overload",actual);
    if(ArrayResultState.builds!=1||!ArrayResultState.trace.equals(fail?"P":leaf?"PX":"PO"))throw new AssertionError("producer/virtual dispatch priority");rows++;
   }
   ArrayResultState.begin();String selected=null;Throwable actual=null;
   try{selected=ArrayResultOwner.checked("input");}catch(Throwable e){actual=e;}
   boolean compatible=packet==null||packet instanceof String[];
   if(fail?actual!=ArrayResultState.fault:compatible?actual!=null||!"string".equals(selected):!(actual instanceof ClassCastException))throw new AssertionError("original CHECKCAST and selected String[] overload",actual);
   if(ArrayResultState.builds!=1||!ArrayResultState.trace.equals(!fail&&compatible?"PS":"P"))throw new AssertionError("checked conversion priority");rows++;
   for(int constructor=0;constructor<4;constructor++){
    ArrayResultState.begin();List<?> result=null;actual=null;
    try{if(constructor==0)result=ArrayResultOwner.inferred(new String[]{"input"});else{
     ArrayResultOwner value=constructor==1?new ArrayResultOwner("input"):new ArrayResultOwner(constructor==2,"input");
     if(value.enabled!=(constructor!=3))throw new AssertionError("descriptor-selected boolean prefix");result=value.stored;
    }}catch(Throwable e){actual=e;}
    boolean narrow=constructor==0?ArrayResultOriginalChecks.RETURN_NARROW:constructor==1?ArrayResultOriginalChecks.CONSTRUCTOR_NARROW:ArrayResultOriginalChecks.PREFIX_NARROW;
    boolean rejected=packet!=null&&narrow&&!(packet instanceof String[]);
    if(fail?actual!=ArrayResultState.fault:packet==null?!(actual instanceof NullPointerException):rejected?!(actual instanceof ClassCastException):actual!=null)throw new AssertionError("generic signature is not result identity",actual);
    if(ArrayResultState.builds!=1||!ArrayResultState.trace.equals("G"))throw new AssertionError("generic producer evaluated more than once");
    if(!fail&&packet!=null&&!rejected){
     if(result.size()!=packet.length)throw new AssertionError("array length");
     for(int i=0;i<packet.length;i++)if(result.get(i)!=packet[i])throw new AssertionError("heap pollution or element identity");
     if(packet.length>0){Object marker=new Object();Throwable store=null;try{((List)result).set(0,marker);}catch(Throwable e){store=e;}
      if(packet instanceof String[]?!(store instanceof ArrayStoreException):store!=null||packet[0]!=marker)throw new AssertionError("backing array identity and runtime component",store);
     }
    }rows++;
   }
  }
  if(!ArrayResultOwner.data().equals("assertj (String[])(convertToPayload(payload))"))throw new AssertionError("application data changed");rows++;
  System.out.println(rows+":array-result:overload:check:identity:pollution");
 }
}`

func TestAdversarialArrayProducerResultCannotBorrowInputArrayType(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			javac := modern
			if profile == NativeJavac8 {
				javac = os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required for independent compiler")
				}
				if version, err := exec.Command(javac, "-version").CombinedOutput(); err != nil || !strings.Contains(string(version), "javac 1.8.") {
					t.Fatalf("actual javac8 required: %v / %s", err, version)
				}
			}
			for _, owner := range []string{"ArrayResultOwner", "AlternateArrayProducer"} {
				t.Run(owner, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							original := t.TempDir()
							base := `public class ArrayResultBase<S>{public S accept(Object... input){ArrayResultState.trace+="O";if(input!=ArrayResultState.packet)throw new AssertionError("array identity");return(S)this;}}`
							fixture := strings.ReplaceAll(arrayProducerResultFixture, "ArrayResultOwner", owner)
							declarations, driver, hasDriver := strings.Cut(fixture, "class ArrayResultDriver{")
							if !hasDriver {
								t.Fatal("authored driver boundary")
							}
							paths := []string{}
							for name, source := range map[string]string{owner + ".java": declarations, "ArrayResultBase.java": base} {
								path := filepath.Join(original, name)
								if err := os.WriteFile(path, []byte(source), 0600); err != nil {
									t.Fatal(err)
								}
								paths = append(paths, path)
							}
							args := []string{"-proc:none", "--release", "8"}
							if profile == NativeJavac8 {
								args = []string{"-proc:none", "-source", "8", "-target", "8"}
							}
							args = append(args, "-g:"+debug, "-d", original)
							if log, err := exec.Command(javac, append(args, paths...)...).CombinedOutput(); err != nil {
								t.Fatalf("original compile: %v\n%s", err, log)
							}
							originalTarget, err := os.ReadFile(filepath.Join(original, owner+".class"))
							if err != nil {
								t.Fatal(err)
							}
							// Freeze the actual original checks into a separate binary caller.
							// javac8 uses Object[] at these edges; modern javac may use
							// String[]. Reading the rebuilt class as its own oracle would
							// conceal exactly the premature narrowing this test forbids.
							checks := "class ArrayResultOriginalChecks{static final boolean RETURN_NARROW=" + strconv.FormatBool(arrayProducerOriginalNarrowing(t, originalTarget, "inferred", "([Ljava/lang/String;)")) + ";static final boolean CONSTRUCTOR_NARROW=" + strconv.FormatBool(arrayProducerOriginalNarrowing(t, originalTarget, "<init>", "([Ljava/lang/String;)")) + ";static final boolean PREFIX_NARROW=" + strconv.FormatBool(arrayProducerOriginalNarrowing(t, originalTarget, "<init>", "(Z[Ljava/lang/String;)")) + ";}"
							driverPath := filepath.Join(original, "ArrayResultDriver.java")
							if err := os.WriteFile(driverPath, []byte("import java.util.*;"+checks+"class ArrayResultDriver{"+driver), 0600); err != nil {
								t.Fatal(err)
							}
							driverArgs := append([]string(nil), args[:len(args)-3]...)
							driverArgs = append(driverArgs, "-g:"+debug, "-cp", original, "-d", original, driverPath)
							if log, err := exec.Command(javac, driverArgs...).CombinedOutput(); err != nil {
								t.Fatalf("independent original-check caller: %v\n%s", err, log)
							}
							if after, err := os.ReadFile(filepath.Join(original, owner+".class")); err != nil || !bytes.Equal(after, originalTarget) {
								t.Fatal("building the independent caller changed its original target", err)
							}
							files := map[string][]byte{}
							entries, err := os.ReadDir(original)
							if err != nil {
								t.Fatal(err)
							}
							for _, entry := range entries {
								if strings.HasSuffix(entry.Name(), ".class") {
									raw, err := os.ReadFile(filepath.Join(original, entry.Name()))
									if err != nil {
										t.Fatal(err)
									}
									files[entry.Name()] = raw
								}
							}
							const want = "57:array-result:overload:check:identity:pollution\n"
							if got := t04RunJava(t, java, original, "ArrayResultDriver"); got != want {
								t.Fatalf("independent original JVM: %q want %q", got, want)
							}
							t.Log("independent original array-result JVM: " + strings.TrimSpace(want))
							for _, mode := range []DecompileMode{Precision, Compatibility} {
								t.Run(string(mode), func(t *testing.T) {
									result, err := DecompileWithOptions(bytes.Clone(files[owner+".class"]), DecompileOptions{Mode: mode, SourceCompiler: profile, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}, Resolve: func(name string) ([]byte, bool) { raw, known := files[name+".class"]; return bytes.Clone(raw), known }})
									if err != nil || result.Status != "complete" || len(result.StubMethods) != 0 || strings.Contains(result.Source, DecompileStubMarker) {
										t.Fatalf("source status=%s: %v diagnostics=%+v\n%s", result.Status, err, result.Diagnostics, result.Source)
									}
									out := t.TempDir()
									for name, raw := range files {
										if name != owner+".class" {
											if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
												t.Fatal(err)
											}
										}
									}
									path := filepath.Join(out, owner+".java")
									if err := os.WriteFile(path, []byte(result.Source), 0600); err != nil {
										t.Fatal(err)
									}
									rebuildArgs := append([]string(nil), args[:len(args)-3]...)
									rebuildArgs = append(rebuildArgs, "-g:"+debug, "-cp", out, "-d", out, path)
									if log, err := exec.Command(javac, rebuildArgs...).CombinedOutput(); err != nil {
										t.Fatalf("rebuild without original target: %v\n%s\n%s", err, log, result.Source)
									}
									if got := t04RunJava(t, java, out, "ArrayResultDriver"); got != want {
										t.Fatalf("unchanged binary caller: %q want %q\n%s", got, want, result.Source)
									}
									rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
									if err != nil {
										t.Fatal(err)
									}
									if a, b := constructorInvocationThrowsDeclarations(t, files[owner+".class"]), constructorInvocationThrowsDeclarations(t, rebuilt); a != b {
										t.Fatalf("declaration inventory changed:\n%s\n%s", a, b)
									}
									if a, b := arrayProducerCheckedInvocations(t, files[owner+".class"]), arrayProducerCheckedInvocations(t, rebuilt); a != b {
										t.Fatalf("original checks or invocation bindings changed:\n%s\n%s", a, b)
									}
									t.Log("fully rebuilt array-result JVM: " + strings.TrimSpace(want))
								})
							}
						})
					}
				})
			}
		})
	}
}

// Compare the actual JVM check targets and ordered invocation tuples in every
// authored method. Ignore only immediately repeated checks to the same target:
// the first has already checked the unchanged stack reference and resolved the
// same class in this defining loader. Any intervening instruction ends that
// equivalence. New/narrower checks, changed overloads and invoke kinds remain
// observable; source spelling never supplies their binding oracle.
func arrayProducerCheckedInvocations(t *testing.T, raw []byte) string {
	t.Helper()
	object, err := Parse(bytes.Clone(raw))
	if err != nil {
		t.Fatal(err)
	}
	rows := []string{}
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		decoder := core.NewDecompiler(methodCodeBytes(method), func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
		if err := decoder.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		parts := []string{name + descriptor}
		previousCheck := ""
		for _, op := range constructorMotionOps(decoder) {
			code := op.Instr.OpCode
			if code == core.OP_CHECKCAST {
				target, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data))
				if !known {
					t.Fatal("unknown original/rebuilt array check target")
				}
				if previousCheck == target {
					continue
				}
				parts = append(parts, "checkcast:"+target)
				previousCheck = target
				continue
			}
			previousCheck = ""
			if code >= core.OP_INVOKEVIRTUAL && code <= core.OP_INVOKEINTERFACE {
				if len(op.Data) < 2 {
					t.Fatal("truncated invocation")
				}
				index := core.Convert2bytesToInt(op.Data[:2])
				if index == 0 || int(index) > len(object.ConstantPool) {
					t.Fatal("unknown invocation reference")
				}
				var member ConstantMemberrefInfo
				switch entry := object.ConstantPool[index-1].(type) {
				case *ConstantMethodrefInfo:
					member = entry.ConstantMemberrefInfo
				case *ConstantInterfaceMethodrefInfo:
					member = entry.ConstantMemberrefInfo
				default:
					t.Fatal("unexpected invocation reference")
				}
				owner, known := sourceBridgeClassName(object, member.ClassIndex)
				if !known {
					t.Fatal("unknown invocation owner")
				}
				n, d := getNameAndType(object.ConstantPool, member.NameAndTypeIndex)
				parts = append(parts, fmt.Sprintf("%02x:%s.%s%s", code, owner, n, d))
			}
		}
		rows = append(rows, strings.Join(parts, " "))
	}
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

func arrayProducerOriginalNarrowing(t *testing.T, raw []byte, name, parameters string) bool {
	t.Helper()
	object, err := Parse(bytes.Clone(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range object.Methods {
		n, _ := sourceBridgeUTF8(object, method.NameIndex)
		d, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		if n != name || !strings.HasPrefix(d, parameters) {
			continue
		}
		decoder := core.NewDecompiler(methodCodeBytes(method), func(i int) values.JavaValue { return GetValueFromCP(object.ConstantPool, i) })
		if err := decoder.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		checks, narrow := 0, false
		for _, op := range constructorMotionOps(decoder) {
			if op.Instr.OpCode == core.OP_CHECKCAST {
				target, known := sourceBridgeClassName(object, core.Convert2bytesToInt(op.Data))
				if !known || target != "[Ljava/lang/Object;" && target != "[Ljava/lang/String;" {
					t.Fatalf("unexpected authored original array check: %q", target)
				}
				checks++
				narrow = target == "[Ljava/lang/String;"
			}
		}
		if checks != 1 {
			t.Fatalf("authored original %s has %d checks", name, checks)
		}
		return narrow
	}
	t.Fatal("authored original array consumer absent", name)
	return false
}

func TestAdversarialArrayResultSourceTextCannotChangeCheckedViews(t *testing.T) {
	for _, source := range []string{
		`class C{String scope="assertj";Object run(Object value){return selected((String[])(convertToValues(value)));}}`,
		`class C{String data="assertj (String[])(convertToPayload(payload))";}`,
	} {
		if got := fixAssertjRemainingReconstructs(source); got != source {
			t.Fatalf("class text changed an original array check, overload view or application data:\nbefore %s\nafter %s", source, got)
		}
	}
	for _, variable := range []string{"var1", "var2"} {
		for _, argument := range []string{variable, "(Object)(" + variable + ")"} {
			source := `class IpSubnetFilter{Object run(){return Arrays.asList(((Object[])(ObjectUtil.checkNotNull(` + argument + `,"rules"))));}}`
			if got := fixNettyRemainingReconstructs(source); got != source {
				t.Fatalf("class spelling cannot narrow an original array check:\nbefore %s\nafter %s", source, got)
			}
		}
	}
}
