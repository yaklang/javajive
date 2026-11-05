package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialDuplicatedByteRetainsLoopExitBinding(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class LengthReader {
 public static byte read(byte[] data,int[] cursor){return data[cursor[0]++];}
 public static long readWide(long[] data,int[] cursor){return data[cursor[0]++];}
 public static char readChar(char[] data,int[] cursor){return data[cursor[0]++];}
 public static long decodeWide(long[] data,int[] cursor,int rounds){long total=0;
  for(int r=0;r<rounds;r++){{long word;while((word=readWide(data,cursor))==Long.MIN_VALUE)total+=31;total^=word;}long later=readWide(data,cursor);total=total*17+later;}return total;
 }
 public static long decodeChars(char[] data,int[] cursor,int rounds){long total=0;
  for(int r=0;r<rounds;r++){{int word;while((word=readChar(data,cursor))==65535)total+=37;total^=word;}int later=readChar(data,cursor);total=total*19+later;}return total;
 }
 public static long decode(byte[] data,int[] cursor,int rounds){long total=0;
  for(int round=0;round<rounds;round++){
   {int length=0,word;while((word=read(data,cursor))==-1)length+=255;total+=length+(word&255);}
   int low=read(data,cursor)&255;int high=read(data,cursor)&255;int distance=low|(high<<8);total=total*100003L+distance;
  }return total;
 }
}
class LengthReaderDriver {public static void main(String[]args){int checks=0;
 for(int rounds=0;rounds<9;rounds++)for(int salt=0;salt<32;salt++){
  java.io.ByteArrayOutputStream bytes=new java.io.ByteArrayOutputStream();long expected=0;
  for(int r=0;r<rounds;r++){int repeats=(r+salt)%5,last=(r*83+salt*17)%255,distance=(r*3011+salt*8191)&65535;
   for(int i=0;i<repeats;i++)bytes.write(255);bytes.write(last);bytes.write(distance&255);bytes.write(distance>>>8);expected=(expected+repeats*255+last)*100003L+distance;
  }byte[] data=bytes.toByteArray();int[] cursor={0};long actual=LengthReader.decode(data,cursor,rounds);
  if(actual!=expected||cursor[0]!=data.length)throw new AssertionError("word/exit/evaluation count "+rounds+" "+salt);checks++;
 }
 for(int rounds=0;rounds<7;rounds++)for(int salt=0;salt<16;salt++){
  long[] wide=new long[rounds*5];char[] chars=new char[rounds*5];int size=0;long expectedWide=0,expectedChars=0;
  for(int r=0;r<rounds;r++){int repeats=(r+salt)%4;long word=(Long.MAX_VALUE-193L*r)^((long)salt<<33),later=(long)(r-salt)*7919;int letter=(r*8191+salt*3001)%65535,other=(r*3011+salt*1193)%65535;
   for(int i=0;i<repeats;i++){wide[size]=Long.MIN_VALUE;chars[size++]=(char)65535;}
   wide[size]=word;chars[size++]=(char)letter;wide[size]=later;chars[size++]=(char)other;
   expectedWide=((expectedWide+31*repeats)^word)*17+later;expectedChars=((expectedChars+37*repeats)^letter)*19+other;
  }int[] cursor={0};if(LengthReader.decodeWide(wide,cursor,rounds)!=expectedWide||cursor[0]!=size)throw new AssertionError("category two duplicate");checks++;
  cursor[0]=0;if(LengthReader.decodeChars(chars,cursor,rounds)!=expectedChars||cursor[0]!=size)throw new AssertionError("unsigned character copy");checks++;
 }System.out.println(checks+":byte:loop:exit:binding");}}
`
	for _, name := range []string{"LengthReader", "PacketAccumulator"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "LengthReader", name), name, name+"Driver", "512:byte:loop:exit:binding\n")
		})
	}
}
