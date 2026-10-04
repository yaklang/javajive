package javaclassparser

import "testing"

func TestAdversarialCachedFieldDefUseRoundTrip(t *testing.T) {
	t.Parallel()
	// Sentinel-result cache lowering must preserve producer count, cached object
	// identity, array identity, retry after failure, and checked-load fallback.
	roundTripGenericFlow(t, "CacheDefUseReview", `
import java.io.IOException;
class ReviewedCacheProducer {
 int calls;final Object value=new String("made");final Object fallback=new String("fallback");final Object[] array=new Object[]{value};final IOException failure=new IOException("producer");
 Object create(int mode)throws IOException{calls++;if(mode==2)throw failure;return mode==1?null:value;}
 Object[] createArray(int mode)throws IOException{calls++;if(mode==2)throw failure;return mode==1?null:array;}
 Object load(int mode)throws ClassNotFoundException{calls++;if(mode==2)throw new ClassNotFoundException("missing");return mode==1?null:value;}
}
public class CacheDefUseReview {
 final ReviewedCacheProducer producer=new ReviewedCacheProducer();Object cached;Object[] cachedArray;Object cachedFallback;
 Object read(int mode)throws IOException{Object result=this.cached!=null?null:producer.create(mode);if(result==null)result=this.cached;else this.cached=result;return result;}
 Object[] array(int mode)throws IOException{Object[] result=this.cachedArray!=null?null:producer.createArray(mode);if(result==null)result=this.cachedArray;else this.cachedArray=result;return result;}
 Object fallback(int mode){Object result=null;if(this.cachedFallback!=null)result=null;else{Object original=producer.fallback;try{result=producer.load(mode);}catch(ClassNotFoundException failure){result=original;}}if(result==null)result=this.cachedFallback;else this.cachedFallback=result;return result;}
 static String object(int mode,boolean seeded){CacheDefUseReview c=new CacheDefUseReview();Object seed=new String("seed");if(seeded)c.cached=seed;Object first=null;String trace="";try{first=c.read(mode);trace="ok:"+(first==c.producer.value)+":"+(first==seed)+":"+(first==null);}catch(IOException e){trace="fail:"+(e==c.producer.failure)+":"+(c.cached==null);}try{Object next=c.read(0);return trace+":"+(next==first)+":"+(next==c.producer.value)+":"+(next==seed)+":"+c.producer.calls;}catch(IOException e){throw new AssertionError(e);}}
 static String arrays(int mode,boolean seeded){CacheDefUseReview c=new CacheDefUseReview();Object[] seed=new Object[0];if(seeded)c.cachedArray=seed;Object[] first=null;String trace="";try{first=c.array(mode);trace="ok:"+(first==c.producer.array)+":"+(first==seed)+":"+(first==null);}catch(IOException e){trace="fail:"+(e==c.producer.failure)+":"+(c.cachedArray==null);}try{Object[] next=c.array(0);return trace+":"+(next==first)+":"+(next==c.producer.array)+":"+(next==seed)+":"+next.length+":"+c.producer.calls;}catch(IOException e){throw new AssertionError(e);}}
 static String fallbacks(int mode,boolean seeded){CacheDefUseReview c=new CacheDefUseReview();Object seed=new String("seed");if(seeded)c.cachedFallback=seed;Object first=c.fallback(mode);Object next=c.fallback(0);return (first==c.producer.fallback)+":"+(first==c.producer.value)+":"+(first==seed)+":"+(first==null)+":"+(next==first)+":"+c.producer.calls;}
 public static void main(String[] args){for(int mode=0;mode<3;mode++){System.out.println(object(mode,false));System.out.println(object(mode,true));System.out.println(arrays(mode,false));System.out.println(arrays(mode,true));System.out.println(fallbacks(mode,false));System.out.println(fallbacks(mode,true));}}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialIntentionalCheckedExceptionWrapperRoundTrip(t *testing.T) {
	t.Parallel()
	// Package and normal-path saved exception are deliberate counterexamples to
	// normalizing genuine wrappers by generated-variable-name coincidence.
	roundTripGenericFlow(t, "com.google.zxing.review.IntentionalWrapperReview", `
package com.google.zxing.review;
import java.io.IOException;
public class IntentionalWrapperReview {
 static final IOException original=new IOException("original");static String trace;
 static void work(int mode)throws IOException{trace+="W";if(mode!=0)throw original;}
 static void wrapped(int mode)throws IOException{IOException saved=original;if(mode==2){trace+="S";throw saved;}try{work(mode);}catch(IOException caught){trace+="C";throw new RuntimeException(caught);}}
 static String run(int mode){trace="";try{wrapped(mode);return "ok:"+trace;}catch(Throwable e){return e.getClass().getName()+":"+(e==original)+":"+(e.getCause()==original)+":"+e.getMessage()+":"+trace;}}
 public static void main(String[] args){for(int mode=0;mode<3;mode++)System.out.println(run(mode));}
}`, Precision, Compatibility, "legacy")
}
