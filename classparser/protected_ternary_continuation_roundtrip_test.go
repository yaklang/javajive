package javaclassparser

import (
	"strings"
	"testing"
)

const protectedTernaryContinuationFixture = `class RegionEffects{static String trace="";static final java.io.IOException failure=new java.io.IOException("original");static void inside(int mode)throws java.io.IOException{trace+="I";if(mode==1)throw failure;}static boolean condition(boolean select,int mode)throws java.io.IOException{trace+="C";if(mode==2)throw failure;return select;}static long arm(long value,int mode)throws java.io.IOException{trace+="A";if(mode==3)throw failure;return value;}}
class RegionOwner{private Object token;RegionOwner(Object token){this.token=token;}static class Reader{Object read(RegionOwner owner){return owner.token;}}long evaluate(boolean gate,boolean select,long value,int mode)throws java.io.IOException{long result=0;if(gate){try{RegionEffects.inside(mode);}catch(java.io.IOException e){RegionEffects.trace+="H";return -11;}result=RegionEffects.condition(select,mode)?RegionEffects.arm(value,mode):7;}return result;}}
class RegionDriver{public static void main(String[]args)throws Exception{RegionOwner owner=new RegionOwner(new Object());if(new RegionOwner.Reader().read(owner)==null)throw new AssertionError("binding");int rows=0;for(boolean gate:new boolean[]{false,true})for(boolean select:new boolean[]{false,true})for(long value:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(int mode:new int[]{0,1,2,3}){RegionEffects.trace="";boolean escaped=gate&&(mode==2||mode==3&&select);try{long got=owner.evaluate(gate,select,value,mode);long expected=!gate?0:mode==1?-11:select?value:7;if(escaped||got!=expected)throw new AssertionError("value/exception ownership");}catch(java.io.IOException e){if(!escaped||e!=RegionEffects.failure)throw new AssertionError("failure identity/domain");}String trace=!gate?"":mode==1?"IH":mode==2?"IC":select?"ICA":"IC";if(!RegionEffects.trace.equals(trace))throw new AssertionError("effect/handler order "+RegionEffects.trace);rows++;}System.out.println(rows+":protected:ternary:exclusive:domain:identity");}}`

func TestNativeProtectedTernaryContinuationOriginalDomainRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, protectedTernaryContinuationFixture, "RegionOwner", "RegionDriver", "48:protected:ternary:exclusive:domain:identity\n")
}
func TestNativeProtectedTernaryContinuationRenamedRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, strings.ReplaceAll(protectedTernaryContinuationFixture, "RegionOwner", "DifferentRegionScope"), "DifferentRegionScope", "RegionDriver", "48:protected:ternary:exclusive:domain:identity\n")
}
