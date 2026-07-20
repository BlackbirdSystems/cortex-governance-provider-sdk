FROM alpine:3

RUN apk --no-cache add ca-certificates tzdata

ENV SOCKETS_DIR=/var/run/cortex-governance/providers

COPY entrypoint.sh /usr/local/bin/cortex-provider-entrypoint.sh
RUN chmod +x /usr/local/bin/cortex-provider-entrypoint.sh

ENTRYPOINT ["/usr/local/bin/cortex-provider-entrypoint.sh"]
