FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /build
RUN git clone https://github.com/openbao/openbao-plugin-secrets-oauthapp.git .
RUN CGO_ENABLED=0 go build -o vault-plugin-secrets-oauthapp ./cmd/vault-plugin-secrets-oauthapp

FROM openbao/openbao:2
COPY --from=builder /build/vault-plugin-secrets-oauthapp /openbao/plugins/
