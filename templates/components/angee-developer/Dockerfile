FROM node:20

RUN apt-get update && apt-get install -y --no-install-recommends \
    git less procps sudo vim nano fzf jq curl \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# Match Anthropic's devcontainer user setup
ARG USERNAME=node
RUN mkdir -p /usr/local/share/npm-global && chown -R node:node /usr/local/share/npm-global
RUN mkdir -p /workspace /home/node/.claude && chown -R node:node /workspace /home/node/.claude

USER node
ENV NPM_CONFIG_PREFIX=/usr/local/share/npm-global
ENV PATH=$PATH:/usr/local/share/npm-global/bin
ENV SHELL=/bin/bash

# Install Claude Code
RUN npm install -g @anthropic-ai/claude-code@latest

WORKDIR /workspace

# Claude Code runs interactively as PID 1; angee develop attaches via docker attach
CMD ["claude"]
