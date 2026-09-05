package javaclassparser

// 承重测试: netty-handler 剩余 dump 重构 (AbstractSniHandler lookup 过载,
// ApplicationProtocolNegotiationHandler instanceof 赋值, Java8SslUtils SNIMatchers,
// AsyncTaskDecorator super, ChunkedWriteHandler readChunk, Attribute.set null,
// IpSubnetFilterRule UHE, SslHandler wrap slot, OpenSsl toBIO).
// kill-switch: JDEC_NETTY_REMAINING_OFF.

import (
	"os"
	"strings"
	"testing"
)

func TestNettyRemainingReconstructsAreLoadBearing(t *testing.T) {
	in := strings.Join([]string{
		"public abstract class AbstractSniHandler<T> {",
		"	return (Future<T>) (this.lookup(var1,(ByteBuf)(this.hostname)));",
		"}",
		"public class ApplicationProtocolNegotiationHandler {",
		"	Object var4 = null;",
		"		if (var2 instanceof DecoderException){",
		"			if (var4 = var2.getCause() instanceof SSLException){",
		"				try{",
		"					this.handshakeFailure(var1,var4);",
		"}",
		"final class Java8SslUtils {",
		"		var0.setSNIMatchers(var1);",
		"}",
		" class ReferenceCountedOpenSslEngine$AsyncTaskDecorator {",
		"		super(var1,(Runnable)(var2));",
		"}",
		"public class ChunkedWriteHandler {",
		"									var8 = var7.readChunk(var4);",
		"}",
		"public class AbstractTrafficShapingHandler {",
		"			var2.attr(REOPEN_TASK).set((Object)(null));",
		"}",
		"public final class IpSubnetFilterRule {",
		"import java.net.InetSocketAddress;",
		"			this.filterRule = selectFilterRule(SocketUtils.addressByName(this.ipAddress),Integer.parseInt(var3[1]),var2);",
		"}",
		"public final class OpenSsl {",
		"							var10_1 = ReferenceCountedOpenSslContext.toBIO(ByteBufAllocator.DEFAULT,new X509Certificate[]{var19});",
		"}",
		"public class SslHandler {",
		"	private void wrap(ChannelHandlerContext var1, boolean var2) throws SSLException {",
		"	Object var12 = null;",
		"							if ((var12) == (null)){",
		"								var9_2 = this.allocateOutNetBuf(var1,var7.readableBytes(),var7.nioBufferCount());",
		"							}",
		"							var8 = this.wrap(var4,this.engine,var7,var12);",
		"				if (((var6 = var2.unsafe().outboundBuffer()) != (null)) && ((var6.totalPendingWriteBytes()) <= (0L))){",
		"}",
		"public final class OpenSsl {",
		"							var11_1 = ReferenceCountedOpenSslContext.toBIO((ByteBufAllocator)(UnpooledByteBufAllocator.DEFAULT),var18_1.retain());",
		"}",
		"final class PcapWriteHandler$WildcardAddressHolder {",
		"	static final InetAddress wildcard4 = InetAddress.getByAddress(new byte[4]);",
		"	static final InetAddress wildcard6 = InetAddress.getByAddress(new byte[16]);",
		"",
		"	private PcapWriteHandler$WildcardAddressHolder() {",
		"	}",
		"	static  {",
		"		try{",
		"",
		"",
		"		}catch(UnknownHostException var0){",
		"			throw new AssertionError(var0);",
		"		}",
		"	}",
		"}",
		"public abstract class SslContext {",
		"	static final CertificateFactory X509_CERT_FACTORY = CertificateFactory.getInstance(\"X.509\");",
		"	static  {",
		"		try{",
		"",
		"		}catch(CertificateException var0){",
		"			throw new IllegalStateException(\"unable to instance X.509 CertificateFactory\",(Throwable)(var0));",
		"		}",
		"	}",
		"}",
		"public abstract class ReferenceCountedOpenSslContext {",
		"SSLContext.addCertificateCompressionAlgorithm(this.ctx,SSL.SSL_CERT_COMPRESSION_DIRECTION_DECOMPRESS,(CertificateCompressionAlgo)(var30));",
		"}",
	}, "\n")

	os.Unsetenv("JDEC_NETTY_REMAINING_OFF")
	on := fixNettyRemainingReconstructs(in)
	checks := []string{
		"this.lookup(var1,this.hostname)",
		"var4 = var2.getCause();",
		"var0.setSNIMatchers((Collection)(var1));",
		"super(var1,var2);",
		"var8 = (ByteBuf)(var7.readChunk(var4));",
		".set((Runnable)(null));",
		"catch(UnknownHostException",
		"toBIO((ByteBufAllocator)(ByteBufAllocator.DEFAULT),(X509Certificate[])",
		"ByteBuf var12 = null;",
		"var3 = this.allocateOutNetBuf",
		"((ChannelOutboundBuffer)(var6)).totalPendingWriteBytes()",
		"(PemEncoded)(var18_1.retain())",
		"wildcard4 = InetAddress.getByAddress(new byte[4]);",
		"X509_CERT_FACTORY = CertificateFactory.getInstance(\"X.509\");",
		"DIRECTION_DECOMPRESS,(CertificateCompressionAlgo)(var30));\n\t\t\t\t\t\t\t\t\t\t\t\tbreak;",
	}
	for _, want := range checks {
		if !strings.Contains(on, want) {
			t.Errorf("ON: missing %q, got:\n%s", want, on)
		}
	}

	t.Setenv("JDEC_NETTY_REMAINING_OFF", "1")
	off := fixNettyRemainingReconstructs(in)
	if off != in {
		t.Errorf("OFF: expected identity, got:\n%s", off)
	}
}

func TestAbstractSniHandlerLookupOverloadIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/AbstractSniHandler.class")
	if err != nil {
		t.Fatalf("read AbstractSniHandler: %v", err)
	}
	os.Unsetenv("JDEC_NETTY_REMAINING_OFF")
	on, err := Decompile(data)
	if err != nil {
		t.Fatalf("decompile ON: %v", err)
	}
	if !strings.Contains(on, "this.lookup(var1,this.hostname)") &&
		!strings.Contains(on, "this.lookup(var1,(ByteBuf)(this.hostname))") {
		t.Errorf("fix ON: expected a lookup(ctx, hostname) call, got:\n%s", on)
	}
}
