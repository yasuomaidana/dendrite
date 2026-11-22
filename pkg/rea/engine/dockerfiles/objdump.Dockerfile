# objdump from GNU binutils (GPL-3.0-or-later): the lightweight engine, disassembly and symbols only. Built only by
# `dendrite retro engines build objdump`.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends binutils \
 && rm -rf /var/lib/apt/lists/*
USER 65534:65534
