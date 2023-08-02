# Running e2e tests

The e2e tests run inside of a k8s cluster. They will run against whatever cluster your current kubectl context is set to. This can be an external k8s cluster or a local one (using something like minikube or k3d). 

Note: If running against a local k8s cluster, make sure you have plenty of ram allocated for docker, 12 gb if running individual tests and a lot more if you run parallel test like the ones in `make test_smoke` since it can run multiple tests in parallel

Steps to run the e2e tests:

1. Build using the `make build` command if you haven't already built the contracts.
2. Make sure your kubectl context is pointing to the cluster you want to run tests against.
3. Run a test, you have several options
    - `make test_smoke` will run the ocr2 e2e tests
    - `make test_chaos` will run the chaos tests

## Env variables
```bash
CHAINLINK_ENV_USER=John;
CHAINLINK_IMAGE={AWS_OIDC}.dkr.ecr.{AWS_REGION}.amazonaws.com/chainlink;
CHAINLINK_VERSION=develop; # Can be SHA
SELECTED_NETWORKS=SIMULATED;
INTERNAL_DOCKER_REPO={AWS_OIDC}.dkr.ecr.{AWS_REGION}.amazonaws.com
TTL=72h; # optional
TEST_LOG_LEVEL=debug # optional

# Running on testnet
LINK_TOKEN=Dmw5mDvteezKfop9zd3RQbJmZfBATF3QuSqDU66axyts;
PROGRAM_ID_ACCESS_CONTROLLER=9xi644bRR8birboDGdTiwBq3C7VEeR7VuamRYYXCubUW;
PROGRAM_ID_OCR2=cjg3oHmg9uuPsP8D6g29NWvhySJkdYdAo9D25PRbKXJ;
PROGRAM_ID_STORE=HEvSKofvBgfaexv23kMabbYqxasxU3mQ4ibBMEmJWHny;
VAULT_ADDRESS=G27m7KxTh4KVLapxB9MXfEA8HLUfYuGYQ1ELEs2zQdiQ;
PRIVATE_KEY=[123, 123, ...];
RPC_URL=https://api.devnet.solana.com;
WS_URL=wss://api.devnet.solana.com/;
```

You can always look at the [Makefile](../Makefile) in this repo to see other commands or tests that have been added since this readme was last updated.