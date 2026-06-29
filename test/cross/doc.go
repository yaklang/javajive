// Package cross holds cross-implementation tests: real Java artifacts are produced
// with the JDK (javac/java) at test time and then verified against javajive's
// pure-Go decompiler, class parser and serialization implementation.
//
// The tests skip automatically when no JDK is available on PATH, so the package
// still builds and `go test` stays green in environments without Java.
package cross
