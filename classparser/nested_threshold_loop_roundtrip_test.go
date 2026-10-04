package javaclassparser

import "testing"

// A top-tested outer loop contains an inner counted loop and several returns.
// Its exit edge must still mean remaining <= threshold after jump rewriting.
func TestAdversarialNestedThresholdLoopRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ThresholdLoop", `
public class ThresholdLoop {
  static void decode(ThresholdReader reader,StringBuilder out,int threshold) {
    while(true) {
      if(reader.remaining()<=threshold) return;
      for(int index=0;index<4;index++) {
        int value=reader.read(6);
        if(value==31) {
          int padding=8-reader.offset();
          if(padding!=8) reader.read(padding);
          return;
        }
        if((value&32)==0) value|=64;
        out.append((char)value);
      }
      if(reader.remaining()<=0) return;
    }
  }
  public static void main(String[] args){ThresholdOracle.run();}
}
class ThresholdReader {
  final byte[] data;int position;
  ThresholdReader(byte[] data,int position){this.data=data;this.position=position;}
  int remaining(){ThresholdOracle.mark("remaining:"+(data.length*8-position));return data.length*8-position;}
  int offset(){ThresholdOracle.mark("offset:"+(position&7));return position&7;}
  int read(int width){
    ThresholdOracle.mark("read:"+width+":"+position);
    if(width<1 || width>data.length*8-position)throw new IllegalArgumentException("width:"+width);
    int value=0;
    for(int i=0;i<width;i++){value=(value<<1)|((data[position>>>3]>>>(7-(position&7)))&1);position++;}
    return value;
  }
}
class ThresholdOracle {
  static StringBuilder trace;static int calls,failAt;
  static void mark(String text){trace.append(text).append(';');if(++calls==failAt)throw new IllegalStateException(text);}
  static void run(){
    for(int sample=0;sample<12;sample++)for(int skip:new int[]{0,1,3,7})for(int failure:new int[]{0,1,3,8})for(int threshold:new int[]{0,16,24}) {
      byte[] input=new byte[sample%8];
      for(int i=0;i<input.length;i++)input[i]=(byte)(sample<4 ? 0 : sample<8 ? 255 : sample*37+i*53);
      ThresholdReader reader=new ThresholdReader(input,skip==0 || input.length!=0 ? skip : 0);
      trace=new StringBuilder();calls=0;failAt=failure;StringBuilder out=new StringBuilder();String error="";
      try {ThresholdLoop.decode(reader,out,threshold);}catch(Throwable e){error=e.getClass().getSimpleName()+":"+e.getMessage();}
      System.out.println(sample+":"+skip+":"+failure+":"+threshold+":"+out+":"+reader.position+":"+error+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
