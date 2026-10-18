package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The retry preparation and resend have distinct protected intervals. The
// same IOException type occurs in three handlers; neither nesting a source
// try nor reusing its JVM local slot can remove an alternative from another
// handler. The unchanged caller observes catch selection, exception identity,
// effects and monitor release on every path.
const retryMultiCatchFixture = `class RetryBoundaryEffects {
 static String trace="";
 static final java.io.IOException first=new java.io.IOException("first"), later=new java.io.IOException("later");
 static final ReflectiveOperationException reflect=new ReflectiveOperationException("reflect");
 static final RuntimeException unchecked=new UnsupportedOperationException("unchecked");
 static void mark(Object owner,String value){if(!Thread.holdsLock(owner))throw new AssertionError("lost monitor");trace+=value;}
 static void first(Object owner,int mode)throws java.io.IOException{mark(owner,"F");if(mode!=0)throw first;}
 static void close(Object owner,int mode)throws ReflectiveOperationException{mark(owner,"C");if(mode==1)throw reflect;if(mode==4)throw unchecked;}
 static void reconnect(Object owner,int mode)throws java.io.IOException{mark(owner,"R");if(mode==2)throw later;}
 static void send(Object owner,int mode)throws java.io.IOException{mark(owner,"S");if(mode==3||mode==6)throw later;}
 static void resendCaught(Object owner,int mode)throws java.io.IOException{mark(owner,"N");if(mode==6)throw later;}
}
public class RetryBoundaryOwner {
 void run(int mode,boolean retry)throws java.io.IOException{synchronized(this){
  try{RetryBoundaryEffects.first(this,mode);}catch(java.io.IOException original){if(retry){
   try{RetryBoundaryEffects.close(this,mode);RetryBoundaryEffects.reconnect(this,mode);}
   catch(ReflectiveOperationException|java.io.IOException recovery){RetryBoundaryEffects.mark(this,"O");throw new IllegalStateException("restore",original);}
   try{RetryBoundaryEffects.send(this,mode);}
   catch(java.io.IOException resend){RetryBoundaryEffects.resendCaught(this,mode);throw new IllegalArgumentException("resend",original);}
  }}
 }}
}
class RetryBoundaryDriver {
 public static void main(String[]args){int rows=0;for(int mode=0;mode<7;mode++)for(boolean retry:new boolean[]{false,true}){
  RetryBoundaryOwner owner=new RetryBoundaryOwner();RetryBoundaryEffects.trace="";Throwable failure=null;
  try{owner.run(mode,retry);}catch(Exception actual){failure=actual;}
  String expected=!retry||mode==0?"F":mode==1?"FCO":mode==2?"FCRO":mode==3||mode==6?"FCRSN":mode==4?"FC":"FCRS";
  if(!RetryBoundaryEffects.trace.equals(expected)||Thread.holdsLock(owner))throw new AssertionError("effects/monitor "+mode+":"+RetryBoundaryEffects.trace);
  if(retry&&mode>0&&mode<4){if(failure==null||failure.getCause()!=RetryBoundaryEffects.first||!(mode==3?failure instanceof IllegalArgumentException:failure instanceof IllegalStateException))throw new AssertionError("wrong catch/cause");}
  else if(retry&&mode==4){if(failure!=RetryBoundaryEffects.unchecked)throw new AssertionError("unchecked identity");}
  else if(retry&&mode==6){if(failure!=RetryBoundaryEffects.later)throw new AssertionError("handler exception was caught by a former region",failure);}
  else if(failure!=null)throw new AssertionError("new exception",failure);
  rows++;
 }System.out.println(rows+":retry:multicatch:identity:effects:monitor");}
}`

func TestAdversarialRetryMultiCatchKeepsOriginalHandlerMembership(t *testing.T) {
	javac, java := t04Tools(t)
	for _, owner := range []string{"RetryBoundaryOwner", "AlternateRecoveryScope"} {
		t.Run(owner, func(t *testing.T) {
			fixture := strings.ReplaceAll(retryMultiCatchFixture, "RetryBoundaryOwner", owner)
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(debug, func(t *testing.T) {
					files := nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
					write := func(dir, name string, raw []byte) {
						t.Helper()
						if err := os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					original := t.TempDir()
					for name, raw := range files {
						write(original, name, raw)
					}
					oracle := t04RunJava(t, java, original, "RetryBoundaryDriver")
					if oracle != "14:retry:multicatch:identity:effects:monitor\n" {
						t.Fatal("original JVM", oracle)
					}
					for _, mode := range []DecompileMode{Precision, Compatibility} {
						t.Run(string(mode), func(t *testing.T) {
							result, err := DecompileWithOptions(files[owner+".class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: func(name string) ([]byte, bool) {
								raw, known := files[name+".class"]
								return append([]byte(nil), raw...), known
							}})
							if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
								t.Fatalf("source: %v status=%s\n%s", err, result.Status, result.Source)
							}
							if mode == Precision && strings.Contains(result.Source, "if(false)throw") {
								t.Fatal("typed reconstruction depended on an unreachable synthetic throw")
							}
							out := t.TempDir()
							for name, raw := range files {
								if name != owner+".class" {
									write(out, name, raw)
								}
							}
							write(out, owner+".java", []byte(result.Source))
							if data, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, filepath.Join(out, owner+".java")).CombinedOutput(); err != nil {
								t.Fatalf("rebuilt: %v\n%s\n%s", err, data, result.Source)
							}
							if got := t04RunJava(t, java, out, "RetryBoundaryDriver"); got != oracle {
								t.Fatalf("rebuilt JVM=%q want=%q", got, oracle)
							}
							rebuilt, err := os.ReadFile(filepath.Join(out, owner+".class"))
							if err != nil {
								t.Fatal(err)
							}
							if nativeBinaryShape(t, rebuilt) != nativeBinaryShape(t, files[owner+".class"]) {
								t.Fatal("original method/field table changed")
							}
							nativeLexicalExactSignatures(t, owner, files[owner+".class"], rebuilt)
						})
					}
				})
			}
		})
	}
}
