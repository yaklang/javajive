package javaclassparser

import "testing"

func TestAdversarialEnumLambdaArgumentsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "EnumLambdaArguments", `interface EnumIntStep {int apply(int value);}
class EnumLambdaTrace {
  static final StringBuilder events=new StringBuilder();
  static int twice(int value) {events.append("twice;");return value*2;}
}
public enum EnumLambdaArguments {
  INCREMENT(value -> {EnumLambdaTrace.events.append("inc;");return value+1;}),
  DOUBLE(EnumLambdaTrace::twice),
  CHECK(value -> {EnumLambdaTrace.events.append("check;");if(value<0)throw new IllegalArgumentException("negative");return value;}),
  PAIR(value -> {EnumLambdaTrace.events.append("left;");return value-1;},
       value -> {EnumLambdaTrace.events.append("right;");return value+3;});
  final EnumIntStep first;
  final EnumIntStep second;
  EnumLambdaArguments(EnumIntStep first) {this(first,null);}
  EnumLambdaArguments(EnumIntStep first,EnumIntStep second) {
    EnumLambdaTrace.events.append("ctor:").append(name()).append(";");
    this.first=first;this.second=second;
  }
  int apply(int value) {int result=first.apply(value);return second==null?result:second.apply(result);}
  public static void main(String[] args) {
    for(EnumLambdaArguments step:values())for(int value:new int[]{-1,0,1,7,Integer.MAX_VALUE}) {
      try {System.out.print(step.name()+":"+step.apply(value)+"|");}
      catch(RuntimeException ex){System.out.print(ex.getClass().getSimpleName()+":"+ex.getMessage()+"|");}
    }
    System.out.print(EnumLambdaTrace.events);
  }
}`, Precision, Compatibility, "legacy")
}
