package javaclassparser

import (
	"strings"
	"testing"
)

// A loop's int web must survive a later long occupying the same local slot.
// Original JVM byte loads sign-extend; masks, joins, cursor effects and wide
// post-loop values are checked independently of the reconstructed source.
func TestAdversarialByteWideReuseKeepsSelectedWord(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `public class PacketCursor{
 public byte[] bytes;public int offset,observed,steps;public char current;
 public PacketCursor(byte[] value){bytes=value;}
 public long scan(){int i=0;for(;i<bytes.length;){int word=bytes[i];if(word==7){i++;word=bytes[i]&255;while(word==32){word=bytes[++i]&255;steps++;}if(word!=6)throw new IllegalArgumentException();if(++i==bytes.length)word=26;else word=bytes[i];while(word==32){word=bytes[++i];steps++;}offset=i+1;observed=word;current=(char)word;break;}else if(word>=0){i++;}else{word&=255;switch(word>>4){case 12:case 13:i+=2;break;case 14:i+=3;break;default:throw new IllegalArgumentException();}}} long first=bytes[0];long second=bytes[bytes.length-1];return first*31+second;}
 }
class PacketCursorDriver{public static void main(String[]args){int rows=0;for(byte[]prefix:new byte[][]{new byte[0],new byte[]{1,2},new byte[]{(byte)0xc3,(byte)0xa9},new byte[]{(byte)0xe4,(byte)0xb8,(byte)0xad}})for(int a=0;a<3;a++)for(int b=0;b<3;b++)for(byte value:new byte[]{-128,-1,0,1,33,127})for(boolean eof:new boolean[]{false,true}){
 byte[]data=new byte[prefix.length+2+a+(eof?0:b+1)];System.arraycopy(prefix,0,data,0,prefix.length);data[prefix.length]=7;for(int i=0;i<a;i++)data[prefix.length+1+i]=32;data[prefix.length+1+a]=6;if(!eof){for(int i=0;i<b;i++)data[prefix.length+2+a+i]=32;data[data.length-1]=value;}
 PacketCursor owner=new PacketCursor(data);long result=owner.scan();if(result!=data[0]*31L+data[data.length-1])throw new AssertionError("wide reuse");int want=eof?26:value;if(owner.observed!=want||owner.current!=(char)want||owner.offset!=(eof?data.length+1:data.length)||owner.steps!=a+(eof?0:b))throw new AssertionError(prefix.length+":"+a+":"+b+":"+value+":"+eof+":"+owner.observed);rows++;
 }
 for(byte[] invalid:new byte[][]{null,new byte[]{7},new byte[]{7,5}}){PacketCursor owner=new PacketCursor(invalid);try{owner.scan();throw new AssertionError("expected failure");}catch(RuntimeException failure){Class<?> expected=invalid==null?NullPointerException.class:invalid.length==1?ArrayIndexOutOfBoundsException.class:IllegalArgumentException.class;if(failure.getClass()!=expected||owner.offset!=0||owner.observed!=0||owner.current!=0||owner.steps!=0)throw new AssertionError("exception/effect ordering",failure);}}
 System.out.println(rows+":loop:byte-cursor:selection:effects");}}`
	for _, shape := range []string{"field", "static", "parameter", "copy", "call"} {
		t.Run(shape, func(t *testing.T) {
			variant := source
			switch shape {
			case "static":
				variant = strings.Replace(variant, "public byte[] bytes;", "public static byte[] bytes;", 1)
			case "parameter", "copy":
				variant = strings.Replace(variant, "public long scan(){", "public long scan(){return scanValue(bytes);} public long scanValue(byte[] input){", 1)
				start, end := strings.Index(variant, "public long scanValue"), strings.Index(variant, "class PacketCursorDriver")
				part := strings.ReplaceAll(variant[start:end], "bytes", "input")
				if shape == "copy" {
					part = strings.Replace(part, "int i=0;", "byte[] alias=input;byte[] local=alias;int i=0;", 1)
					part = strings.ReplaceAll(part, "input[", "local[")
					part = strings.ReplaceAll(part, "input.length", "local.length")
				}
				variant = variant[:start] + part + variant[end:]
			case "call":
				variant = strings.Replace(variant, "public long scan(){", "public byte[] data(){return bytes;} public long scan(){", 1)
				start, end := strings.Index(variant, "public long scan"), strings.Index(variant, "class PacketCursorDriver")
				part := strings.ReplaceAll(strings.ReplaceAll(variant[start:end], "bytes[", "data()["), "bytes.length", "data().length")
				variant = variant[:start] + part + variant[end:]
			}
			if shape == "parameter" {
				variant = strings.ReplaceAll(variant, "input[i]&255", "(input[i]|0)&255")
				variant = strings.ReplaceAll(variant, "input[++i]&255", "(input[++i]|0)&255")
			}
			if shape == "call" {
				variant = strings.ReplaceAll(variant, "data()[i]&255", "(data()[i]^0)&255")
				variant = strings.ReplaceAll(variant, "data()[++i]&255", "(data()[++i]^0)&255")
			}
			owners := []string{"PacketCursor"}
			if shape == "field" {
				owners = append(owners, "FrameReader")
			}
			for _, owner := range owners {
				t.Run(owner, func(t *testing.T) {
					testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(variant, "PacketCursor", owner), owner, owner+"Driver", "432:loop:byte-cursor:selection:effects\n")
				})
			}
		})
	}
}
