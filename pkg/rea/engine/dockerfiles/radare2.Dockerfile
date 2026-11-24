# radare2 (LGPL-3.0): same commands and JSON as rizin, used when rizin is not built. Built only by
# `dendrite retro engines build radare2`.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends radare2 \
 && rm -rf /var/lib/apt/lists/*
USER 65534:65534
