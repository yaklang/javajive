package javaclassparser

import "testing"

// Branch-selected values are loop invariants even when continue/break create
// additional copying joins. An SSA join is a stable equation, not a changing
// alias of whichever predecessor was processed last.
func TestNativeOriginalLoopSnapshotConvergesForBranchSelectedValues(t *testing.T) {
	files := nativeCompileClasses(t, `class LoopOriginWitness {
 static long branchStores(Integer input,int count){long value;if(input!=null){value=input.intValue();}else{value=0L;}for(int i=0;i<count;i++){System.nanoTime();}return value;}
 static long invariant(boolean flag,long input,int count){long value=flag?input:7L;for(int i=0;i<count;i++){if(flag)continue;System.nanoTime();}return value;}
 static double precise(boolean flag,double input,int count){double value=flag?input:-0.0;for(int i=0;i<count;i++){if(i==3)break;if(flag)continue;System.nanoTime();}return value;}
 static Object reference(boolean flag,Object first,Object second,int count){Object value=flag?first:second;for(int i=0;i<count;i++){if(flag)continue;System.nanoTime();}return value;}
}`)
	object, err := Parse(files["LoopOriginWitness.class"])
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name == "<init>" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			for _, attribute := range method.Attributes {
				if code, ok := attribute.(*CodeAttribute); ok {
					_, fn, known := NewClassObjectDumper(object).nativeOriginalMethodSnapshot(method, code)
					if !known || fn == nil {
						t.Fatal("valid original loop did not reach the SSA fixed point")
					}
					if fn.Work > 1000 {
						t.Fatalf("small loop revisited %d blocks", fn.Work)
					}
				}
			}
		})
	}
}
