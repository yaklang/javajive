package javaclassparser

import "testing"

// Two optional lookup paths reuse JVM slots for a name and a reader. Both
// String definitions remain branch-local even when the reader needs a wider
// declaration. The oracle observes lookup order, null results and failures.
func TestAdversarialDisjointGuardedLookupRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "DisjointLookup", `
public class DisjointLookup {
  static LookupReader pick(int kind,long hash) {
    if(LookupOracle.handler) {
      Class<?> type=LookupOracle.resolveHash(hash);
      if(type==null) {
        String name=LookupOracle.name();
        type=LookupOracle.resolveName(name);
      }
      if(type!=null) {
        LookupReader reader=LookupOracle.reader(type);
        return reader;
      }
    }
    if(!LookupOracle.allowed) {
      if(kind==0) return LookupOracle.fallback("object");
      if(kind==1) return LookupOracle.fallback("array");
      throw new IllegalArgumentException("disabled");
    } else {
      LookupReader reader=LookupOracle.lookupHash(hash);
      if(reader==null) {
        String name=LookupOracle.name();
        reader=LookupOracle.lookupName(name);
        if(reader==null) throw new IllegalArgumentException("missing:"+name);
      }
      return reader;
    }
  }
  public static void main(String[] args) { LookupOracle.run(); }
}
class LookupReader { final String value; LookupReader(String v){value=v;} }
class LookupOracle {
  static final LookupReader SHARED=new LookupReader("shared");
  static boolean handler,allowed,hashFound,nameFound;
  static StringBuilder trace;static int calls,failAt;
  static void mark(String s){trace.append(s).append(';');if(++calls==failAt)throw new IllegalStateException(s);}
  static Class<?> resolveHash(long hash){mark("class-hash:"+hash);return hashFound ? String.class : null;}
  static Class<?> resolveName(String name){mark("class-name:"+name);return nameFound ? Integer.class : null;}
  static String name(){mark("name");return nameFound ? "known" : null;}
  static LookupReader reader(Class<?> type){mark("reader:"+type.getName());return SHARED;}
  static LookupReader lookupHash(long hash){mark("reader-hash:"+hash);return hashFound ? SHARED : null;}
  static LookupReader lookupName(String name){mark("reader-name:"+name);return nameFound ? new LookupReader(name) : null;}
  static LookupReader fallback(String kind){mark("fallback:"+kind);return new LookupReader(kind);}
  static void run(){
    for(int flags=0;flags<16;flags++)for(int kind=0;kind<3;kind++)for(int failure:new int[]{0,1,3,5}) {
      handler=(flags&1)!=0;allowed=(flags&2)!=0;hashFound=(flags&4)!=0;nameFound=(flags&8)!=0;
      trace=new StringBuilder();calls=0;failAt=failure;String result;
      try { LookupReader reader=DisjointLookup.pick(kind,17L);result=reader.value+":"+(reader==SHARED); }
      catch(Throwable e){result=e.getClass().getSimpleName()+":"+e.getMessage();}
      System.out.println(flags+":"+kind+":"+failure+":"+result+":"+calls+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
