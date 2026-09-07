package cross

// 本文件是「反编译 -> javac 重编译 -> 错误计数」的权威度量 harness, 对应 CODEC_TODO 里
// 引用的 TestScratchPerFileIso / TestScratchProfile / TestScratchJarErrDelta。它从本机
// ~/.m2 解析真实 jar (guava / fastjson2 / commons-codec / spring-core) 及其传递依赖,
// 用生产 JarFS 路径反编译每个 .class, 再用 javac 重编译, 统计 decompiler 错误数。
//
// 两种度量口径:
//   - tree: 整目录一次性 javac (deps 上 classpath)。快, 但受 javac 错误遮蔽, 仅用于找最大杠杆。
//   - iso : 逐文件隔离 javac (deps + 原 jar 上 classpath, 并行)。免遮蔽, 是治本 delta 的准绳。
//
// 用法 (全部 opt-in, 缺 javac 或缺 jar 自动 t.Skip):
//   PROFILE_JAR=fastjson2 go test -run TestJarRecompileProfile ./test/cross/
//   PROFILE_JAR=all RECOMPILE_MODE=iso go test -run TestJarRecompileProfile ./test/cross/
//   PROFILE_JAR=fastjson2 KILL_SWITCH=JDEC_LIVEINTERVAL_OFF go test -run TestJarRecompileDelta ./test/cross/
//
// 其它可选环境变量: MAXFILES (限制单元数, 快速冒烟), RECOMPILE_WORKERS (并行 javac 数)。
//
// 基线快照 (javac 17.0.12, 本机 ~/.m2, 各 Phase 治本以此为 delta 参照系; 绝对值随 JDK 版本浮动):
//
//	jar         units   tree decErr   iso fail
//	codec       106     1             38
//	fastjson2   681     680           342
//	guava       1825    688           1154
//	spring      974     14            379
//
// tree 口径与 CODEC_TODO 权威整树度量吻合 (codec=1 / spring=14 精确, guava≈699, 受 javac 遮蔽);
// iso 口径与 TODO 逐文件率吻合 (fastjson2≈50.5% / spring≈61.6% / codec≈68.9%)。

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	classparser "github.com/yaklang/javajive/classparser"
)

// compileTimeout bounds a single javac invocation so a pathological unit cannot stall the profile.
const compileTimeout = 60 * time.Second

// javacLocaleArgs forces javac diagnostics to English. The local JDK may be localized (e.g. Chinese
// "错误:"), which would make the tree-mode `: error:` line count read zero; pinning the locale makes
// the error histogram reliable. iso mode keys off the exit code so it is unaffected, but the flag is
// harmless there too.
var javacLocaleArgs = []string{"-J-Duser.language=en", "-J-Duser.country=US"}

// jarSpec 描述一个待度量的真实 jar 及其依赖 jar 的 maven 相对路径 (相对 ~/.m2/repository)。
// 依赖用 glob 模式表达 (按 artifact 名匹配, 不写死版本), 解析不到的依赖被静默跳过 (classpath 降级)。
type jarSpec struct {
	relPath    string   // 主 jar 相对 ~/.m2/repository 的路径
	depGlob    []string // 依赖 jar 的 glob (相对 ~/.m2/repository), 找不到则跳过
	minRelease int      // optional floor for javac --release (0 = jarBaseRelease only)
}

