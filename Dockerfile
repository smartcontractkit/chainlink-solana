# Build the plugin binary
FROM golang:1.17-buster
WORKDIR /chainlink-solana

# Cache go mod download
ADD go.mod go.sum ./
RUN go mod download

COPY . ./

# Build plugin
RUN go build -o ./plugin ./cmd/plugin/main.go

# Final layer: ubuntu with chainlink and solana binaries
FROM smartcontract/chainlink:go-plugin

# Install solana plugin
COPY --from=0 /chainlink-solana/plugin /plugins/solana
ENV PLUGIN_SOLANA /plugins/solana

CMD ["local", "node"]