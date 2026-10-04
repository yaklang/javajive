package javaclassparser

import "testing"

// The withheld marker interface is not required to interpret original throws
// ancestry or handler outcomes. A spurious bridge would require its unknown
// member namespace and incorrectly turn these expressible bodies into stubs.
func TestAdversarialUncheckedHandlerAndConstructorPrefixRoundTrip(t *testing.T) {
	roundTripGenericFlowWithResolverFilter(t, "CheckedAbsorptionReview", `import java.io.*;import java.lang.reflect.*;
interface UnknownBoundaryMarker {}
class AbsorptionState {static String trace="";static final AssertionError assertion=new AssertionError("identity");static final IOException io=new IOException("identity");static final IllegalAccessException access=new IllegalAccessException("identity");static final InstantiationException instantiation=new InstantiationException("identity");static final InvocationTargetException invocation=new InvocationTargetException(io);static final RuntimeException runtime=new IllegalArgumentException("identity");static int prefix(int mode)throws AssertionError{trace+="P";if(mode==3)throw assertion;return mode;}static void effect(int mode)throws IOException,IllegalAccessException,InstantiationException,InvocationTargetException{trace+="E";if(mode==1)throw io;if(mode==2)throw access;if(mode==3)throw instantiation;if(mode==4)throw invocation;if(mode==5)throw assertion;if(mode==6)throw runtime;}}
class AbsorptionParent {AbsorptionParent(int value){AbsorptionState.trace+="S";}}
public class CheckedAbsorptionReview extends AbsorptionParent implements UnknownBoundaryMarker {
 CheckedAbsorptionReview(int mode){super(AbsorptionState.prefix(mode));AbsorptionState.trace+="C";}
 static void declared(int mode)throws Exception {AbsorptionState.effect(mode);}
 static void wrapped(int mode){try{AbsorptionState.effect(mode);}catch(Exception failure){AbsorptionState.trace+="H";throw new IllegalArgumentException("wrapped",failure);}finally{AbsorptionState.trace+="F";}}
 public static void main(String[]args){for(int mode=0;mode<7;mode++){AbsorptionState.trace="";try{declared(mode);System.out.println("ok:"+AbsorptionState.trace);}catch(Throwable failure){System.out.println((failure==AbsorptionState.io)+":"+(failure==AbsorptionState.access)+":"+(failure==AbsorptionState.instantiation)+":"+(failure==AbsorptionState.invocation)+":"+(failure==AbsorptionState.assertion)+":"+(failure==AbsorptionState.runtime)+":"+AbsorptionState.trace);}AbsorptionState.trace="";try{wrapped(mode);System.out.println("ok:"+AbsorptionState.trace);}catch(Throwable failure){System.out.println((failure.getCause()==AbsorptionState.io)+":"+(failure.getCause()==AbsorptionState.access)+":"+(failure.getCause()==AbsorptionState.instantiation)+":"+(failure.getCause()==AbsorptionState.invocation)+":"+(failure==AbsorptionState.assertion)+":"+(failure.getCause()==AbsorptionState.runtime)+":"+AbsorptionState.trace);}}for(int mode:new int[]{0,3}){AbsorptionState.trace="";try{new CheckedAbsorptionReview(mode);System.out.println(AbsorptionState.trace);}catch(Throwable failure){System.out.println((failure==AbsorptionState.assertion)+":"+AbsorptionState.trace);}}}
}`, func(name string) bool { return name != "UnknownBoundaryMarker" }, Precision, Compatibility, "legacy")
}
