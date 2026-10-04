package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Constructors and element producers stay on the original classpath. The
// consumer's seven stores include a branch allocation and a final cloned array
// operand: all must complete in bytecode order before the first delegation.
func TestAdversarialSevenElementSuperArrayRoundTrip(t *testing.T) {
	t.Setenv("JDEC_HTTPCLIENT_REMAINING_OFF", "1")
	roundTripGenericFlow(t, "CtorArrayReview", `import java.io.*;import java.util.*;
interface CtorHandler{int id();}
class CtorEvents{static String trace="";static int fail=-1;static final IOException failure=new IOException("original");static String[] defaults={"default"};static void seen(int id)throws IOException{trace+=id+":";if(fail==id)throw failure;}static String[] fallback(){trace+="D:";return defaults;}}
class CtorLeaf implements CtorHandler{final int id;CtorLeaf(int id)throws IOException{CtorEvents.seen(id);this.id=id;}public int id(){return id;}}
class CtorLast extends CtorLeaf{final String[] patterns;CtorLast(String[]patterns)throws IOException{super(7);this.patterns=patterns;}}
class CtorBase{final CtorHandler[] handlers;CtorBase(CtorHandler[] handlers){CtorEvents.trace+="B:";this.handlers=handlers;}}
public class CtorArrayReview extends CtorBase{
 CtorArrayReview(String[]patterns,boolean choose)throws IOException{super(new CtorHandler[]{new CtorLeaf(0),new CtorLeaf(1),choose?new CtorLeaf(2):new CtorLeaf(3),new CtorLeaf(4),new CtorLeaf(5),new CtorLeaf(6),new CtorLast(patterns!=null?patterns.clone():CtorEvents.fallback())});CtorEvents.trace+="C:";}
 public static void main(String[]args){for(boolean choose:new boolean[]{false,true})for(String[]patterns:new String[][]{null,new String[0],new String[]{"original"}})for(int fail:new int[]{-1,0,2,3,5,6,7}){CtorEvents.fail=fail;CtorEvents.trace="";try{CtorArrayReview owner=new CtorArrayReview(patterns,choose);CtorLast last=(CtorLast)owner.handlers[6];System.out.println(owner.handlers.length+":"+owner.handlers[2].id()+":"+(last.patterns==(patterns==null?CtorEvents.defaults:patterns))+":"+Arrays.toString(last.patterns)+":"+CtorEvents.trace);if(patterns!=null&&patterns.length>0){patterns[0]="changed";System.out.println(last.patterns[0]);patterns[0]="original";}}catch(Throwable error){System.out.println((error==CtorEvents.failure)+":"+CtorEvents.trace);}}}
}`, Precision, Compatibility, "legacy")
}

// Original metadata bytes from httpclient 4.5.13, JAR SHA256 6fe9026a566c6a5001608cf3fc32196641f6c1e5e1986d1037ccdbd5f31ef743.
// Only the closed handler/interface parent chains are retained and parsed.
func reviewedBrowserDelegationMetadata(t *testing.T) func(string) ([]byte, bool) {
	t.Helper()
	expected := map[string]string{
		"org/apache/http/cookie/CommonCookieAttributeHandler":              "6adb6b92a9302abf1bdcebc47c8e1bc2f7d4617dd0b29b6184ff1ee29e32236a",
		"org/apache/http/cookie/CookieAttributeHandler":                    "f6ef768a46b1428c26ef1fab6d16f4d4f1df89cbb0533c01f4ad77a387f350c1",
		"org/apache/http/impl/cookie/AbstractCookieAttributeHandler":       "53fdd055ff4327615ef0149e43f7308a391e03fa0b7fac1247b9d463da037d20",
		"org/apache/http/impl/cookie/BasicCommentHandler":                  "bd1c8da3ab05fe05272fcbee8fceddc78ec943cdb65e6afac8667e214088a53b",
		"org/apache/http/impl/cookie/BasicDomainHandler":                   "9cf6ac3d7efe6e5ee3bba994e14b888513c7c419251d2085c9f6047026d590fb",
		"org/apache/http/impl/cookie/BasicExpiresHandler":                  "00543c2bb8511c347277f59c4db3ea609e79a10311005d0046a28a112c1a231f",
		"org/apache/http/impl/cookie/BasicMaxAgeHandler":                   "ec6ed0fb6f7a3473ed251fafd10a873d0ee9d72d53e073231d0a27e1a70a65c9",
		"org/apache/http/impl/cookie/BasicPathHandler":                     "a6eeed1071374079c62aaf2e77a103f80700a868673f2ac7a81b1285f2c0380c",
		"org/apache/http/impl/cookie/BasicSecureHandler":                   "0fc9cac8b3297253b41bc29255f011088d2fb661f03b36bdd161ecc5e6a4e0ee",
		"org/apache/http/impl/cookie/BrowserCompatSpec$1":                  "8e42fd73225bcd9001c7f60c2b39595a053040caddbc80908b2a2707d59afe2c",
		"org/apache/http/impl/cookie/BrowserCompatVersionAttributeHandler": "56af026d22b28023752daa2c5b903b62c9ca466e25364f2822fda6661eaebe7f",
	}
	data := map[string][]byte{}
	for name, hash := range expected {
		raw, err := os.ReadFile(filepath.Join("testdata/regression/httpclient-4.5.13-delegation-metadata", name+".class"))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(raw)) != hash {
			t.Fatalf("original hierarchy metadata %s changed", name)
		}
		if _, err := Parse(raw); err != nil {
			t.Fatal(err)
		}
		data[name] = raw
	}
	return func(name string) ([]byte, bool) { raw, ok := data[strings.ReplaceAll(name, ".", "/")]; return raw, ok }
}