var jarSpecs = map[string]jarSpec{
	"guava": {
		relPath: "com/google/guava/guava/28.2-android/guava-28.2-android.jar",
		depGlob: []string{
			"com/google/code/findbugs/jsr305/*/jsr305-*.jar",
			"com/google/errorprone/error_prone_annotations/*/error_prone_annotations-*.jar",
			"com/google/j2objc/j2objc-annotations/*/j2objc-annotations-*.jar",
			"com/google/guava/failureaccess/*/failureaccess-*.jar",
			"org/checkerframework/checker-compat-qual/*/checker-compat-qual-*.jar",
			"org/checkerframework/checker-qual/*/checker-qual-*.jar",
		},
	},
	"fastjson2": {
		relPath: "com/alibaba/fastjson2/fastjson2/2.0.43/fastjson2-2.0.43.jar",
		// fastjson2 的 KotlinUtils 是 kotlin 可选集成, 引用 kotlin.reflect / kotlin.jvm.internal
		// (编译期接口全在 kotlin-stdlib 内)。缺失该依赖会误报 "package kotlin.* does not exist",
		// 与 spring-jcl / failureaccess 同属 classpath 补全, 不是反编译缺陷。
		depGlob: []string{
			"org/jetbrains/kotlin/kotlin-stdlib/*/kotlin-stdlib-*.jar",
		},
	},
	"codec": {
		relPath: "commons-codec/commons-codec/1.15/commons-codec-1.15.jar",
	},
	"spring": {
		relPath: "org/springframework/spring-core/5.3.27/spring-core-5.3.27.jar",
		// spring-core 的 gradle 元数据把 reactor / kotlin / rxjava / mutiny / netty / aspectj /
		// jopt-simple / ant 全部标为 optional 依赖 (只有用到对应特性时才需要)。这些类被忠实反编译
		// 后必须 import 这些包, 缺失会误报 "package ... does not exist" / "cannot find symbol",
		// 与 guava 的 sun.misc.Unsafe、fastjson2 的 kotlin 同属 classpath 补全而非反编译缺陷。
		// jdk.jfr 是 JDK 内部模块, 在 --release 8 下不可见, 由 withJfr shim 补 (见 jdk_jfr_test.go)。
		depGlob: []string{
			"org/springframework/spring-jcl/5.3.27/spring-jcl-5.3.27.jar",
			"io/projectreactor/reactor-core/*/reactor-core-*.jar",
			"org/reactivestreams/reactive-streams/*/reactive-streams-*.jar",
			"io/netty/netty-buffer/*/netty-buffer-*.jar",
			"io/netty/netty-common/*/netty-common-*.jar",
			"org/jetbrains/kotlin/kotlin-stdlib/*/kotlin-stdlib-*.jar",
			"org/jetbrains/kotlin/kotlin-reflect/*/kotlin-reflect-*.jar",
			"org/jetbrains/kotlinx/kotlinx-coroutines-core-jvm/*/kotlinx-coroutines-core-jvm-*.jar",
			"org/jetbrains/kotlinx/kotlinx-coroutines-reactor/*/kotlinx-coroutines-reactor-*.jar",
			"io/reactivex/rxjava/*/rxjava-*.jar",
			"io/reactivex/rxjava2/rxjava/*/rxjava-*.jar",
			"io/reactivex/rxjava3/rxjava/*/rxjava-*.jar",
			// Mutiny 2.x switched publisher()/toPublisher() to java.util.concurrent.Flow.Publisher;
			// spring-core 5.3.27 was compiled against Mutiny 1.x (org.reactivestreams.Publisher).
			// A wildcard that picks 2.x is an ENVIRONMENT false-positive, same class as sun.misc.
			"io/smallrye/reactive/mutiny/1.*/mutiny-*.jar",
			"net/sf/jopt-simple/jopt-simple/*/jopt-simple-*.jar",
			"org/apache/ant/ant/*/ant-*.jar",
			"org/aspectj/aspectjweaver/*/aspectjweaver-*.jar",
			"org/jetbrains/kotlinx/kotlinx-coroutines-reactive/*/kotlinx-coroutines-reactive-*.jar",
			"io/reactivex/rxjava-reactive-streams/*/rxjava-reactive-streams-*.jar",
			"org/jetbrains/annotations/*/annotations-*.jar",
			"io/projectreactor/tools/blockhound/*/blockhound-*.jar",
			"com/google/code/findbugs/jsr305/3.0.2/jsr305-3.0.2.jar",
		},
	},
	// 以下四个与 benchmark_test.go 的 benchmarkJars 对齐, 使 tree/iso inventory 也能对它们分桶选靶。
	"gson": {
		relPath: "com/google/code/gson/gson/2.8.9/gson-2.8.9.jar",
	},
	"commons-lang3": {
		relPath: "org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.jar",
	},
	"jsoup": {
		relPath: "org/jsoup/jsoup/1.10.2/jsoup-1.10.2.jar",
	},
	"snakeyaml": {
		relPath: "org/yaml/snakeyaml/2.2/snakeyaml-2.2.jar",
	},
	// Six expansion jars (v0.2+ coverage). Optional/native packages on the decompiled
	// import list are classpath completion, not decompile defects (same class as
	// spring reactor / guava sun.misc / Mutiny 1.x pin).
	"jackson": {
		relPath: "com/fasterxml/jackson/core/jackson-databind/2.15.4/jackson-databind-2.15.4.jar",
		depGlob: []string{
			"com/fasterxml/jackson/core/jackson-core/2.15.4/jackson-core-2.15.4.jar",
			"com/fasterxml/jackson/core/jackson-annotations/2.15.4/jackson-annotations-2.15.4.jar",
			"com/fasterxml/jackson/datatype/jackson-datatype-jsr310/2.15.4/jackson-datatype-jsr310-2.15.4.jar",
			"com/fasterxml/jackson/datatype/jackson-datatype-jdk8/*/jackson-datatype-jdk8-*.jar",
			"com/google/code/findbugs/jsr305/*/jsr305-*.jar",
		},
	},
	"okhttp": {
		// Java 3.x, not Kotlin 4.x (4.x + kotlin-stdlib miss is an environment false-positive).
		// android.* / org.conscrypt are optional platform packages (Android SDK / native TLS);
		// missing them is an environment false-positive, completed by withOptionalPlatforms
		// (see optional_platform_shim_test.go). Android10Platform's Java 9 SSL ALPN calls are
		// compiled at --release 9 (see mrFileRelease / treeCompileToDir).
		relPath: "com/squareup/okhttp3/okhttp/3.14.9/okhttp-3.14.9.jar",
		depGlob: []string{
			"com/squareup/okio/okio/1.17.2/okio-1.17.2.jar",
			"com/google/code/findbugs/jsr305/*/jsr305-*.jar",
			"org/codehaus/mojo/animal-sniffer-annotations/*/animal-sniffer-annotations-*.jar",
			"org/conscrypt/conscrypt-openjdk-uber/*/conscrypt-openjdk-uber-*.jar",
		},
	},
	"netty": {
		relPath: "io/netty/netty-handler/4.1.108.Final/netty-handler-4.1.108.Final.jar",
		depGlob: []string{
			"io/netty/netty-common/4.1.108.Final/netty-common-4.1.108.Final.jar",
			"io/netty/netty-buffer/4.1.108.Final/netty-buffer-4.1.108.Final.jar",
			"io/netty/netty-transport/4.1.108.Final/netty-transport-4.1.108.Final.jar",
			"io/netty/netty-codec/4.1.108.Final/netty-codec-4.1.108.Final.jar",
			"io/netty/netty-resolver/4.1.108.Final/netty-resolver-4.1.108.Final.jar",
			"io/netty/netty-transport-native-unix-common/4.1.108.Final/netty-transport-native-unix-common-4.1.108.Final.jar",
			"io/netty/netty-transport-classes-epoll/4.1.108.Final/netty-transport-classes-epoll-4.1.108.Final.jar",
			"io/netty/netty-transport-classes-kqueue/4.1.108.Final/netty-transport-classes-kqueue-4.1.108.Final.jar",
			"io/netty/netty-tcnative-classes/*/netty-tcnative-classes-*.jar",
			"org/bouncycastle/bcpkix-jdk15on/*/bcpkix-jdk15on-*.jar",
			"org/bouncycastle/bcprov-jdk15on/*/bcprov-jdk15on-*.jar",
			"org/bouncycastle/bctls-jdk15on/*/bctls-jdk15on-*.jar",
			"org/slf4j/slf4j-api/*/slf4j-api-*.jar",
		},
	},
	"log4j": {
		// Optional plugins (jms/mail/jansi/disruptor/kafka/csv/jeromq/osgi annotations,
		// findbugs, stax2) are compile-time deps of faithfully decompiled sources.
		// Missing them is an environment false-positive, completed here via ~/.m2
		// (same class as netty conscrypt/jetty and okhttp android.*).
		relPath: "org/apache/logging/log4j/log4j-core/2.23.1/log4j-core-2.23.1.jar",
		depGlob: []string{
			"org/apache/logging/log4j/log4j-api/2.23.1/log4j-api-2.23.1.jar",
			"org/slf4j/slf4j-api/*/slf4j-api-*.jar",
			"org/jctools/jctools-core/*/jctools-core-*.jar",
			"com/fasterxml/jackson/core/jackson-core/2.15.4/jackson-core-2.15.4.jar",
			"com/fasterxml/jackson/core/jackson-databind/2.15.4/jackson-databind-2.15.4.jar",
			"com/fasterxml/jackson/core/jackson-annotations/2.15.4/jackson-annotations-2.15.4.jar",
			"com/fasterxml/jackson/dataformat/jackson-dataformat-yaml/*/jackson-dataformat-yaml-*.jar",
			"com/fasterxml/jackson/dataformat/jackson-dataformat-xml/*/jackson-dataformat-xml-*.jar",
			"org/yaml/snakeyaml/*/snakeyaml-*.jar",
			"commons-codec/commons-codec/*/commons-codec-*.jar",
			"org/apache/commons/commons-compress/*/commons-compress-*.jar",
			"org/apache/commons/commons-csv/*/commons-csv-*.jar",
			"org/fusesource/jansi/jansi/*/jansi-*.jar",
			"org/osgi/org.osgi.core/*/org.osgi.core-*.jar",
			"org/osgi/org.osgi.annotation.bundle/*/org.osgi.annotation.bundle-*.jar",
			"org/osgi/org.osgi.annotation.versioning/*/org.osgi.annotation.versioning-*.jar",
			"org/osgi/osgi.annotation/*/osgi.annotation-*.jar",
			"javax/activation/javax.activation-api/*/javax.activation-api-*.jar",
			"javax/mail/javax.mail-api/*/javax.mail-api-*.jar",
			"com/sun/mail/javax.mail/*/javax.mail-*.jar",
			"javax/jms/javax.jms-api/*/javax.jms-api-*.jar",
			"com/lmax/disruptor/*/disruptor-*.jar",
			"com/conversantmedia/disruptor/*/disruptor-*.jar",
			"org/apache/kafka/kafka-clients/*/kafka-clients-*.jar",
			"org/zeromq/jeromq/*/jeromq-*.jar",
			"com/google/code/findbugs/annotations/*/annotations-*.jar",
			"com/github/spotbugs/spotbugs-annotations/*/spotbugs-annotations-*.jar",
			"org/codehaus/woodstox/stax2-api/*/stax2-api-*.jar",
			"biz/aQute/bnd/biz.aQute.bnd.annotation/*/biz.aQute.bnd.annotation-*.jar",
		},
	},
	"protobuf": {
		relPath: "com/google/protobuf/protobuf-java/3.21.9/protobuf-java-3.21.9.jar",
	},
	"collections4": {
		relPath: "org/apache/commons/commons-collections4/4.4/commons-collections4-4.4.jar",
	},
	// Twenty typical-library expansion (tree coverage). Optional plugin / native /
	// metrics packages on the decompiled import list are classpath completion, not
	// decompile defects. Java 11+ jars (logback 1.4, HikariCP 5, freemarker
	// _Java16Impl) pick --release from jarBaseRelease, not a hardcoded 8.
	"asm": {
		relPath: "org/ow2/asm/asm/9.7/asm-9.7.jar",
	},
	"joda-time": {
		relPath: "joda-time/joda-time/2.10.13/joda-time-2.10.13.jar",
		depGlob: []string{
			"org/joda/joda-convert/*/joda-convert-*.jar",
		},
	},
	"commons-io": {
		relPath: "commons-io/commons-io/2.16.0/commons-io-2.16.0.jar",
	},
	"compress": {
		relPath: "org/apache/commons/commons-compress/1.26.2/commons-compress-1.26.2.jar",
		depGlob: []string{
			"org/tukaani/xz/*/xz-*.jar",
			"com/github/luben/zstd-jni/*/zstd-jni-*.jar",
			"com/aayushatharva/brotli4j/brotli4j/*/brotli4j-*.jar",
			"org/brotli/dec/*/dec-*.jar",
			"org/xerial/snappy/snappy-java/*/snappy-java-*.jar",
			// Pin 1.16.1: string-sort last match is 1.9, which lacks XXHash32 /
			// PureJavaCrc32C that commons-compress 1.26.2's decompiled sources use.
			"commons-codec/commons-codec/1.16.1/commons-codec-1.16.1.jar",
			// Pin 3.14.0: string-sort last match is 3.9, which lacks ArrayFill
			// that commons-compress 1.26.2's decompiled sources call.
			"org/apache/commons/commons-lang3/3.14.0/commons-lang3-3.14.0.jar",
			"org/osgi/org.osgi.core/*/org.osgi.core-*.jar",
			"org/osgi/osgi.annotation/*/osgi.annotation-*.jar",
			// Pin 2.16.0: the wildcard's string-sort last match is 2.9.0, which
			// lacks org.apache.commons.io.build / file.attribute.FileTimes that
			// commons-compress 1.26.2's decompiled sources import.
			"commons-io/commons-io/2.16.0/commons-io-2.16.0.jar",
			"org/ow2/asm/asm/*/asm-*.jar",
		},
	},
	"httpclient": {
		relPath: "org/apache/httpcomponents/httpclient/4.5.14/httpclient-4.5.14.jar",
		depGlob: []string{
			"org/apache/httpcomponents/httpcore/4.4.*/httpcore-4.4.*.jar",
			"commons-logging/commons-logging/1.2/commons-logging-1.2.jar",
			"commons-codec/commons-codec/*/commons-codec-*.jar",
		},
	},
	"slf4j": {
		relPath: "org/slf4j/slf4j-api/2.0.13/slf4j-api-2.0.13.jar",
	},
	"logback": {
		relPath: "ch/qos/logback/logback-core/1.4.14/logback-core-1.4.14.jar",
		depGlob: []string{
			"org/slf4j/slf4j-api/*/slf4j-api-*.jar",
			"org/codehaus/janino/janino/*/janino-*.jar",
			"org/codehaus/janino/commons-compiler/*/commons-compiler-*.jar",
			"jakarta/mail/jakarta.mail-api/*/jakarta.mail-api-*.jar",
			"jakarta/activation/jakarta.activation-api/*/jakarta.activation-api-*.jar",
			"javax/mail/javax.mail-api/*/javax.mail-api-*.jar",
			"jakarta/servlet/jakarta.servlet-api/*/jakarta.servlet-api-*.jar",
			"javax/servlet/javax.servlet-api/*/javax.servlet-api-*.jar",
			"org/apache/groovy/groovy/*/groovy-*.jar",
			"com/sun/mail/jakarta.mail/*/jakarta.mail-*.jar",
		},
	},
	"caffeine": {
		relPath: "com/github/ben-manes/caffeine/caffeine/2.9.3/caffeine-2.9.3.jar",
		depGlob: []string{
			"org/checkerframework/checker-qual/*/checker-qual-*.jar",
			"com/google/errorprone/error_prone_annotations/*/error_prone_annotations-*.jar",
			"com/google/code/findbugs/jsr305/*/jsr305-*.jar",
		},
	},
	"rxjava": {
		relPath: "io/reactivex/rxjava2/rxjava/2.2.21/rxjava-2.2.21.jar",
		depGlob: []string{
			"org/reactivestreams/reactive-streams/*/reactive-streams-*.jar",
		},
	},
	"javassist": {
		relPath: "org/javassist/javassist/3.30.2-GA/javassist-3.30.2-GA.jar",
		// Bytecode is Java 8 but the decompiled source calls Java 9+ APIs
		// (ClassLoader.getDefinedPackage, Module, MethodHandles.privateLookupIn)
		// behind runtime MAJOR_VERSION gates. javac --release 8 type-checks both
		// branches and reports those as missing symbols.
		minRelease: 11,
	},
	"xstream": {
		relPath: "com/thoughtworks/xstream/xstream/1.4.20/xstream-1.4.20.jar",
		depGlob: []string{
			"xmlpull/xmlpull/*/xmlpull-*.jar",
			"io/github/x-stream/mxparser/*/mxparser-*.jar",
			"xpp3/xpp3_min/*/xpp3_min-*.jar",
			"org/dom4j/dom4j/*/dom4j-*.jar",
			"org/jdom/jdom2/*/jdom2-*.jar",
			"org/codehaus/jettison/jettison/*/jettison-*.jar",
			"joda-time/joda-time/*/joda-time-*.jar",
			"cglib/cglib/*/cglib-*.jar",
			"org/jdom/jdom/1.*/jdom-1.*.jar",
			"com/fasterxml/woodstox/woodstox-core/*/woodstox-core-*.jar",
			"org/codehaus/woodstox/stax2-api/*/stax2-api-*.jar",
			"net/sf/kxml/kxml2/*/kxml2-*.jar",
			"xom/xom/*/xom-*.jar",
			"stax/stax/*/stax-*.jar", // BEA StAX RI: com.bea.xml.stream
		},
	},
	"math3": {
		relPath: "org/apache/commons/commons-math3/3.6.1/commons-math3-3.6.1.jar",
	},
	"hikaricp": {
		relPath: "com/zaxxer/HikariCP/5.0.1/HikariCP-5.0.1.jar",
		depGlob: []string{
			"org/slf4j/slf4j-api/*/slf4j-api-*.jar",
			"org/javassist/javassist/*/javassist-*.jar",
			"io/micrometer/micrometer-core/*/micrometer-core-*.jar",
			"io/dropwizard/metrics/metrics-core/*/metrics-core-*.jar",
			"io/prometheus/simpleclient/*/simpleclient-*.jar",
			"org/hibernate/hibernate-core/*/hibernate-core-*.jar",
			"javax/persistence/javax.persistence-api/*/javax.persistence-api-*.jar",
			"jakarta/persistence/jakarta.persistence-api/*/jakarta.persistence-api-*.jar",
			"io/dropwizard/metrics/metrics-healthchecks/*/metrics-healthchecks-*.jar",
		},
	},
	"jedis": {
		relPath: "redis/clients/jedis/3.8.0/jedis-3.8.0.jar",
		depGlob: []string{
			"org/apache/commons/commons-pool2/*/commons-pool2-*.jar",
			"org/slf4j/slf4j-api/*/slf4j-api-*.jar",
			"org/json/json/*/json-*.jar",
			"com/google/code/gson/gson/*/gson-*.jar",
			"commons-codec/commons-codec/*/commons-codec-*.jar",
		},
	},
	"junit": {
		relPath: "junit/junit/4.13.2/junit-4.13.2.jar",
		depGlob: []string{
			// junit 4.13.2 is compiled against hamcrest-core 1.3 (Factory annotation,
			// everyItem → Matcher<Iterable<T>>). A hamcrest 2.x jar on the tree CP
			// drops Factory and changes everyItem to Matcher<Iterable<? extends U>>.
			"org/hamcrest/hamcrest-core/1.3/hamcrest-core-1.3.jar",
		},
	},
	"assertj": {
		relPath: "org/assertj/assertj-core/3.24.2/assertj-core-3.24.2.jar",
		depGlob: []string{
			// string-sort of * picks 1.9.13 over 1.14.x, and 1.9 has no namedOneOf.
			"net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar",
			"org/hamcrest/hamcrest/*/hamcrest-*.jar",
			"org/hamcrest/hamcrest-core/*/hamcrest-core-*.jar",
			"junit/junit/*/junit-*.jar",
			"org/junit/jupiter/junit-jupiter-api/*/junit-jupiter-api-*.jar",
			"org/opentest4j/opentest4j/*/opentest4j-*.jar",
			"org/junit/platform/junit-platform-commons/*/junit-platform-commons-*.jar",
		},
	},
	"picocli": {
		relPath: "info/picocli/picocli/4.3.2/picocli-4.3.2.jar",
	},
	"pool2": {
		relPath: "org/apache/commons/commons-pool2/2.11.1/commons-pool2-2.11.1.jar",
		depGlob: []string{
			"cglib/cglib/*/cglib-*.jar",
		},
	},
	"zxing": {
		relPath: "com/google/zxing/core/3.3.3/core-3.3.3.jar",
	},
	// Typical-hard expansion (control-flow / generics / bytecode-enhancement).
	// Not in the original 34. Optional plugin packages on the decompiled import
	// list are classpath completion, same class as spring reactor / okhttp android.
	"bytebuddy": {
		relPath: "net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar",
		depGlob: []string{
			"com/google/code/findbugs/annotations/*/annotations-*.jar",
			"com/github/spotbugs/spotbugs-annotations/*/spotbugs-annotations-*.jar",
			"com/google/code/findbugs/jsr305/*/jsr305-*.jar",
			"net/java/dev/jna/jna/*/jna-*.jar",
			"net/bytebuddy/byte-buddy-agent/1.12.23/byte-buddy-agent-1.12.23.jar",
		},
	},
	"mockito": {
		relPath: "org/mockito/mockito-core/4.5.1/mockito-core-4.5.1.jar",
		depGlob: []string{
			// string-sort of 1.* would pick 1.9.13; pin the 1.12 line mockito 4.5 was built with.
			"net/bytebuddy/byte-buddy/1.12.23/byte-buddy-1.12.23.jar",
			"net/bytebuddy/byte-buddy-agent/1.12.23/byte-buddy-agent-1.12.23.jar",
			"org/objenesis/objenesis/3.2/objenesis-3.2.jar",
			"org/opentest4j/opentest4j/*/opentest4j-*.jar",
			"junit/junit/*/junit-*.jar",
			"org/hamcrest/hamcrest-core/*/hamcrest-core-*.jar",
			"org/hamcrest/hamcrest/*/hamcrest-*.jar",
		},
	},
	"spring-beans": {
		relPath: "org/springframework/spring-beans/5.3.27/spring-beans-5.3.27.jar",
		depGlob: []string{
			"org/springframework/spring-core/5.3.27/spring-core-5.3.27.jar",
			"org/springframework/spring-jcl/5.3.27/spring-jcl-5.3.27.jar",
			"org/yaml/snakeyaml/2.2/snakeyaml-2.2.jar",
			"javax/inject/javax.inject/*/javax.inject-*.jar",
			"jakarta/inject/jakarta.inject-api/*/jakarta.inject-api-*.jar",
			// Spring 5.3 GroovyBeanDefinitionReader is Groovy 2.x; 4.x is an
			// environment false-positive (same class as Mutiny 2.x vs 1.x).
			"org/codehaus/groovy/groovy/2.5.14/groovy-2.5.14.jar",
			// StreamingMarkupBuilder lives in groovy-xml, not groovy-core.
			"org/codehaus/groovy/groovy-xml/2.5.14/groovy-xml-2.5.14.jar",
			"org/jetbrains/kotlin/kotlin-stdlib/*/kotlin-stdlib-*.jar",
			"org/jetbrains/kotlin/kotlin-reflect/*/kotlin-reflect-*.jar",
			"javax/validation/validation-api/*/validation-api-*.jar",
			"jakarta/validation/jakarta.validation-api/*/jakarta.validation-api-*.jar",
			"com/google/code/findbugs/jsr305/*/jsr305-*.jar",
		},
	},
	"lucene": {
		relPath: "org/apache/lucene/lucene-core/8.11.1/lucene-core-8.11.1.jar",
	},
	"freemarker": {
		relPath: "org/freemarker/freemarker/2.3.33/freemarker-2.3.33.jar",
		depGlob: []string{
			"javax/servlet/javax.servlet-api/*/javax.servlet-api-*.jar",
			"jakarta/servlet/jakarta.servlet-api/*/jakarta.servlet-api-*.jar",
			"javax/xml/bind/jaxb-api/*/jaxb-api-*.jar",
			"jakarta/xml/bind/jakarta.xml.bind-api/*/jakarta.xml.bind-api-*.jar",
			"org/apache/logging/log4j/log4j-api/*/log4j-api-*.jar",
			"commons-logging/commons-logging/1.2/commons-logging-1.2.jar",
			"org/slf4j/slf4j-api/*/slf4j-api-*.jar",
			"org/apache/ant/ant/*/ant-*.jar",
			"org/jdom/jdom/1.*/jdom-1.*.jar",
			"org/dom4j/dom4j/*/dom4j-*.jar",
			"jaxen/jaxen/*/jaxen-*.jar",
			"org/python/jython-standalone/*/jython-standalone-*.jar",
			"org/mozilla/rhino/*/rhino-*.jar",
			"log4j/log4j/1.2.17/log4j-1.2.17.jar",
			"jakarta/el/jakarta.el-api/*/jakarta.el-api-*.jar",
			"javax/el/javax.el-api/*/javax.el-api-*.jar",
			"jakarta/servlet/jsp/jakarta.servlet.jsp-api/*/jakarta.servlet.jsp-api-*.jar",
			"javax/servlet/jsp/jsp-api/*/jsp-api-*.jar",
			"xalan/xalan/*/xalan-*.jar",
		},
	},
}

