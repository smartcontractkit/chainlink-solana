ARG ANCHOR_CLI

FROM backpackapp/build:${ANCHOR_CLI}

RUN git clone https://github.com/smartcontractkit/chainlink-solana.git /chainlink-solana

RUN cd /chainlink-solana/contracts && anchor build

# only keep downloaded artifacts in /root/.cargo cached
RUN rm -rf /chainlink-solana
