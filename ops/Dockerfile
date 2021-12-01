# create build environment
FROM golang:1.17-buster as build-env
RUN apt-get update && apt-get install -y ca-certificates

# copy go files and build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/relay/ ./
RUN go build -o bin/relay

# create container for binary
FROM scratch

# copy required cgo binaries
COPY --from=build-env /lib /lib
COPY --from=build-env /lib/x86_64-linux-gnu /lib64
# COPY --from=build-env /lib/x86_64-linux-gnu/libgcc_s.so.1 /lib64/libgcc_s.so.1
# COPY --from=build-env /lib/x86_64-linux-gnu/librt.so.1 /lib64/librt.so.1
# COPY --from=build-env /lib/x86_64-linux-gnu/libdl.so.2 /lib64/libdl.so.2
# COPY --from=build-env /lib/x86_64-linux-gnu/libm.so.6 /lib64/libm.so.6

# copy certs and compiled relay
COPY --from=build-env /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build-env /app/bin/relay /bin/relay

ENTRYPOINT ["bin/relay"]
