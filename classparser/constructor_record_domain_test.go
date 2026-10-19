package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestAdversarialRecordCanonicalParametersUseComponentHeaderProjection(t *testing.T) {
	const source = `public record RecordNamedBinding(long amount,String token) {
 public RecordNamedBinding {if(token==null)token="nil";}
 public static void main(String[]args)throws Exception{
  int rows=0;for(long amount:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(String token:new String[]{null,"",new String("value")}){
   RecordNamedBinding value=new RecordNamedBinding(amount,token);if(value.amount()!=amount||!value.token().equals(token==null?"nil":token))throw new AssertionError("canonical entry/storage/component identity");rows++;
  }
  java.lang.reflect.Parameter[] parameters=RecordNamedBinding.class.getDeclaredConstructor(long.class,String.class).getParameters();if(parameters.length!=2||!parameters[0].getName().equals("amount")||!parameters[1].getName().equals("token")||!parameters[0].isNamePresent()||!parameters[1].isNamePresent())throw new AssertionError("component header parameter identity");System.out.println(rows+":record:component:canonical:names");
 }
}`
	oracle, files := t20CompileRun(t, "17", "RecordNamedBinding", map[string]string{"RecordNamedBinding.java": source})
	if oracle != "15:record:component:canonical:names\n" {
		t.Fatal("independent original record model", oracle)
	}
	raw := files["RecordNamedBinding"]
	for _, mandated := range []bool{false, true} {
		t.Run(map[bool]string{false: "compiler", true: "mandated canonical formals"}[mandated], func(t *testing.T) {
			d, method, descriptor := constructorNameGuardInput(t, raw)
			var table *UnparsedAttribute
			for _, a := range method.Attributes {
				if a, ok := a.(*UnparsedAttribute); ok && a.Name == "MethodParameters" {
					table = a
				}
			}
			if table == nil || len(table.Info) != 9 || table.Info[0] != 2 {
				t.Fatal("original canonical parameter table")
			}
			if mandated {
				// A separate legal classfile protocol exercises JDKs which
				// mark compiler-projected canonical formals as mandated. It
				// must not acquire the plain-constructor naming authority.
				for i := 0; i < 2; i++ {
					binary.BigEndian.PutUint16(table.Info[3+4*i:5+4*i], 0x8000)
				}
			}
			before := d.obj.Bytes()
			if names, known := d.originalOrdinaryMethodParameterNames(method, descriptor); !known || names != nil {
				t.Fatal("component-projected constructor treated as an ordinary source header", names, known)
			}
			if !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatal("record naming-domain query modified original input")
			}
		})
	}
	for _, mode := range []DecompileMode{Precision, Compatibility} {
		t.Run(string(mode), func(t *testing.T) {
			result, err := DecompileWithOptions(bytes.Clone(raw), DecompileOptions{Mode: mode, TargetSourceVersion: 17, EnvSnapshot: map[string]string{}})
			if err != nil || result.Status != "complete" || strings.Contains(result.Source, DecompileStubMarker) {
				t.Fatalf("canonical component source: %v status=%s\n%s", err, result.Status, result.Source)
			}
			t20RebuildRun(t, "17", "RecordNamedBinding", map[string]string{"RecordNamedBinding.java": result.Source}, oracle)
		})
	}
}
