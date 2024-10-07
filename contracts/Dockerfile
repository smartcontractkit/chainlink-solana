ARG ANCHOR_CLI

FROM backpackapp/build:${ANCHOR_CLI}

COPY . /contracts

RUN cd /contracts && anchor build

# only keep downloaded artifacts in /root/.cargo cached
RUN rm -rf /contracts

# remove yarn dependency, use pnpm
RUN npm uninstall -g yarn
RUN npm install -g pnpm
