FROM alpine:3.20

RUN apk add --no-cache ca-certificates git tini \
    && addgroup -S agora \
    && adduser -S -G agora -u 10001 -h /home/agora -s /sbin/nologin agora \
    && mkdir -p /home/agora/.agora \
    && chown -R agora:agora /home/agora

COPY --chown=root:root agora /usr/local/bin/agora
RUN chmod 0755 /usr/local/bin/agora

LABEL org.opencontainers.image.title="agora-cli" \
      org.opencontainers.image.description="Native Agora CLI for authentication, project management, quickstart setup, and developer onboarding." \
      org.opencontainers.image.url="https://github.com/AgoraIO/cli" \
      org.opencontainers.image.documentation="https://agoraio.github.io/cli/" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.vendor="Agora.io"

USER agora
WORKDIR /home/agora
ENV HOME=/home/agora \
    AGORA_HOME=/home/agora/.agora

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/agora"]
CMD ["--help"]
