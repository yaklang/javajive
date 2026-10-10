package javaclassparser

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Compare the cast's semantic type and operand, allowing incidental parentheses.
// A catch VALUE must never be rendered as the source TYPE of its own payload.
func assertReviewedTargetExceptionPayload(t *testing.T, body string, includeError bool) {
	t.Helper()
	caught := requireReviewedPattern(t, body, `catch\s*\(InvocationTargetException\s+(\w+)\)`)[1]
	plain := strings.NewReplacer("(", "", ")", "", " ", "", "\t", "", "\n", "").Replace(body)
	for _, typ := range []string{"Exception", "Error"} {
		if typ == "Error" && !includeError {
			continue
		}
		if !strings.Contains(plain, caught+".getTargetExceptioninstanceof"+typ) || !strings.Contains(plain, "throw"+typ+caught+".getTargetException;") {
			t.Fatalf("lost original %s checkcast/ATHROW payload binding:\n%s", typ, body)
		}
	}
	if !strings.Contains(plain, "throw"+caught+";") {
		t.Fatalf("nonmatching target lost original InvocationTargetException identity:\n%s", body)
	}
	want := 2
	if includeError {
		want = 4
	}
	if strings.Count(body, ".getTargetException()") != want || strings.Contains(plain, "throw"+caught+caught+".getTargetException") {
		t.Fatalf("original target evaluations were replayed or catch value became a type:\n%s", body)
	}
}

// Resolve only locked original metadata. This test never loads a third-party
// class into the host JVM, and the target bytes must match the standalone seed.
func reviewedCategoryOriginalMetadata(t *testing.T, raw []byte) func(string) ([]byte, bool) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".m2/repository/junit/junit/4.13.2/junit-4.13.2.jar")
	jarBytes, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("locked native metadata JAR is not installed: " + path)
	}
	if err != nil {
		t.Fatal(err)
	}
	const lockedSHA = "8e495b634469d64fb8acfa3495a065cbacc8a0fff55ce1e31007be4c16dc57d3"
	if got := fmt.Sprintf("%x", sha256.Sum256(jarBytes)); got != lockedSHA {
		t.Fatalf("locked original junit4.13.2 metadata SHA changed: %s", got)
	}
	jar, err := zip.NewReader(bytes.NewReader(jarBytes), int64(len(jarBytes)))
	if err != nil {
		t.Fatal(err)
	}
	classes := make(map[string][]byte)
	for _, file := range jar.File {
		if !strings.HasSuffix(file.Name, ".class") {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		value, err := io.ReadAll(reader)
		reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		classes[strings.TrimSuffix(file.Name, ".class")] = value
	}
	if !bytes.Equal(raw, classes["org/junit/experimental/categories/CategoryFilterFactory"]) {
		t.Fatal("locked JAR target differs from the reviewed original seed")
	}
	return func(name string) ([]byte, bool) { value, ok := classes[name]; return value, ok }
}

