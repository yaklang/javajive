import java.util.ArrayList;
import java.util.List;

public class WildcardRetSeed {
    List<?> helper() {
        return new ArrayList<Object>();
    }

    @SuppressWarnings("unchecked")
    <T> List<T> create(int x) {
        if (x == 0) {
            return null;
        }
        return (List<T>) this.helper();
    }
}
