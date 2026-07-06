// Regression seed for classForNameReturnCast: a method whose declared return type is a `Class<...>`
// parameterization MENTIONING a type variable (`Class<Wrapper<T>>`) returns `Class.forName(name)`. The JDK
// signature is `Class<?> forName(String)`, so javac captures the wildcard to CAP#1 and rejects
// `Class<CAP#1>` -> `Class<Wrapper<T>>`; the source carried an unchecked `(Class<Wrapper<T>>)` cast the
// bytecode dropped. Mirrors spring objenesis DelegatingToExoticInstantiator.instantiatorClass().
//
// Recompile: javac -d . ClassForNameRetSeed.java
public class ClassForNameRetSeed<T> {
    interface Wrapper<X> {}

    @SuppressWarnings("unchecked")
    Class<Wrapper<T>> load(String name) throws ClassNotFoundException {
        return (Class<Wrapper<T>>) Class.forName(name);
    }
}
