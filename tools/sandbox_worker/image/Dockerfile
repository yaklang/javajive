# Trusted toolchain image for T30 isolation worker.
# Built on the host/CI with network; untrusted runs use --pull=never --network=none.
FROM alpine:3.20@sha256:d9e853e87e55526f6b2917df91a2115c36dd7c696a35be12163d44e6e2a4b6bc
RUN apk add --no-cache openjdk21-jdk python3
ENV JAVA_HOME=/usr/lib/jvm/java-21-openjdk
ENV PATH="/usr/lib/jvm/java-21-openjdk/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
WORKDIR /work