func m2Repo() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".m2", "repository")
}

// compileRelease is the javac --release for a jar: max(jarBaseRelease, spec.minRelease).
func compileRelease(spec jarSpec, jarPath string) int {
	rel := jarBaseRelease(jarPath)
	if spec.minRelease > rel {
		return spec.minRelease
	}
	return rel
}

// resolveJar returns the absolute path of relPath under ~/.m2/repository, or "" when absent.
func resolveJar(relPath string) string {
	p := filepath.Join(m2Repo(), filepath.FromSlash(relPath))
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// resolveDeps globs each dep pattern under ~/.m2/repository and returns the first match for each.
func resolveDeps(globs []string) []string {
	var out []string
	for _, g := range globs {
		matches, _ := filepath.Glob(filepath.Join(m2Repo(), filepath.FromSlash(g)))
		if len(matches) > 0 {
			sort.Strings(matches)
			out = append(out, matches[len(matches)-1]) // 取版本号最大的一个
		}
	}
	return out
}

// classEntries lists every *.class entry name (slash form) inside jarPath.
func classEntries(t *testing.T, jarPath string) []string {
	t.Helper()
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Fatalf("open jar %s: %v", jarPath, err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, ".class") && !strings.HasSuffix(f.Name, "module-info.class") {
			names = append(names, f.Name)
			continue
		}
		// Mockito stores bootstrap-injected classes as CAFEBABE `.raw` (MockMethodDispatcher).
		if strings.HasSuffix(f.Name, ".raw") {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			hdr := make([]byte, 4)
			_, err = io.ReadFull(rc, hdr)
			rc.Close()
			if err == nil && hdr[0] == 0xca && hdr[1] == 0xfe && hdr[2] == 0xba && hdr[3] == 0xbe {
				names = append(names, f.Name)
			}
		}
	}
	sort.Strings(names)
	return names
}

