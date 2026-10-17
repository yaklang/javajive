package javaclassparser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// One factory is retained and activated repeatedly, and two factories carry
// independent supplier environments. The unchanged caller detects eager calls,
// cached results, swapped/merged captures, lost exception identity and extra
// evaluation. Exact original class/field/constructor ABI remains a separate gate.
const captureActivationInput = `
class CaptureInput {
 interface View {Object retained();}
 int calls;final RuntimeException failed=new IllegalStateException("supplier");final boolean fail;String trace="";
 CaptureInput(boolean fail){this.fail=fail;}
 java.util.function.Supplier<Object> supplier(){return ()->{calls++;trace+="S";if(fail&&calls==1)throw failed;return new Object();};}
}
`
const captureActivationDriver = `class CaptureDriver {public static void main(String[]args){int rows=0;for(boolean failure:new boolean[]{false,true})for(int n=0;n<8;n++){
 CaptureInput left=new CaptureInput(failure),right=new CaptureInput(false);java.util.function.Function<Object,CaptureInput.View>a=CaptureTransport.bind(left.supplier()),b=CaptureTransport.bind(right.supplier());
 if(left.calls!=0||right.calls!=0)throw new AssertionError("premature bind");CaptureInput.View x=a.apply(new Object()),y=a.apply(null),z=b.apply(new Object());if(left.calls!=0||right.calls!=0)throw new AssertionError("premature factory");
 Object first=null;try{first=x.retained();if(failure)throw new AssertionError("missing supplier failure");}catch(RuntimeException e){if(!failure||e!=left.failed)throw new AssertionError("failure identity",e);}
 Object second=y.retained(),third=x.retained(),other=z.retained();if(second==null||third==null||other==null||second==third||second==other||third==other||first!=null&&(first==second||first==third||first==other)||left.calls!=3||right.calls!=1||!left.trace.equals("SSS")||!right.trace.equals("S"))throw new AssertionError("capture environment or snapshot identity");rows++;}
 if(rows!=16)throw new AssertionError("coverage");System.out.println("16:nested:capture:timing:activation:failure:identity");}}
`

func TestAdversarialCaptureEnvironmentActivationAndDeferredEvaluation(t *testing.T) {
	for _, binding := range []string{"alias", "channel"} {
		t.Run(binding, func(t *testing.T) {
			for _, shape := range []struct{ name, body string }{
				{"direct-anonymous", `static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){return input->new CaptureInput.View(){public Object retained(){return alias.get();}};}`},
				{"method-local-implementation", `static CaptureInput.View make(java.util.function.Supplier<Object> alias){class Local implements CaptureInput.View {public Object retained(){return alias.get();}}return new Local();}static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){return input->make(alias);}`},
			} {
				t.Run(shape.name, func(t *testing.T) {
					fixture := captureActivationInput + "class CaptureTransport {" + shape.body + "}\n" + captureActivationDriver
					fixture = strings.ReplaceAll(fixture, "alias", binding)
					testNativeIndependentFamilyFixture(t, fixture, []string{"CaptureTransport"}, "CaptureDriver", "16:nested:capture:timing:activation:failure:identity\n", nativeLexicalExactSignatures)
				})
			}
		})
	}
}

// These are deliberately authored wrong projections, not decompiler output.
// They make the caller's ability to detect the targeted defects load-bearing.
func TestAdversarialCaptureEnvironmentOracleKillsInvalidProjections(t *testing.T) {
	_, java := t04Tools(t)
	run := func(body string) (string, error) {
		fixture := captureActivationInput + "class CaptureTransport {" + body + "}" + captureActivationDriver
		files := nativeCompileDebugClasses(t, fixture, "none")
		dir := t.TempDir()
		for name, raw := range files {
			path := filepath.Join(dir, filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, java, "-Xverify:all", "-cp", dir, "CaptureDriver").CombinedOutput()
		if ctx.Err() != nil {
			t.Fatal("authored oracle timed out", ctx.Err())
		}
		return string(out), err
	}
	const correct = `static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){return input->new CaptureInput.View(){public Object retained(){return alias.get();}};}`
	if out, err := run(correct); err != nil || out != "16:nested:capture:timing:activation:failure:identity\n" {
		t.Fatalf("independent oracle control failed: %v\n%s", err, out)
	}
	for _, mutant := range []struct{ name, body, observation string }{
		{"eager bind", `static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){final Object cached=alias.get();return input->new CaptureInput.View(){public Object retained(){return cached;}};}`, "premature bind"},
		{"eager factory", `static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){return input->{final Object cached=alias.get();return new CaptureInput.View(){public Object retained(){return cached;}};};}`, "premature factory"},
		{"duplicate evaluation", `static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){return input->new CaptureInput.View(){public Object retained(){alias.get();return alias.get();}};}`, "capture environment or snapshot identity"},
		{"merged environments", `static java.util.function.Supplier<Object> environment;static java.util.function.Function<Object,CaptureInput.View> bind(java.util.function.Supplier<Object> alias){environment=alias;return input->new CaptureInput.View(){public Object retained(){return environment.get();}};}`, "capture environment or snapshot identity"},
	} {
		t.Run(mutant.name, func(t *testing.T) {
			out, err := run(mutant.body)
			if err == nil || !strings.Contains(out, "java.lang.AssertionError: "+mutant.observation) {
				t.Fatalf("wrong projection survived or failed outside its oracle: %v\n%s", err, out)
			}
		})
	}
}
