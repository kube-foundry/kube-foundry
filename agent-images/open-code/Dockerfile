FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    gh \
    jq \
    ca-certificates \
    openssh-client \
    curl \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://opencode.ai/install | bash

RUN useradd -m -u 1000 -s /bin/bash opencode

RUN mkdir -p /workspace /home/opencode/.config/opencode /tmp/termination-log && \
    chown -R opencode:opencode /workspace /home/opencode /tmp

COPY opencode.json /home/opencode/.config/opencode/config.json
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh && \
    chown opencode:opencode /home/opencode/.config/opencode/config.json

USER opencode
WORKDIR /workspace

RUN git config --global init.defaultBranch main

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
