package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The JVM does not enforce a source Exceptions attribute. Compile a trusted
// program and remove only caller declarations, retaining every instruction,
// handler, frame and callee declaration. The verified JVM remains the oracle.
func TestAdversarialOriginalCheckedEscapeIdentityRoundTrip(t *testing.T) {
	javac, java := t04Tools(t)
	const source = `
import java.io.*;
interface EscapeBoundary {Object invoke(int mode)throws IOException;}
class OriginalEscapeActions {
 static final IOException checked=new IOException("checked");static final RuntimeException runtime=new IllegalStateException("runtime");static final AssertionError error=new AssertionError("error");static int trace;static boolean failBase;
 static Object action(int mode)throws IOException{trace=trace*10+2;if(mode==1)throw checked;if(mode==2)throw runtime;if(mode==3)throw error;return Integer.valueOf(mode);}
 static OriginalEscapeActions receiver(boolean missing){trace=trace*10+1;return missing?null:new OriginalEscapeActions();}
 Object instance(int mode)throws IOException{return action(mode);}
}
class OriginalEscapeBase {OriginalEscapeBase(){OriginalEscapeActions.trace=OriginalEscapeActions.trace*10+9;if(OriginalEscapeActions.failBase)throw OriginalEscapeActions.runtime;}}
public class OriginalCheckedEscapeReview extends OriginalEscapeBase implements EscapeBoundary {
 public OriginalCheckedEscapeReview(){}
 OriginalCheckedEscapeReview(int mode)throws IOException{OriginalEscapeActions.action(mode);}
 OriginalCheckedEscapeReview(int mode,boolean delegated)throws IOException{this();OriginalEscapeActions.action(mode);}
 OriginalCheckedEscapeReview(IOException failure)throws IOException{throw failure;}
 public Object invoke(int jdec$escape$0)throws IOException{return OriginalEscapeActions.action(jdec$escape$0);}
 Object receiverCall(int mode,boolean missing)throws IOException{return OriginalEscapeActions.receiver(missing).instance(mode);}
 Object cleanupCall(int mode)throws IOException{try{return OriginalEscapeActions.action(mode);}finally{OriginalEscapeActions.trace=OriginalEscapeActions.trace*10+3;}}
 Object protectedCall(int mode){try{return OriginalEscapeActions.action(mode);}catch(IOException error){return error;}}
 Object declaredCall(int mode)throws IOException{return OriginalEscapeActions.action(mode);}
 static void report(Object value,Throwable caught){System.out.println(String.valueOf(value)+":"+(caught==OriginalEscapeActions.checked)+":"+(caught==OriginalEscapeActions.runtime)+":"+(caught==OriginalEscapeActions.error)+":"+(caught==null?"none":caught.getClass().getName())+":"+OriginalEscapeActions.trace);}
 public static void main(String[]args){OriginalCheckedEscapeReview owner=new OriginalCheckedEscapeReview();for(int kind=0;kind<8;kind++)for(int mode=0;mode<4;mode++){OriginalEscapeActions.trace=0;Object value=null;Throwable caught=null;try{if(kind==0)value=owner.invoke(mode);if(kind==1)value=owner.receiverCall(mode,false);if(kind==2)value=owner.cleanupCall(mode);if(kind==3)value=owner.protectedCall(mode);if(kind==4)value=owner.declaredCall(mode);if(kind==5){new OriginalCheckedEscapeReview(mode);value="constructor";}if(kind==6){new OriginalCheckedEscapeReview(mode,true);value="delegate";}if(kind==7)new OriginalCheckedEscapeReview(mode==0?null:OriginalEscapeActions.checked);}catch(Throwable failure){caught=failure;}report(value,caught);}OriginalEscapeActions.trace=0;try{owner.receiverCall(0,true);}catch(Throwable failure){report(null,failure);}for(boolean delegated:new boolean[]{false,true}){OriginalEscapeActions.trace=0;OriginalEscapeActions.failBase=true;try{if(delegated)new OriginalCheckedEscapeReview(0,true);else new OriginalCheckedEscapeReview(0);}catch(Throwable failure){report(null,failure);}OriginalEscapeActions.failBase=false;}}
}`
	for _, debug := range []string{"-g", "-g:none"} {
		dir := t.TempDir()
		file := filepath.Join(dir, "OriginalCheckedEscapeReview.java")
		if err := os.WriteFile(file, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, file).CombinedOutput(); err != nil {
			t.Fatalf("original compile: %v\n%s", err, out)
		}
		want := t04RunJava(t, java, dir, "OriginalCheckedEscapeReview")
		classFile := filepath.Join(dir, "OriginalCheckedEscapeReview.class")
		raw, err := os.ReadFile(classFile)
		if err != nil {
			t.Fatal(err)
		}
		object, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		changed := 0
		for _, method := range object.Methods {
			name, _ := object.getUtf8(method.NameIndex)
			if name != "invoke" && name != "receiverCall" && name != "cleanupCall" && name != "<init>" {
				continue
			}
			attrs := method.Attributes[:0]
			for _, attribute := range method.Attributes {
				if _, ok := attribute.(*ExceptionsAttribute); ok {
					changed++
					continue
				}
				attrs = append(attrs, attribute)
			}
			method.Attributes = attrs
		}
		if changed != 6 {
			t.Fatalf("expected six caller-only declaration removals, got%d", changed)
		}
		raw = object.Bytes()
		if err := os.WriteFile(classFile, raw, 0644); err != nil {
			t.Fatal(err)
		}
		if got := t04RunJava(t, java, dir, "OriginalCheckedEscapeReview"); got != want {
			t.Fatalf("Exceptions mutation changed verified JVM oracle: got%q want%q", got, want)
		}
		resolve := func(name string) ([]byte, bool) {
			bytes, err := os.ReadFile(filepath.Join(dir, name+".class"))
			return bytes, err == nil
		}
		for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
			var result DecompileResult
			if mode == "legacy" {
				result.Source, err = DecompileWithResolver(raw, resolve)
			} else {
				result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
			}
			if err != nil {
				t.Fatal(err)
			}
			rebuilt := t.TempDir()
			path := filepath.Join(rebuilt, "OriginalCheckedEscapeReview.java")
			if err := os.WriteFile(path, []byte(result.Source), 0644); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", dir, "-d", rebuilt, path).CombinedOutput(); err != nil {
				t.Fatalf("rebuild %s/%s: %v\n%s\n%s", mode, debug, err, out, result.Source)
			}
			if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+dir, "OriginalCheckedEscapeReview"); got != want {
				t.Fatalf("%s/%s got%q want%q\n%s", mode, debug, got, want, result.Source)
			}
		}
	}
}
