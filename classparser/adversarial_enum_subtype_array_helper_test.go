package javaclassparser

import (
 "bytes"
 "os"
 "os/exec"
 "path/filepath"
 "sort"
 "strings"
 "testing"
)

// Array construction is independent of whether a constant's allocation has
// the enum's exact class or a constant-specific subclass. Keep the original
// constructor effects and dispatch observer unchanged across recompilation.
func TestAdversarialEnumSubtypeArrayHelperKeepsAllocationIdentityAndDispatch(t *testing.T) {
 compiler,java:=t04Tools(t);native:=os.Getenv("JAVA8_JAVAC");if native==""{t.Fatal("native original compiler required")}
 for _,owner:=range []string{"ArgumentOwner","ReboundDispatchOwner"}{for _,debug:=range []string{"none","source,lines,vars"}{t.Run(owner+"/"+debug,func(t *testing.T){
  fixture:=strings.ReplaceAll(enumArgumentHelperFixture,"ArgumentOwner",owner)
  fixture=strings.Replace(fixture,"FIRST(Effects.next(3),Long.MIN_VALUE,-0.0,new Box(Effects.next(5)))","FIRST(Effects.next(3),Long.MIN_VALUE,-0.0,new Box(Effects.next(5))){public long fold(long input){return super.fold(input)+17;}}",1)
  fixture=strings.Replace(fixture,"if(modes[i].fold(value)!=expected)","if(modes[i].fold(value)!=(expected+(i==0?17:0)))",1)
  fixture=strings.Replace(fixture,"int rows=0;for(long value",`if(modes[0].getClass()==`+owner+`.Mode.class||modes[1].getClass()!=`+owner+`.Mode.class)throw new AssertionError("constant-specific allocation class");int rows=0;for(long value`,1)
  files:=compileLambdaParameterMetadataFixture(t,native,NativeJavac8,fixture,owner,debug,false)
  run:=func(dir string){t.Helper();for _,mode:=range []string{"-ea","-da"}{for _,failure:=range []string{"0","3","7"}{out,err:=enumFreshOracleRun(t,java,mode,dir,"ArgumentDriver",failure);rows:="0";if failure=="0"{rows="10"};if err!=nil||string(out)!=rows+":typed:enum:arguments:effects:binding\n"{t.Fatal("unchanged original subtype/effect oracle",err,string(out))}}}
  run(sourceArtifactWriteFiles(t,files))
  owned,external:=map[string][]byte{},map[string][]byte{}
  for n,b:=range files{if n==owner+".class"||strings.HasPrefix(n,owner+"$"){owned[n]=b}else{external[n]=b}}
  jar:=filepath.Join(t.TempDir(),"input.jar");if err:=os.WriteFile(jar,t23Zip(t,owned),0600);err!=nil{t.Fatal(err)}
  a,err:=NewJarReconstructionArtifactFromLocal(jar,8,ModernJavac,func(n string)([]byte,bool){b,k:=files[n+".class"];return b,k});if err!=nil{t.Fatal(err)};defer a.Close()
  source,err:=a.ReadFile(owner+".class");if err!=nil||legacySourceHasFailureMarker(string(source))||!strings.Contains(string(source),"enum Mode"){t.Fatal("complete native enum family",err,string(source))}
  out:=sourceArtifactWriteFiles(t,external);path:=filepath.Join(out,owner+".java");if err:=os.WriteFile(path,source,0600);err!=nil{t.Fatal(err)}
  if log,err:=exec.Command(compiler,"-proc:none","--release","8","-g:"+debug,"-cp",out,"-sourcepath",out,"-d",out,path).CombinedOutput();err!=nil{t.Fatal("source only native subtype family",err,string(log),string(source))}
  names:=[]string{owner+"$Mode.class"};for n:=range owned{if n!=names[0]{names=append(names,n)}};sort.Strings(names[1:])
  for _,name:=range names{path:=filepath.Join(out,name);fresh,err:=os.ReadFile(path);if err!=nil{t.Fatal("complete declaration universe",name,err)};saved:=bytes.Clone(fresh)
   finished,err:=a.FinishClass(name,fresh);if err!=nil{t.Fatal("subtype allocation before fresh array cut",name,err)}
   if !bytes.Equal(saved,fresh){t.Fatal("fresh input modified")};if again,err:=a.FinishClass(name,finished);err!=nil||!bytes.Equal(again,finished){t.Fatal("idempotence",name,err)}
   if err:=os.WriteFile(path,finished,0600);err!=nil{t.Fatal(err)}
  };run(out)
 })}}
}
