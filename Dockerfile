# This dockerfile is used by goreleaser
FROM debian:bullseye-slim

RUN groupadd --gid=999 --system conduit && \
    useradd --uid=999 --no-log-init --create-home --system --gid conduit conduit && \
    mkdir -p /data && \
    chown -R conduit.conduit /data

# binary is passed into the build
COPY conduit /usr/local/bin/
COPY docker/docker-entrypoint.sh /usr/local/bin/

ENV CONDUIT_DATA_DIR /data
WORKDIR /data
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["conduit"]
