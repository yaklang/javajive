public record RecordCustom(int x, String s) {
  public RecordCustom { if(s==null) s="nil"; }
  @Override public String toString(){return "CUSTOM:"+x+":"+s;}
  public static void main(String[] a){RecordCustom r=new RecordCustom(3,null);System.out.println(r);System.out.println(r.equals(new RecordCustom(3,"nil")));System.out.println(r.hashCode()==new RecordCustom(3,"nil").hashCode());System.out.println(RecordCustom.class.isRecord());System.out.println(RecordCustom.class.getRecordComponents()[1].getName());}
}
