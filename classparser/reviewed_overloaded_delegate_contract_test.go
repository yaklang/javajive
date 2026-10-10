package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func assertReviewedAssertjObjectDelegation(t *testing.T, class string, pc uint16, argument string) {
	t.Helper()
	const jar = "org/assertj/assertj-core/3.24.2/assertj-core-3.24.2.jar"
	owner := "org/assertj/core/api/" + class
	raw := originalJarClassForReview(t, jar, owner+".class")
	desc := "(Ljava/lang/String;)L" + owner + ";"
	assertReviewedTypeVarMethod(t, raw, "isEqualTo", desc, "(Ljava/lang/String;)TSELF;")
	path := filepath.Join(t.TempDir(), class+".class")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	assertReviewedTypeVarInvoke(t, path, "isEqualTo", desc, pc, core.OP_INVOKEVIRTUAL, owner, "isEqualTo", "(Ljava/lang/Object;)Lorg/assertj/core/api/AbstractAssert;")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_ASSERTJ_REMAINING_OFF", setting)
		fs, err := NewJarFSFromLocal(filepath.Join(home, ".m2/repository", jar))
		if err != nil {
			t.Fatal(err)
		}
		source, err := fs.ReadFile(owner + ".class")
		fs.Close()
		if err != nil {
			t.Fatal(err)
		}
		body := reviewedSourceMethod(t, string(source), `SELF\s+isEqualTo\(String\s+\w+\)`)
		requireReviewedPattern(t, body, `this\.isEqualTo\(`+argument+`\)`)
		if strings.Contains(body, "(String)(") {
			t.Fatalf("Object overload gained an unrelated String check:\n%s", body)
		}
	}
}

func TestAdversarialOverloadedTypedDelegateRoundTrip(t *testing.T) {
	// Both original call sites select the Object descriptor, despite a String
	// overload in the consumer. Observe dispatch, return identity and parsing
	// errors rather than asking a text workaround to change the whole class.
	t.Setenv("JDEC_ASSERTJ_REMAINING_OFF", "1")
	roundTripGenericFlow(t, "OverloadedDelegateReview", `import java.math.*;import java.time.*;
class ReviewedDelegateBase<S> {String trace="";Object actual;S isEqualTo(Object value){trace+="O";actual=value;return (S)this;}}
public class OverloadedDelegateReview extends ReviewedDelegateBase<OverloadedDelegateReview> {
 OverloadedDelegateReview isEqualTo(String value){trace+="S";return isEqualTo(new BigDecimal(value));}
 Instant parse(String value){trace+="P";return Instant.parse(value);}
 OverloadedDelegateReview instant(String value){return isEqualTo(parse(value));}
 static String run(String value,boolean instant){OverloadedDelegateReview c=new OverloadedDelegateReview();try{OverloadedDelegateReview result=instant?c.instant(value):c.isEqualTo(value);return (result==c)+":"+c.actual.getClass().getName()+":"+c.actual+":"+c.trace;}catch(Throwable e){return e.getClass().getName()+":"+c.trace+":"+(c.actual==null);}}
 public static void main(String[] args){for(String value:new String[]{null,"bad","0","1.25","2020-01-02T03:04:05Z"}){System.out.println(run(value,false));System.out.println(run(value,true));}}
}`, Precision, Compatibility, "legacy")
}