// decompileFailedMarker reports whether src is a decompile-failure sentinel comment (the
// decompileClassBytes fallback). Such a "source" is just a comment and would silently compile to
// nothing, so it must be counted as a failure rather than a success.
func decompileFailedMarker(src string) bool {
	s := strings.TrimSpace(src)
	return strings.HasPrefix(s, "// decompile parse failed") ||
		strings.HasPrefix(s, "// decompile dump failed")
}

// enumFoldSuppressed reports whether src is the synthetic enum-subclass suppression marker (folded
// into its enum, regenerated by javac). These are intentional no-ops and excluded from the unit set.
func enumFoldSuppressed(src string) bool {
	return strings.Contains(src, "synthetic enum constant-body subclass folded into enum")
}

type recompileResult struct {
	units      int // 参与编译的单元数 (排除 enum-fold 抑制单元)
	decErr     int // 编译失败的单元数 (iso) 或 javac 报告的 error 行数 (tree)
	decompFail int // 反编译本身失败 (sentinel comment) 的单元数, 计入 decErr
}

// decompileAll decompiles every class entry via the production JarFS path and writes each unit to
// <root>/<package path>/<SimpleName>.java. Returns the written file paths and the unit/fail counts.
func decompileAll(t *testing.T, jarPath, root string, maxFiles int) (files []string, units, decompFail int) {
	t.Helper()
	jfs, err := classparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("NewJarFSFromLocal %s: %v", jarPath, err)
	}
	// Release the jar's file handle on return. On Windows an open handle blocks the caller's
	// t.TempDir() RemoveAll cleanup ("file is being used by another process"), which fails the test
	// even after a successful round-trip (observed on TestSyntheticJarRoundTrip, windows-latest).
	defer jfs.Close()
	entries := classEntries(t, jarPath)
	if maxFiles > 0 && len(entries) > maxFiles {
		entries = entries[:maxFiles]
	}
	for _, entry := range entries {
		raw, err := jfs.ReadFile(entry)
		if err != nil {
			// ReadFile maps a class entry to its decompiled source; a read error is itself a failure.
			decompFail++
			continue
		}
		src := string(raw)
		if enumFoldSuppressed(src) {
			continue // 折叠进 enum, 不算独立单元
		}
		units++
		if decompileFailedMarker(src) {
			decompFail++
			// 仍写出 (会编译失败), 让 iso 计数把它算进 decErr
		}
		// entry 形如 com/google/common/math/LongMath$1.class (or mockito .raw class bytes)
		rel := strings.TrimSuffix(strings.TrimSuffix(entry, ".class"), ".raw") + ".java" // 保留扁平 $ 名
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(dst, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
		files = append(files, dst)
	}
	return files, units, decompFail
}

