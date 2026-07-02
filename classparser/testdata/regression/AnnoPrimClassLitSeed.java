import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

// Regression seed for the primitive class-literal annotation-default bug: a `void.class` / `int.class`
// default was rendered as `void_.class` / `int_.class` because the class-literal renderer ran the
// primitive keyword through ShortTypeName -> SafeIdentifier (which appends '_' to Java keywords),
// producing uncompilable source. Real hit: fastjson2 @JSONType `builder() default void.class`.
// Compile with `--release 8`.
@Retention(RetentionPolicy.RUNTIME)
@Target(ElementType.TYPE)
public @interface AnnoPrimClassLitSeed {
    Class<?> builder() default void.class;
    Class<?> boxer() default int.class;
    Class<?> flagType() default boolean.class;
    Class<?> ref() default String.class;
}
