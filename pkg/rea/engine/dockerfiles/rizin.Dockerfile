# rizin (LGPL-3.0), the engine Dendrite prefers. Built only by `dendrite retro engines build rizin`; never pulled
# implicitly. The analysis runs it with no network, no capabilities and a read-only root (see pkg/sandbox).
FROM debian:trixie-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends rizin \
 && rm -rf /var/lib/apt/lists/*
USER 65534:65534
