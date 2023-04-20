# This dockerfile is used by goreleaser
FROM debian:bullseye-slim

RUN groupadd --gid=999 --system algorand && \
    useradd --uid=999 --no-log-init --create-home --system --gid algorand algorand && \
    mkdir -p /data && \
    chown -R algorand.algorand /data && \
    apt-get update && \
    apt-get install -y gosu ca-certificates && \
    update-ca-certificates && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# binary is passed into the build
COPY conduit /usr/local/bin/conduit
COPY docker/docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

ENV CONDUIT_DATA_DIR /data
WORKDIR /data
ENTRYPOINT ["docker-entrypoint.sh"]
