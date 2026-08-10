ARG ANCHOR_CLI

FROM backpackapp/build:${ANCHOR_CLI}

COPY . /contracts

RUN cd /contracts && anchor build

# only keep downloaded artifacts in /root/.cargo cached
RUN rm -rf /contracts

# remove yarn dependency, use pnpm
# Pin pnpm: latest (>=10.18) requires Node 22.13+, but the base image (backpackapp/build)
# ships Node 20.16. Match the pinned version used elsewhere in the workflow.
RUN npm uninstall -g yarn
RUN npm install -g pnpm@10.4.1
