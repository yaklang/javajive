package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This valid, authored program deliberately shares legacy recovery spellings.
// Neither spelling implies a Comparable bound nor String/Value storage. The
// original compiler and an identity oracle expose the removed text rewrites:
// one makes the Object instantiation illegal; the other inserts a failing cast.
func TestAdversarialLegacyGenericSpellingCannotInventBoundsOrCasts(t *testing.T) {
	javac, java := t04Tools(t)
	t.Setenv("JDEC_PROTOBUF_REMAINING_OFF", "")
	const fixture = `class Value {}
class SmallSortedMap<K,V> {}
final class SmallSortedMap$1<FieldDescriptorType> extends SmallSortedMap<FieldDescriptorType, Object> {}
public class Struct {
 Object key,value;
 Struct setKey(Object key){this.key=key;return this;}
 Struct setValue(Object value){this.value=value;return this;}
 static Struct fill(java.util.Map.Entry<Object,Object> var3){return new Struct()
 .setKey(var3.getKey()).setValue(var3.getValue());}
 public static void main(String[] args){
 new SmallSortedMap$1<Object>();int rows=0;
 for(Object key:new Object[]{null,"key",Integer.valueOf(17),new Object()})
 for(Object value:new Object[]{null,"text",new Value(),new Object()}){
 Struct result=fill(new java.util.AbstractMap.SimpleImmutableEntry<Object,Object>(key,value));
 if(result.key!=key||result.value!=value)throw new AssertionError("storage identity");rows++;
 }System.out.println(rows+":unbounded:storage:identity");}}
`
	for _, shape := range []string{"both", "bound only", "casts only", "renamed control"} {
		t.Run(shape, func(t *testing.T) {
			source, name := fixture, "Struct"
			if shape == "bound only" || shape == "renamed control" {
				source = strings.ReplaceAll(source, "Struct", "OrdinaryStorage")
				name = "OrdinaryStorage"
			}
			if shape == "casts only" || shape == "renamed control" {
				source = strings.ReplaceAll(source, "SmallSortedMap", "OrdinaryMap")
			}
			run := func(text string) {
				t.Helper()
				dir := t.TempDir()
				path := filepath.Join(dir, name+".java")
				if err := os.WriteFile(path, []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, path).CombinedOutput(); err != nil {
					t.Fatal("authored generic source", err, string(out))
				}
				if out, err := enumFreshOracleRun(t, java, "-ea", dir, name); err != nil || string(out) != "16:unbounded:storage:identity\n" {
					t.Fatal("storage oracle", err, string(out))
				}
			}
			run(source)
			run(fixProtobufRemainingReconstructs(source))
		})
	}
}
