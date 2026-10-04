package javaclassparser

import "testing"

// A null return saved across resource cleanup is a different local web from
// the synthetic catch exception. Final result types make accidental Throwable
// declarations fail compilation instead of concealing the alias as a cast.
func TestAdversarialResourceNullReturnIdentityRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ResourceNullIdentity", `
final class FinalToken {
  final String text;
  FinalToken(String text) {this.text=text;}
  public String toString() {return text;}
}


class IdentityResource implements AutoCloseable {
  static String trace;
  static boolean closeFailure,readFailure;
  IdentityResource() {trace+="O";}
  FinalToken read() {trace+="R";if(readFailure)throw new IllegalStateException("read");return new FinalToken("value");}
  public void close() {trace+="C";if(closeFailure)throw new IllegalArgumentException("close");}
}
public class ResourceNullIdentity {
  static FinalToken read(boolean empty) {
    try(IdentityResource resource=new IdentityResource()) {
      if(empty)return null;
      return resource.read();
    }
  }
  public static void main(String[] args) { ResourceNullIdentityOracle.main(args); }
}
class ResourceNullIdentityOracle {
  public static void main(String[] args) {
    for(int empty=0;empty<2;empty++)for(int body=0;body<2;body++)for(int close=0;close<2;close++) {
      IdentityResource.trace="";IdentityResource.readFailure=body!=0;IdentityResource.closeFailure=close!=0;
      try {System.out.print(ResourceNullIdentity.read(empty!=0)+":");}
      catch(Throwable failure) {
        System.out.print(failure.getClass().getSimpleName()+":"+failure.getMessage()+":");
        for(Throwable suppressed:failure.getSuppressed())System.out.print(suppressed.getMessage()+",");
      }
      System.out.print(IdentityResource.trace+";");
    }
  }
}`, Precision, Compatibility, "legacy")
}

// An effectful decision is protected, whereas the successful-path close is
// not. Factoring the shared tail must neither move nor duplicate that decision.
func TestAdversarialProtectedDecisionCleanupRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ProtectedDecisionCleanup", `
final class DecisionValue {
  public String toString() {return "value";}
}
class DecisionResource implements AutoCloseable {
  static String trace;
  static boolean decideFailure,readFailure,closeFailure;
  DecisionResource() {trace+="O";}
  static boolean decide(boolean empty) {
    trace+="D";
    if(decideFailure)throw new IllegalStateException("decision");
    return empty;
  }
  DecisionValue read() {
    trace+="R";
    if(readFailure)throw new IllegalStateException("read");
    return new DecisionValue();
  }
  public void close() {
    trace+="C";
    if(closeFailure)throw new IllegalArgumentException("close");
  }
}
public class ProtectedDecisionCleanup {
  static DecisionValue run(boolean empty) {
    try(DecisionResource resource=new DecisionResource()) {
      if(DecisionResource.decide(empty))return null;
      return resource.read();
    }
  }
  public static void main(String[] args) { ProtectedDecisionCleanupOracle.main(args); }
}
class ProtectedDecisionCleanupOracle {
  public static void main(String[] args) {
    for(int mask=0;mask<16;mask++) {
      DecisionResource.trace="";
      DecisionResource.decideFailure=(mask&1)!=0;
      DecisionResource.readFailure=(mask&2)!=0;
      DecisionResource.closeFailure=(mask&4)!=0;
      try {System.out.print(ProtectedDecisionCleanup.run((mask&8)!=0)+":");}
      catch(Throwable failure) {
        System.out.print(failure.getClass().getSimpleName()+":"+failure.getMessage()+":");
        for(Throwable suppressed:failure.getSuppressed())System.out.print(suppressed.getMessage()+",");
      }
      System.out.print(DecisionResource.trace+";");
    }
  }
}`, Precision, Compatibility, "legacy")
}
