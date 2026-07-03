import java.util.HashMap;
import java.util.Map;
import java.util.function.UnaryOperator;

public class NewRecvDiamondSeed {
    private final Map<String, String> aliasMap = new HashMap<String, String>();

    public void resolve(UnaryOperator<String> resolver) {
        new HashMap<String, String>(this.aliasMap).forEach((k, v) -> {
            String a = resolver.apply(k);
            String b = resolver.apply(v);
            if (a != null && b != null && !a.equals(b)) {
                this.aliasMap.put(a, b);
            }
        });
    }
}