// recompileISO compiles each file in isolation (deps + original jar on classpath) in parallel and
// returns the number of units that fail to compile. This is the un-masked, authoritative metric.
func recompileISO(t *testing.T, files []string, classpath string, workers, minRelease int) int {
	t.Helper()
	javac := lookJavac(t)
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		failed int
		ch     = make(chan string, len(files))
	)
	for _, f := range files {
		ch <- f
	}
	close(ch)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outDir := t.TempDir()
			for f := range ch {
				ctx, cancel := context.WithTimeout(context.Background(), compileTimeout)
				// Multi-Release versioned units compile under their own --release N (see mrFileRelease).
				args := append(append([]string{}, javacLocaleArgs...),
					"-encoding", "UTF-8", "--release", strconv.Itoa(mrFileRelease(f, minRelease)), "-nowarn",
					"-cp", classpath, "-d", outDir, f)
				cmd := exec.CommandContext(ctx, javac, args...)
				// Run javac from the throwaway out dir: with a long deps+jar classpath the JDK
				// launcher auto-spills a `javac.<ts>.args` argfile into the CWD, which would
				// otherwise litter test/cross. Mirrors recompileTree. f and classpath are absolute.
				cmd.Dir = outDir
				err := cmd.Run()
				cancel()
				if err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	return failed
}

