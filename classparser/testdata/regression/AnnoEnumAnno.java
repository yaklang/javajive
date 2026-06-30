// The "external" annotation: carries a NESTED enum (Kind), mirroring j2objc's
// com.google.j2objc.annotations.ReflectionSupport.Level. Compiled alongside AnnoEnumSeed, but only
// AnnoEnumSeed.class is used as the regression seed -- so at decompile time AnnoEnumAnno$Kind is an
// external (non-sibling) nested enum, exactly like ReflectionSupport$Level for guava.
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;

@Retention(RetentionPolicy.RUNTIME)
public @interface AnnoEnumAnno {
    Kind value();

    enum Kind {
        FULL,
        NONE
    }
}
