package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const ancestorLoopFixture = `class AncestorLoop{static long run(int limit,int mask){int i=0;long trace=7;outer:while(i<limit){int k=0;for(;;){if(k==0&&(mask&1)==0){trace=trace*31+i+1000;k++;continue;}if(((i+mask)&1)==0){trace=trace*31+i*3+1;i++;continue outer;}if(((i+mask)&2)==0){trace=trace*31+i*3+2;i+=2;continue outer;}trace=trace*31+i*3+3;i+=3;continue outer;}}return trace;}}
class AncestorLoopDriver{public static void main(String[]args){int rows=0;for(int limit=0;limit<18;limit++)for(int mask=-8;mask<8;mask++){int step=0;long oracle=7;while(step<limit){int low=(step+mask)&3;int delta=low==0||low==2?1:low==1?2:3;if((mask&1)==0)oracle=oracle*31+step+1000;oracle=oracle*31+step*3+delta;step+=delta;}long got=AncestorLoop.run(limit,mask);if(got!=oracle)throw new AssertionError("trace/termination "+limit+"/"+mask+" "+got+"/"+oracle);rows++;}System.out.println(rows+":ancestor:continue:trace:termination");}}`

func testSharedAncestorLoop(t *testing.T, original8 bool) {
	for _, shape := range []string{"while", "do-while", "for", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			f := ancestorLoopFixture
			owner := "AncestorLoop"
			switch shape {
			case "do-while":
				f = strings.Replace(f, "for(;;){", "do{", 1)
				f = strings.Replace(f, "continue outer;}}return", "continue outer;}while(true);}return", 1)
			case "for":
				f = strings.Replace(f, "outer:while(i<limit)", "outer:for(;i<limit;)", 1)
			case "renamed":
				f = strings.ReplaceAll(f, "AncestorLoop", "RegionOwner")
				owner = "RegionOwner"
			}
			driver := strings.TrimSuffix(owner, "Loop") + "LoopDriver"
			if shape == "renamed" {
				driver = "RegionOwnerDriver"
			}
			if original8 {
				javac := os.Getenv("JAVA8_JAVAC")
				if javac == "" {
					t.Skip("JAVA8_JAVAC required for actual original compiler")
				}
				testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, f, owner, debug) }, NativeJavac8, javac, []string{owner}, driver, "288:ancestor:continue:trace:termination\n", nil)
			} else {
				testNativeIndependentFamilyFixture(t, f, []string{owner}, driver, "288:ancestor:continue:trace:termination\n")
			}
		})
	}
}

func TestAdversarialSharedLoopExitRetainsEnclosingContinue(t *testing.T) {
	testSharedAncestorLoop(t, false)
}
func TestAdversarialSharedLoopExitEnclosingContinueOriginalJavac8(t *testing.T) {
	testSharedAncestorLoop(t, true)
}