// recompileTree compiles all files in one javac invocation (deps on classpath) and returns the
// number of error lines javac reports. This is fast but masked; use only to find the biggest levers.
// Multi-Release `META-INF/versions/N/` units get their own `--release N` pass via treeCompileToDir.
func recompileTree(t *testing.T, files []string, classpath string, minRelease int) int {
	t.Helper()
	errc, _ := treeCompileToDirAt(t, files, classpath, t.TempDir(), minRelease)
	return errc
}

// runProfile decompiles+recompiles one jar under the current environment and returns the result.
func runProfile(t *testing.T, name string, maxFiles int) recompileResult {
	t.Helper()
	spec, ok := jarSpecs[name]
	if !ok {
		t.Fatalf("unknown jar %q (have: %v)", name, jarKeys())
	}
	jarPath := resolveJar(spec.relPath)
	if jarPath == "" {
		t.Skipf("jar %s not found under %s; skipping", spec.relPath, m2Repo())
	}
	deps := resolveDeps(spec.depGlob)

	root := t.TempDir()
	files, units, decompFail := decompileAll(t, jarPath, root, maxFiles)

	mode := os.Getenv("RECOMPILE_MODE")
	if mode == "" {
		mode = "iso"
	}
	workers, _ := strconv.Atoi(os.Getenv("RECOMPILE_WORKERS"))

	rel := compileRelease(spec, jarPath)
	var decErr int
	switch mode {
	case "tree":
		cp := withEnvShims(t, strings.Join(deps, string(os.PathListSeparator)))
		decErr = recompileTree(t, files, cp, rel)
	default: // iso
		cpParts := append([]string{jarPath}, deps...)
		cp := withEnvShims(t, strings.Join(cpParts, string(os.PathListSeparator)))
		decErr = recompileISO(t, files, cp, workers, rel)
	}
	return recompileResult{units: units, decErr: decErr, decompFail: decompFail}
}

