package javaclassparser

import (
	"strings"
	"testing"
)

func TestR06ConcatConversionTimingAndCaptureSnapshots(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "EvaluationProof", map[string]string{
		"EvaluationProof.java": `import java.util.function.IntSupplier;
public class EvaluationProof {
 static String trace="";
 static int n;
 static Object object(){trace+="E";return new ConversionProof();}
 static int operand(){trace+="B";return ++n;}
 static int capture(int x){int captured=x; IntSupplier saved=()->captured; x=99; return saved.getAsInt();}
 public static void main(String[] a){
  try{String value=""+object()+operand();System.out.println(value);}catch(IllegalStateException e){System.out.println(trace);System.out.println(n);}
  System.out.println(capture(7));
  int y=7;System.out.println(""+y+(++y));
 }
}`,
		"ConversionProof.java": `public class ConversionProof {public String toString(){EvaluationProof.trace+="C";throw new IllegalStateException("stop");}}`,
	})
	if strings.TrimSpace(original) != "EC\n0\n7\n78" {
		t.Fatalf("fixture trace changed: %q", original)
	}
	t04RoundTripModes(t, "17", "EvaluationProof", original, classes, func(t *testing.T, src string) {})
}

func TestR06ConstructorArgumentOrder(t *testing.T) {
	original, classes := t04CompileRun(t, "17", "CtorMain", map[string]string{
		"CtorBase.java": `public class CtorBase { public CtorBase(String s){System.out.println(s);} }`,
		"CtorMain.java": `public class CtorMain extends CtorBase { public static String take(String s){System.out.print(s);return s;} public CtorMain(){super(take("L")+take("R"));} public static void main(String[] a){new CtorMain();} }`,
	})
	if strings.TrimSpace(original) != "LRLR" {
		t.Fatalf("bad fixture oracle: %q", original)
	}
	t04RoundTripModes(t, "17", "CtorMain", original, classes, func(t *testing.T, s string) {})
}
