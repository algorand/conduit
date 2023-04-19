# This dockerfile is used by goreleaser
FROM debian:bullseye-slim

RUN useradd conduit
RUN mkdir -p /conduit/data && \
    chown -R conduit.conduit /conduit

# binary is passed into the build
COPY conduit /conduit/conduit

USER conduit
WORKDIR /conduit
ENTRYPOINT ["./conduit"]
CMD ["-d", "data"]
