package javaclassparser

import (
	"os"
	"testing"
)

func TestAdversarialLoopCursorSnapshotSurvivesSlotReuse(t *testing.T) {
	const fixture = `
public class CursorOwner {
 public static int find(Object[] words,Object target,int start){
  if(words==null)return -1;if(start<0)start=0;
  if(target==null){for(int i=start;i<words.length;i++)if(words[i]==null)return i;}
  else{for(int i=start;i<words.length;i++)if(target.equals(words[i]))return i;}
  return -1;
 }
 public static int findLong(Object[] words,Object target,int start){
  if(words==null)return -1;if(start<0)start=0;
  if(target==null){for(long i=start;i<words.length;i++)if(words[(int)i]==null)return (int)i;}
  else{for(long i=start;i<words.length;i++)if(target.equals(words[(int)i]))return (int)i;}
  return -1;
 }
 public static int findFloat(Object[] words,Object target,int start){
  if(words==null)return -1;if(start<0)start=0;
  if(target==null){for(float i=start;i<words.length;i++)if(words[(int)i]==null)return (int)i;}
  else{for(float i=start;i<words.length;i++)if(target.equals(words[(int)i]))return (int)i;}
  return -1;
 }
 public static int findDouble(Object[] words,Object target,int start){
  if(words==null)return -1;if(start<0)start=0;
  if(target==null){for(double i=start;i<words.length;i++)if(words[(int)i]==null)return (int)i;}
  else{for(double i=start;i<words.length;i++)if(target.equals(words[(int)i]))return (int)i;}
  return -1;
 }
 public static int append(int[] links,int[] keys,int cursor,int key,int fresh){
  if(cursor==-1){links[0]=fresh;}else{
   int previous;
   do{previous=cursor;if(keys[cursor]==key)return cursor;cursor=links[cursor];}while(cursor!=-1);
   links[previous]=fresh;
  }
  int size=fresh+1;return size;
 }
}
class CursorDriver {public static void main(String[]args){int rows=0;int[] base={-1,-1,-1,5,-1,2,-1,-1,-1};int[] keys={10,11,12,13,14,15,16,17,18};
 for(int start:new int[]{-1,2,5,3})for(int key:new int[]{12,13,15,99}){int[] links=base.clone(),expected=base.clone();int tail=start,found=-1;while(tail!=-1){if(keys[tail]==key){found=tail;break;}if(expected[tail]==-1)break;tail=expected[tail];}
  if(found==-1)expected[start==-1?0:tail]=8;int want=found==-1?9:found;int got=CursorOwner.append(links,keys,start,key,8);
  if(got!=want||!java.util.Arrays.equals(links,expected))throw new AssertionError("snapshot start="+start+" key="+key+" result="+got+" links="+java.util.Arrays.toString(links));rows++;
 }
 Object[] words={"a",null,"b","b",null};for(int start=-1;start<=6;start++)for(Object target:new Object[]{null,"a","b","z"}){int want=-1;for(int i=Math.max(0,start);i<words.length;i++)if(java.util.Objects.equals(target,words[i])){want=i;break;}int got=CursorOwner.find(words,target,start);if(got!=want||CursorOwner.findLong(words,target,start)!=want||CursorOwner.findFloat(words,target,start)!=want||CursorOwner.findDouble(words,target,start)!=want)throw new AssertionError("index copy "+start+":"+target);rows++;}
 if(CursorOwner.find(null,"a",0)!=-1)throw new AssertionError("null array");rows++;
 System.out.println(rows+":cursor:snapshot:slot-reuse:effects");}}
`
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Fatal("native compiler required")
				}
			}
			compile := func(debug string) map[string][]byte {
				return compileLambdaParameterMetadataFixture(t, compiler, profile, fixture, "CursorOwner", debug, false)
			}
			testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"CursorOwner"}, "CursorDriver", "49:cursor:snapshot:slot-reuse:effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}