func jarKeys() []string {
	var ks []string
	for k := range jarSpecs {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// TestJarRecompileProfile measures decompile->recompile error counts for one jar (or all). It is the
// baseline-capture tool: run it per jar to snapshot decErr, then compare across phases.
func TestJarRecompileProfile(t *testing.T) {
	target := os.Getenv("PROFILE_JAR")
	if target == "" {
		t.Skip("set PROFILE_JAR=<guava|fastjson2|codec|spring|all> to run the recompile profile")
	}
	lookJavac(t)
	maxFiles, _ := strconv.Atoi(os.Getenv("MAXFILES"))

	names := []string{target}
	if target == "all" {
		names = jarKeys()
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			res := runProfile(t, name, maxFiles)
			t.Logf("[%s] mode=%s units=%d decErr=%d decompileFail=%d",
				name, orDefault(os.Getenv("RECOMPILE_MODE"), "iso"), res.units, res.decErr, res.decompFail)
		})
	}
}

// TestJarRecompileDelta runs the A/B kill-switch comparison: pass A with the switch unset (fix ON =
// baseline), pass B with the switch set (fix OFF). A positive delta (B-A) is the fix's real benefit.
func TestJarRecompileDelta(t *testing.T) {
	target := os.Getenv("PROFILE_JAR")
	ks := os.Getenv("KILL_SWITCH")
	if target == "" || ks == "" {
		t.Skip("set PROFILE_JAR=<jar> and KILL_SWITCH=<JDEC_X> to run the A/B delta")
	}
	lookJavac(t)
	maxFiles, _ := strconv.Atoi(os.Getenv("MAXFILES"))

	names := []string{target}
	if target == "all" {
		names = jarKeys()
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			prev, had := os.LookupEnv(ks)
			defer func() {
				if had {
					os.Setenv(ks, prev)
				} else {
					os.Unsetenv(ks)
				}
			}()

			os.Unsetenv(ks) // 修复 ON (baseline)
			on := runProfile(t, name, maxFiles)
			os.Setenv(ks, "1") // 修复 OFF
			off := runProfile(t, name, maxFiles)

			delta := off.decErr - on.decErr
			t.Logf("[%s] %s ON(decErr)=%d OFF(decErr)=%d delta(OFF-ON)=%+d units=%d",
				name, ks, on.decErr, off.decErr, delta, on.units)
			if delta < 0 {
				t.Errorf("kill-switch %s shows fix INCREASED errors by %d (regression?): ON=%d OFF=%d",
					ks, -delta, on.decErr, off.decErr)
			}
		})
	}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
