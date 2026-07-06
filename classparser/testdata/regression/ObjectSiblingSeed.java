import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;

// 承重种子: 两条互斥分支各自 ALLOCATE 一个引用类型, 存入同一个槽位, 随后在分支合流后被共同读取。
// 窄合并 (跨类 jar 内 LUB / JDK 表 LUB) 都无法把两臂关联起来时, 由 reachingRefSlotObjectSiblingArmMerge
// 用「桥接 LUB」(jar 父链接入 JDK 表) 统一。覆盖两种形态:
//   pickObject: HashMap(Map) vs ArrayList(List) -> 真正只共享 java.lang.Object。
//   pickMap:    HashMap vs MyMap(extends LinkedHashMap extends HashMap) -> 桥接后 LUB=HashMap,
//               若盲目升到 Object 则 map.put(...) 会 "cannot find symbol"。
public class ObjectSiblingSeed {
    static class MyMap extends LinkedHashMap {
    }

    static void sink(Object o) {
    }

    Object pickObject(boolean b) {
        Object obj;
        if (b) {
            HashMap m = new HashMap();
            m.put("k", "v");
            obj = m;
        } else {
            ArrayList a = new ArrayList();
            a.add("x");
            obj = a;
        }
        sink(obj);
        return obj;
    }

    HashMap pickMap(boolean b) {
        HashMap map;
        if (b) {
            map = new HashMap();
        } else {
            map = new MyMap();
        }
        map.put("k", "v");
        return map;
    }
}
