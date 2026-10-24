package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The driver and effect producers remain ORIGINAL classfiles. Only the
// consumer is rebuilt, so a wrong handler/receiver rewrite cannot rewrite
// its own oracle. Each original driver proves values, exact exception/cause
// identity, null/cast/array failures, integer overflow and effect prefixes.
const protectedReturnFixture = `
class BoundaryEffects {
 static int trace;
 static final RuntimeException before=new IllegalArgumentException("before"),after=new IllegalStateException("after");
 static void mark(int n){trace=trace*7+n;}
 static int read(int n){mark(3);if(n==0)throw after;return n^103;}
}
class BoundaryPost {
 volatile int word;BoundaryPost(int n){BoundaryEffects.mark(3);if(n==0)throw BoundaryEffects.after;word=n;}
 int read(int n){return BoundaryEffects.read(n);}
 int finish(){BoundaryEffects.mark(4);return word+7;}
}
class BoundaryConsumer {
 METHODS
}
class BoundaryDriver {
 public static void main(String[]args){int rows=0;BoundaryPost post=new BoundaryPost(17);
 for(int repeat=0;repeat<3;repeat++)for(int shape=0;shape<7;shape++)for(int mode:new int[]{0,1})for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int receiverMode=0;receiverMode<3;receiverMode++)for(boolean nilArray:new boolean[]{false,true}){
 Object receiver=receiverMode==0?post:receiverMode==1?null:"wrong-type";int[]array=nilArray?null:new int[]{11,13,17};BoundaryEffects.trace=0;Throwable failure=null;int result=0;
 try{switch(shape){case 0:result=BoundaryConsumer.call(mode,n,receiver,array);break;case 1:result=BoundaryConsumer.array(mode,n,receiver,array);break;case 2:result=BoundaryConsumer.cast(mode,n,receiver,array);break;case 3:result=BoundaryConsumer.allocation(mode,n,receiver,array);break;case 4:result=BoundaryConsumer.field(mode,n,receiver,array);break;case 5:result=BoundaryConsumer.loop(mode,n,receiver,array);break;default:result=BoundaryConsumer.nested(mode,n,receiver,array);}}
 catch(Throwable caught){failure=caught;}
 if(mode!=0){if(failure==null||failure.getClass()!=RuntimeException.class||failure.getCause()!=BoundaryEffects.before||BoundaryEffects.trace!=9)throw new AssertionError("original catch wrapper/cause/abrupt effects");}
 else {
 int expectedTrace=9;Class expectedClass=null;Throwable expectedObject=null;int expectedValue=0;
 if(shape==0){expectedTrace=66;if(n==0)expectedObject=BoundaryEffects.after;else expectedValue=n^103;}
 else if(shape==1){if(nilArray)expectedClass=NullPointerException.class;else if((n&3)==3)expectedClass=ArrayIndexOutOfBoundsException.class;else expectedValue=array[n&3];}
 else if(shape==2){if(receiverMode==1)expectedClass=NullPointerException.class;else if(receiverMode==2)expectedClass=ClassCastException.class;else{expectedTrace=66;if(n==0)expectedObject=BoundaryEffects.after;else expectedValue=n^103;}}
 else if(shape==3){expectedTrace=66;if(n==0)expectedObject=BoundaryEffects.after;else{expectedTrace=466;expectedValue=n+7;}}
 else if(shape==4){if(receiverMode==1)expectedClass=NullPointerException.class;else if(receiverMode==2)expectedClass=ClassCastException.class;else expectedValue=17;}
 else if(shape==5){expectedTrace=n==-1?3153:66;if(n==-1||n==0)expectedObject=BoundaryEffects.after;else expectedValue=n^103;}
 else {expectedTrace=n==0?466:66;expectedValue=n==0?77:n^103;}
 if(BoundaryEffects.trace!=expectedTrace)throw new AssertionError("normal continuation effect domain "+shape+":"+BoundaryEffects.trace);
 if(expectedObject!=null){if(failure!=expectedObject)throw new AssertionError("normal-tail exception identity "+shape);}
 else if(expectedClass!=null){if(failure==null||failure.getClass()!=expectedClass||failure.getCause()!=null)throw new AssertionError("normal-tail JVM failure must not enter earlier handler "+shape);}
 else if(failure!=null||result!=expectedValue)throw new AssertionError("normal continuation value "+shape);
 }
 rows++;}
 System.out.println(rows+":outside-return:handler:identity:effects");
 }
}
`

