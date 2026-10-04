package javaclassparser

import "testing"

func TestAdversarialCompletedArrayArgumentRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ArrayArgument", `import java.util.*;
class ArraySink {
  static int reads;
  static byte[] encode(String text) {reads++;return text.getBytes(java.nio.charset.StandardCharsets.UTF_8);}
  String send(byte[]... arrays) {return Arrays.deepToString(arrays);}
}
public class ArrayArgument extends ArraySink {
  String send(String text) {return this.send(new byte[][]{encode(text)});}
  String filled(String text) {byte[][] arrays=new byte[1][];arrays[0]=encode(text);return this.send(arrays);}
  String multiple(String a,String b) {return this.send(new byte[][]{encode(a),encode(b)});}
  public static void main(String[] args) {
    ArrayArgument sink=new ArrayArgument();
    System.out.print(sink.send("ab")+":"+sink.multiple("c","de")+":"+sink.filled("fg")+":"+reads);
  }
}`, Precision, Compatibility, "legacy")
}
