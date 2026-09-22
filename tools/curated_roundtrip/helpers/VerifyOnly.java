public final class VerifyOnly {
 public static void main(String[] args) throws Exception {
  if(args.length==0)throw new IllegalArgumentException("class names required");
  for(String name:args){Class<?> c=Class.forName(name,false,VerifyOnly.class.getClassLoader());c.getDeclaredMethods();c.getDeclaredConstructors();c.getDeclaredFields();}
  System.out.println("VERIFIED_NOT_EXECUTED");
 }
}