func protectedReturnSources() string {
	methods := ""
	for _, row := range []struct{ name, expression string }{
		{"call", "BoundaryEffects.read(n)"},
		{"array", "array[n&3]"},
		{"cast", "((BoundaryPost)receiver).read(n)"},
		{"allocation", "new BoundaryPost(n).finish()"},
		{"field", "((BoundaryPost)receiver).word"},
	} {
		methods += "static int " + row.name + "(int mode,int n,Object receiver,int[]array){BoundaryEffects.mark(1);try{BoundaryEffects.mark(2);if(mode!=0)throw BoundaryEffects.before;}catch(RuntimeException caught){throw new RuntimeException(caught);}return " + row.expression + ";}\n"
	}
	methods += `static int loop(int mode,int n,Object receiver,int[]array){int turn=0;while(turn++<2){BoundaryEffects.mark(1);try{BoundaryEffects.mark(2);if(mode!=0)throw BoundaryEffects.before;if(n==-1)continue;}catch(RuntimeException caught){throw new RuntimeException(caught);}return BoundaryEffects.read(n);}return BoundaryEffects.read(n+1);}
static int nested(int mode,int n,Object receiver,int[]array){try{BoundaryEffects.mark(1);try{BoundaryEffects.mark(2);if(mode!=0)throw BoundaryEffects.before;}catch(RuntimeException caught){throw new RuntimeException(caught);}return BoundaryEffects.read(n);}catch(IllegalStateException after){BoundaryEffects.mark(4);if(after!=BoundaryEffects.after)throw new AssertionError("original outer catch identity");return n+77;}}
`
	return strings.Replace(protectedReturnFixture, "METHODS", methods, 1)
}

func TestAdversarialProtectedReturnKeepsOriginalHandlerDomain(t *testing.T) {
	javac, java := t04Tools(t)
	for _, rename := range []string{"Boundary", "Uncovered"} {
		t.Run(rename, func(t *testing.T) {
			source := strings.ReplaceAll(protectedReturnSources(), "Boundary", rename)
			for _, compiler := range []string{"modern", "javac8"} {
				t.Run(compiler, func(t *testing.T) {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(debug, func(t *testing.T) {
							var files map[string][]byte
							if compiler == "javac8" {
								files = localGetterCompileJavac8(t, map[string]string{rename + "Consumer.java": source}, debug)
							} else {
								files = nativeCompileDebugClasses(t, source, debug)
							}
							original := t.TempDir()
							support := t.TempDir()
							for name, raw := range files {
								if e := os.WriteFile(filepath.Join(original, name), raw, 0600); e != nil {
									t.Fatal(e)
								}
								if name != rename+"Consumer.class" {
									if e := os.WriteFile(filepath.Join(support, name), raw, 0600); e != nil {
										t.Fatal(e)
									}
								}
							}
							want := t04RunJava(t, java, original, rename+"Driver")
							if want != "1260:outside-return:handler:identity:effects\n" {
								t.Fatal("independent original oracle", want)
							}
							resolve := func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
							for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
								t.Run(string(mode), func(t *testing.T) {
									var generated string
									var err error
									if mode == "legacy" {
										generated, err = DecompileWithResolver(bytes.Clone(files[rename+"Consumer.class"]), resolve)
									} else {
										var result DecompileResult
										result, err = DecompileWithOptions(bytes.Clone(files[rename+"Consumer.class"]), DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
										generated = result.Source
									}
									if err != nil || strings.Contains(generated, DecompileStubMarker) {
										t.Fatalf("consumer source: %v\n%s", err, generated)
									}
									out := t.TempDir()
									file := filepath.Join(out, rename+"Consumer.java")
									if e := os.WriteFile(file, []byte(generated), 0600); e != nil {
										t.Fatal(e)
									}
									if raw, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", support, "-d", out, file).CombinedOutput(); e != nil {
										t.Fatalf("rebuilt: %v\n%s\n%s", e, raw, generated)
									}
									if got := t04RunJava(t, java, out+string(os.PathListSeparator)+support, rename+"Driver"); got != want {
										t.Fatalf("candidate output=%q want=%q\n%s", got, want, generated)
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
