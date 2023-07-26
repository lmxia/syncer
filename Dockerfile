FROM alpine:3.7 as syncer
WORKDIR /
COPY ./cmd/bin/syncer .
ENTRYPOINT ["./syncer"]