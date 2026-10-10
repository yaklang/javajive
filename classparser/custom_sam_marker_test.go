package javaclassparser

import "testing"

func TestAdversarialCustomSamMarkerIdentityRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CustomSamMarker", `import java.util.*;
interface Marker {}
interface Make<T>{T get();}
public class CustomSamMarker {
 private static final Make<Map<String,Object>> FACTORY=(Make<Map<String,Object>> & Marker) HashMap::new;
 public static void main(String[] args){Map<String,Object> map=FACTORY.get();map.put("key","value");System.out.print((FACTORY instanceof Marker)+":"+map.get("key"));}
}`, Precision, Compatibility)
}
