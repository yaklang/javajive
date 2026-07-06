package javaclassparser

// 承重测试: 两条互斥分支各自 ALLOCATE 一个引用类型存入同一槽位, 分支合流后被共同读取。窄合并 (跨类 jar 内
// LUB / JDK 表 LUB) 都无法关联两臂时, 由 reachingRefSlotObjectSiblingArmMerge 用「桥接 LUB」(jar 父链
// 接入 JDK 表) 统一。kill-switch: JDEC_REF_SLOT_OBJECT_SIBLING_ARM_MERGE_OFF。
//
// 覆盖两种形态 (镜像 fastjson2 JSON.parse 与 JSONReaderJSONB.readObject):
//   pickObject: HashMap(Map) vs ArrayList(List) -> 真正只共享 java.lang.Object, 合并声明应为 `Object`。
//   pickMap:    HashMap vs MyMap(extends LinkedHashMap extends HashMap) -> jar provider 只看到 MyMap 的
//               直接 JDK 父名, 桥接后 LUB=HashMap; 若盲目升到 Object, `map.put(...)` 会 cannot find symbol。
// 不修 (kill-switch 置位) 时两臂各自分裂, 合流读绑到只在一条分支赋值的变量 -> definite-assignment。

import (
	"os"
	"strings"
	"testing"
)

func TestObjectSiblingArmMergeIsLoadBearing(t *testing.T) {
	outer, err := os.ReadFile("testdata/regression/ObjectSiblingSeed.class")
	if err != nil {
		t.Fatalf("read ObjectSiblingSeed seed: %v", err)
	}
	myMap, err := os.ReadFile("testdata/regression/ObjectSiblingSeed$MyMap.class")
	if err != nil {
		t.Fatalf("read ObjectSiblingSeed$MyMap seed: %v", err)
	}
	// The bridged-LUB (pickMap) case needs the jar-internal supertype provider to see that MyMap extends
	// LinkedHashMap; supply MyMap's bytes via the sibling resolver so SiblingSuperTypes is populated.
	resolver := func(internalName string) ([]byte, bool) {
		switch internalName {
		case "ObjectSiblingSeed$MyMap":
			return myMap, true
		case "ObjectSiblingSeed":
			return outer, true
		}
		return nil, false
	}

	// Fix ON (default): pickObject widens to the true LUB java.lang.Object; pickMap widens to the bridged
	// LUB java.util.HashMap (NOT Object, so `map.put` stays valid). One variable per method, no split.
	os.Unsetenv("JDEC_REF_SLOT_OBJECT_SIBLING_ARM_MERGE_OFF")
	on, err := DecompileWithResolver(outer, resolver)
	if err != nil {
		t.Fatalf("decompile (fix ON) failed: %v", err)
	}
	if !strings.Contains(on, "Object var2;") {
		t.Errorf("fix ON: pickObject expected a merged `Object var2;` (HashMap|ArrayList LUB = Object), got:\n%s", on)
	}
	if !strings.Contains(on, "HashMap var2;") || !strings.Contains(on, "var2.put(") {
		t.Errorf("fix ON: pickMap expected a merged `HashMap var2;` with a valid `var2.put(...)` (bridged LUB), got:\n%s", on)
	}
	if strings.Contains(on, "ObjectSiblingSeed$MyMap var2_1") {
		t.Errorf("fix ON: pickMap must NOT split off a `MyMap var2_1;`, got:\n%s", on)
	}

	// Fix OFF (kill-switch): both arms split into distinct variables; the post-branch read binds a var
	// assigned on only one branch -- the definite-assignment blocker this merge removes.
	t.Setenv("JDEC_REF_SLOT_OBJECT_SIBLING_ARM_MERGE_OFF", "1")
	off, err := DecompileWithResolver(outer, resolver)
	if err != nil {
		t.Fatalf("decompile (fix OFF) failed: %v", err)
	}
	if !strings.Contains(off, "ObjectSiblingSeed$MyMap var2_1;") {
		t.Errorf("fix OFF: expected pickMap to split into `MyMap var2_1;` (kill-switch load-bearing), got:\n%s", off)
	}
}
