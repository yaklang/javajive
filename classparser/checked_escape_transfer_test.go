package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

// The sealed renderer contains only a transfer keyword and identifier. Legacy
// names, annotations and text callbacks cannot prove absence of a throw operand.
func TestCheckedEscapeTypedHandlerSealedTransfer(t *testing.T) {
	for _, kind := range []string{"break", "continue"} {
		for _, variant := range []string{"plain", "labelled", "opaque", "forged annotation", "invalidated label", "added throw operand", "unpermitted throw", "permitted throw", "catchall"} {
			t.Run(kind+"/"+variant, func(t *testing.T) {
				leaf := statements.NewSourceTransferStatement(kind, "")
				want := true
				permitted := map[int]bool{}
				handler := statements.CatchHandler{EntryPC: 20}
				switch variant {
				case "labelled":
					leaf = statements.NewSourceTransferStatement(kind, "OUTER")
				case "opaque", "forged annotation":
					leaf = statements.NewCustomStatement(func(*class_context.ClassContext) string { return kind }, nil)
					if variant == "forged annotation" {
						leaf.Name = kind
						leaf.LoopTransferKind = kind
					}
					want = false
				case "invalidated label":
					leaf.LoopTargetLabel = "x;hidden()"
					want = false
				case "added throw operand":
					leaf.ThrownValue = values.JavaNull
					want = false
				case "unpermitted throw", "permitted throw":
					leaf = statements.NewThrowStatement(values.JavaNull)
					leaf.HasOriginPC = true
					leaf.OriginPC = 41
					permitted[41] = variant == "permitted throw"
					want = variant == "permitted throw"
				case "catchall":
					handler.CatchAll = true
					want = false
				}
				tr := statements.NewTryCatchStatement(nil, [][]statements.Statement{{leaf}})
				tr.Handlers = []statements.CatchHandler{handler}
				if got := typedAbsorbingHandlers([]statements.Statement{tr}, permitted)[20]; got != want {
					t.Fatalf("absorbs=%v want=%v", got, want)
				}
			})
		}
	}
}

// Original operations and the driver remain binary oracles. With the parent
// deliberately unavailable, a needless helper must not hide a lost catch proof.
func TestAdversarialCheckedCatchTransferWithoutHelperNamespace(t *testing.T) {
	const fixture = `
class CatchTransferParent {}
class CatchTransferOps {
 static int calls,limit,mode;static final RuntimeException runtime=new IllegalStateException("runtime");static final Error fatal=new AssertionError("fatal");
 static void attempt()throws InterruptedException {if(++calls<=limit)throw new InterruptedException();if(mode==1)throw runtime;if(mode==2)throw fatal;}
}
class CatchTransferOwner extends CatchTransferParent {
 static int retry(){int count=0;boolean interrupted=false;try{while(true){try{CatchTransferOps.attempt();return count;}catch(InterruptedException e){count++;interrupted=true;continue;}}}finally{if(interrupted)Thread.currentThread().interrupt();}}
 static int labelledRetry(){int count=0;boolean interrupted=false;try{OUTER:while(true){try{CatchTransferOps.attempt();return count;}catch(InterruptedException e){count++;interrupted=true;continue OUTER;}}}finally{if(interrupted)Thread.currentThread().interrupt();}}
 static int stop(){int count=0;boolean interrupted=false;try{OUTER:while(true){try{CatchTransferOps.attempt();return count;}catch(InterruptedException e){count++;interrupted=true;break OUTER;}}return count;}finally{if(interrupted)Thread.currentThread().interrupt();}}
}
public class CatchTransferDriver {public static void main(String[]args){int worlds=0;for(int kind=0;kind<3;kind++)for(int limit:new int[]{0,1,3})for(int mode=0;mode<3;mode++)for(boolean initial:new boolean[]{false,true}){
 Thread.interrupted();if(initial)Thread.currentThread().interrupt();CatchTransferOps.calls=0;CatchTransferOps.limit=limit;CatchTransferOps.mode=mode;Object result;
 try{result=Integer.valueOf(kind==0?CatchTransferOwner.retry():kind==1?CatchTransferOwner.labelledRetry():CatchTransferOwner.stop());}catch(Throwable e){result=e;}
 int count=kind==2&&limit>0?1:limit;Object expected=kind==2&&limit>0?Integer.valueOf(1):mode==1?CatchTransferOps.runtime:mode==2?CatchTransferOps.fatal:Integer.valueOf(limit);
 int calls=kind==2&&limit>0?1:limit+1;boolean interrupted=Thread.interrupted();
 if(CatchTransferOps.calls!=calls||interrupted!=(initial||count>0)||(expected instanceof Throwable?result!=expected:!expected.equals(result)))throw new AssertionError("catch transfer identity/order/finally");worlds++;
 }System.out.println(worlds+":catch-transfer:identity:finally");}}
`
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileSourceReleaseClasses(t, map[string]string{"CatchTransferDriver.java": fixture}, debug, "8")
			original := t.TempDir()
			for name, raw := range files {
				if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "CatchTransferDriver")
			if oracle != "54:catch-transfer:identity:finally\n" {
				t.Fatalf("original=%q", oracle)
			}
			resolve := func(name string) ([]byte, bool) {
				if name == "CatchTransferParent" {
					return nil, false
				}
				raw, ok := files[name+".class"]
				return raw, ok
			}
			for _, mode := range []DecompileMode{Precision, Compatibility} {
				t.Run(string(mode), func(t *testing.T) {
					result, err := DecompileWithOptions(files["CatchTransferOwner.class"], DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
					if err != nil || strings.Contains(result.Source, DecompileStubMarker) {
						t.Fatalf("source:%v\n%s", err, result.Source)
					}
					out := t.TempDir()
					for name, raw := range files {
						if name != "CatchTransferOwner.class" {
							if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
					}
					source := filepath.Join(out, "CatchTransferOwner.java")
					if err := os.WriteFile(source, []byte(result.Source), 0600); err != nil {
						t.Fatal(err)
					}
					if raw, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", out, "-d", out, source).CombinedOutput(); err != nil {
						t.Fatalf("rebuild:%v\n%s\n%s", err, raw, result.Source)
					}
					if got := t04RunJava(t, java, out, "CatchTransferDriver"); got != oracle {
						t.Fatalf("candidate=%q want=%q", got, oracle)
					}
					for name, raw := range files {
						if name == "CatchTransferOwner.class" {
							continue
						}
						got, err := os.ReadFile(filepath.Join(out, name))
						if err != nil || string(got) != string(raw) {
							t.Fatalf("oracle/dependency replaced: %s", name)
						}
					}
				})
			}
		})
	}
}
