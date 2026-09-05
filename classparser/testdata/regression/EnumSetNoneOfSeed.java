import java.util.EnumSet;
public class EnumSetNoneOfSeed {
    EnumRawClass _enumType = new EnumRawClass();
    @SuppressWarnings({"unchecked","rawtypes"})
    EnumSet constructSet() {
        return EnumSet.noneOf((Class) this._enumType.getRawClass());
    }
}
