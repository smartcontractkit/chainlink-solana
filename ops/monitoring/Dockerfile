# Build image

FROM ubuntu:20.04 AS build

# OS dependencies
RUN apt-get update && apt-get install -y wget gcc

# Copy source

RUN mkdir -p /solana-monitoring/cmd
COPY ./cmd/monitoring /solana-monitoring/cmd/monitoring
COPY ./pkg /solana-monitoring/pkg
COPY ./go.mod /solana-monitoring/
COPY ./go.sum /solana-monitoring/

# Install golang

RUN wget -c https://dl.google.com/go/go1.17.4.linux-amd64.tar.gz -O - \
  | tar -xz -C /usr/local \
  && mkdir -p /go/src /go/bin
ENV PATH /usr/local/go/bin:$PATH

# Compile binary

WORKDIR /solana-monitoring
RUN go build -o ./monitoring ./cmd/monitoring/*.go

# Production image

FROM ubuntu:20.04

RUN apt-get update && apt-get install -y ca-certificates
COPY --from=build /solana-monitoring/monitoring /monitoring

# Expose prometheus default port
EXPOSE 9090/tcp

ENTRYPOINT ["/monitoring"]
CMD ["--help"]
