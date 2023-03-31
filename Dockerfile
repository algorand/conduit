FROM golang:1.17 as build

# Download (and cache) dependencies
COPY go.mod /conduit/
COPY go.sum /conduit/
RUN cd /conduit && \
    go mod download

# Add source and build
ADD . /conduit
RUN cd /conduit && \
    make conduit

# Final stage
FROM debian:bullseye-slim

COPY --from=build /conduit/cmd/conduit/conduit /conduit/conduit

RUN useradd conduit
RUN mkdir -p /conduit/data && \
    chown -R conduit.conduit /conduit

USER conduit
WORKDIR /conduit
ENTRYPOINT ["./conduit"]
CMD ["-d", "data"]