func TestAdversarialCheckedTargetAndCallablePayloadIdentityRoundTrip(t *testing.T) {
	t.Setenv("JDEC_IDENT_SELF_CAST_OFF", "1")
	t.Setenv("JDEC_HARDJAR_SHAPE_OFF", "1")
	roundTripGenericFlow(t, "CheckedPayloadReview", `import java.lang.reflect.*;
class CheckedPayloadEffects {static int mode;static String trace;static final Exception checked=new Exception("checked");static final Error error=new AssertionError("error");static final Throwable foreign=new Throwable("foreign");static final InvocationTargetException wrapper=new InvocationTargetException(checked);}
public class CheckedPayloadReview {
 static Object invoke()throws Exception{CheckedPayloadEffects.trace+="I";if(CheckedPayloadEffects.mode==1)throw CheckedPayloadEffects.wrapper;if(CheckedPayloadEffects.mode==2)throw new InvocationTargetException(CheckedPayloadEffects.error);if(CheckedPayloadEffects.mode==3)throw new InvocationTargetException(CheckedPayloadEffects.foreign);return "normal";}
 static Object target(boolean errors)throws Exception{try{return invoke();}catch(InvocationTargetException caught){if(caught.getTargetException() instanceof Exception)throw (Exception)caught.getTargetException();if(errors&&caught.getTargetException() instanceof Error)throw (Error)caught.getTargetException();throw caught;}}
 static void evaluate()throws Throwable{CheckedPayloadEffects.trace+="V";if(CheckedPayloadEffects.mode==1)throw CheckedPayloadEffects.checked;if(CheckedPayloadEffects.mode==2)throw CheckedPayloadEffects.error;if(CheckedPayloadEffects.mode==3)throw CheckedPayloadEffects.foreign;}
 static Throwable call()throws Exception{try{CheckedPayloadEffects.trace+="L";evaluate();return null;}catch(Exception caught){throw caught;}catch(Throwable caught){return caught;}}
 public static void main(String[]args){for(int mode=0;mode<4;mode++){CheckedPayloadEffects.mode=mode;for(boolean errors:new boolean[]{false,true}){CheckedPayloadEffects.trace="";try{System.out.print(target(errors)+":");}catch(Throwable caught){System.out.print((caught==CheckedPayloadEffects.checked)+":"+(caught==CheckedPayloadEffects.error)+":"+(caught==CheckedPayloadEffects.wrapper)+":"+(caught instanceof InvocationTargetException)+":");}System.out.println(CheckedPayloadEffects.trace);}CheckedPayloadEffects.trace="";try{Throwable value=call();System.out.print((value==null)+":"+(value==CheckedPayloadEffects.error)+":"+(value==CheckedPayloadEffects.foreign)+":");}catch(Throwable caught){System.out.print((caught==CheckedPayloadEffects.checked)+":");}System.out.println(CheckedPayloadEffects.trace);}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialThrowFormalErasureIdentityRoundTrip(t *testing.T) {
	t.Setenv("JDEC_FIX_THROW_TYPEVAR_CAST_OFF", "1")
	roundTripGenericFlow(t, "ThrowFormalReview", `import java.io.*;
class ThrowFormalEffects{static int calls;static final IOException checked=new IOException("checked");static final Error error=new AssertionError("error");}
public class ThrowFormalReview{
 private static <R,T extends Throwable>R erased(Throwable value)throws T{ThrowFormalEffects.calls++;throw (T)value;}
 static Object call(Throwable value){return ThrowFormalReview.<Object,RuntimeException>erased(value);}
 public static void main(String[]args){for(Throwable value:new Throwable[]{ThrowFormalEffects.checked,ThrowFormalEffects.error,null}){ThrowFormalEffects.calls=0;try{Object result=call(value);System.out.println("wrong");}catch(Throwable caught){System.out.println((caught==value)+":"+(caught==ThrowFormalEffects.checked)+":"+(caught==ThrowFormalEffects.error)+":"+caught.getClass().getName()+":"+ThrowFormalEffects.calls);}}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialPrivateSerializationHookInheritanceRoundTrip(t *testing.T) {
	t.Setenv("JDEC_SERIAL_HOOK_PRIVATE_OFF", "1")
	t.Setenv("JDEC_NEST_PRIVATE_PACKAGE_OFF", "1")
	roundTripGenericFlow(t, "PrivateSerializationReview", `import java.io.*;import java.lang.reflect.*;
class SerializationHookEffects{static Object last;static int calls;}
class SerializationChild extends PrivateSerializationReview {SerializationChild(int value){super(value);}}
public class PrivateSerializationReview implements Serializable{
 final int value;PrivateSerializationReview(int value){this.value=value;}
 private Object readResolve(){SerializationHookEffects.last=this;SerializationHookEffects.calls++;return this;}
 static Object copy(Object value)throws Exception{ByteArrayOutputStream bytes=new ByteArrayOutputStream();ObjectOutputStream output=new ObjectOutputStream(bytes);output.writeObject(value);output.close();ObjectInputStream input=new ObjectInputStream(new ByteArrayInputStream(bytes.toByteArray()));Object result=input.readObject();input.close();return result;}
 public static void main(String[]args)throws Exception{System.out.println(Modifier.isPrivate(PrivateSerializationReview.class.getDeclaredMethod("readResolve").getModifiers()));for(PrivateSerializationReview value:new PrivateSerializationReview[]{new PrivateSerializationReview(7),new SerializationChild(9)}){SerializationHookEffects.last=null;SerializationHookEffects.calls=0;Object result=copy(value);System.out.println((result!=value)+":"+(result==SerializationHookEffects.last)+":"+((PrivateSerializationReview)result).value+":"+SerializationHookEffects.calls+":"+result.getClass().getName());}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialCheckedInterfaceFilterWrapperIdentityRoundTrip(t *testing.T) {
	t.Setenv("JDEC_IDENT_SELF_CAST_OFF", "1")
	roundTripGenericFlow(t, "CheckedInterfaceFilterReview", `interface FilterContract{Object create(String name)throws FilterFailure;}
class FilterFailure extends Exception{FilterFailure(Exception cause){super(cause);}}
class FilterEffects{static String trace;static int mode;static final ClassNotFoundException missing=new ClassNotFoundException("original");static final RuntimeException runtime=new IllegalArgumentException("original-runtime");static final Object marker=new Object();}
public class CheckedInterfaceFilterReview implements FilterContract{
 private Object parse(String name)throws ClassNotFoundException{FilterEffects.trace+="P";if(FilterEffects.mode==1)throw FilterEffects.missing;if(FilterEffects.mode==2)throw FilterEffects.runtime;return FilterEffects.marker;}
 protected Object build(Object parsed){FilterEffects.trace+="B";return parsed;}
 public Object create(String name)throws FilterFailure{try{return build(parse(name));}catch(ClassNotFoundException caught){throw new FilterFailure(caught);}}
 public static void main(String[]args){FilterContract receiver=new CheckedInterfaceFilterReview();for(int mode=0;mode<3;mode++){FilterEffects.mode=mode;FilterEffects.trace="";try{System.out.print((receiver.create("item")==FilterEffects.marker)+":");}catch(Throwable caught){System.out.print((caught instanceof FilterFailure)+":"+(caught.getCause()==FilterEffects.missing)+":"+(caught==FilterEffects.runtime)+":");}System.out.println(FilterEffects.trace);}}
}`, Precision, Compatibility, "legacy")
}

// Keep a separate explicit negative for an unknown inherited namespace. The
// complete native path is tested in TestCategoryFilterFactorySelfCastIsLoadBearing.
func TestCategoryFilterRequiresOriginalInheritedMetadata(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/CategoryFilterFactory.class")
	if err != nil {
		t.Fatal(err)
	}
	result, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatal(err)
	}
	const member = "createFilter(Lorg/junit/runner/FilterFactoryParams;)Lorg/junit/runner/manipulation/Filter;"
	if len(result.StubMethods) != 1 || result.StubMethods[0] != member || !strings.Contains(result.Source, "checked escape helper requires complete inherited member names") {
		t.Fatalf("missing inherited namespace must explicitly refuse the original member: %#v\n%s", result.StubMethods, result.Source)
	}
	if regexp.MustCompile(`catch\([^)]*\)`).MatchString(reviewedControlBody(t, result.Source, `public\s+Filter\s+createFilter\(`)) {
		t.Fatal("unsupported member invented a partial catch body")
	}
}
