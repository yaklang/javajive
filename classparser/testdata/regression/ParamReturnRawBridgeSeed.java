// 种子: 泛型方法声明返回 Map<K,List<V>>, 方法体 `return var0.asMap();`, 其中 var0 是 ListLike<K,V>。
// ListLike 继承 MapLike<K,V>, 而 MapLike 声明 `Map<K,Collection<V>> asMap()`。故 var0.asMap() 的真实
// 泛型返回是 Map<K,Collection<V>>, 与声明返回 Map<K,List<V>> 同 raw 擦除但实参不同(Map 不变型): 直接
// (Map<K,List<V>>) 造型 inconvertible(两者均完全参数化且实参可证不同), 唯 raw 桥接
// (Map<K,List<V>>)(Map)value 合法。字节码里两处 checkcast 到 Map 均为 no-op 被丢弃, 反编译得裸
// `return var0.asMap();`, javac 解析 asMap 真实返回后报 `Map<K,Collection<V>> cannot be converted to
// Map<K,List<V>>`。治法(JDEC_PARAM_RETURN_RAW_BRIDGE_OFF): 经接收者层级 SiblingClassSig 恢复 asMap 的
// 真实实例化泛型返回, 判定同擦除异参后重新发出 raw 桥接造型。镜像 guava Multimaps.asMap(ListMultimap)。
// 需 MapLike/ListLike 兄弟单元由 resolver 提供以恢复 asMap 的真实泛型返回(单类反编译无 SiblingClassSig,
// 治本不触发)。
import java.util.Collection;
import java.util.List;
import java.util.Map;

public class ParamReturnRawBridgeSeed {
    static <K, V> Map<K, List<V>> asMap(ParamReturnRawBridgeListLike<K, V> var0) {
        return (Map<K, List<V>>) (Map<K, ?>) var0.asMap();
    }
}

interface ParamReturnRawBridgeMapLike<K, V> {
    Map<K, Collection<V>> asMap();
}

interface ParamReturnRawBridgeListLike<K, V> extends ParamReturnRawBridgeMapLike<K, V> {
}
