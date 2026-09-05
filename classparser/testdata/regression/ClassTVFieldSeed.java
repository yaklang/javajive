public class ClassTVFieldSeed<T> {
    Class<T> _handledType;
    ClassTVFieldSeed(RawClassHolder t) {
        this._handledType = (Class) t.getRawClass();
    }
}
