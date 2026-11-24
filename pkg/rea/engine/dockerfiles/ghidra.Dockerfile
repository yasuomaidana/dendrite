# Ghidra (Apache-2.0) headless: the engine with a decompiler. Built only by
# `dendrite retro engines build ghidra --url <release zip> --sha256 <checksum>`: the release is large and Dendrite does
# not pin it for you, so you choose the version and the build refuses a download whose checksum does not match.
FROM debian:bookworm-slim
ARG GHIDRA_URL
ARG GHIDRA_SHA256
RUN test -n "$GHIDRA_URL" && test -n "$GHIDRA_SHA256" \
 && apt-get update \
 && apt-get install -y --no-install-recommends openjdk-17-jdk-headless unzip curl ca-certificates \
 && curl -fsSL "$GHIDRA_URL" -o /tmp/ghidra.zip \
 && echo "$GHIDRA_SHA256  /tmp/ghidra.zip" | sha256sum -c - \
 && unzip -q /tmp/ghidra.zip -d /opt \
 && mv /opt/ghidra_* /opt/ghidra \
 && rm /tmp/ghidra.zip \
 && apt-get purge -y curl unzip \
 && rm -rf /var/lib/apt/lists/*
COPY ExportFunctions.java /scripts/ExportFunctions.java
ENV PATH="/opt/ghidra/support:${PATH}"
USER 65534:65534
