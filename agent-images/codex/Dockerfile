FROM node:20-bookworm-slim

ARG CODEX_VERSION=latest

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    gh \
    jq \
    ca-certificates \
    openssh-client \
    curl \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g @openai/codex@${CODEX_VERSION}

RUN mkdir -p /workspace /home/node/.codex /tmp/termination-log && \
    chown -R node:node /workspace /home/node/.codex /tmp

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

USER node
WORKDIR /workspace

RUN git config --global init.defaultBranch main

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
