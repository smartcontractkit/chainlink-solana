### Persistent K8s env chaos test run
In order to deploy contracts and env only once and then run chaos suite:
1. Paste version of image you want to run in `chainlink-relay-sol.yaml`
2. Spin up an environment
```shell
envcli new --preset chainlink-relay-sol.yaml
```
3. Run initial test
```shell
SELECTED_NETWORKS="solana" NETWORK_SETTINGS="${YOUR_NETWORKS_FILE}" ENVIRONMENT_FILE="/Users/f4hrenh9it/GolandProjects/chainlink-solana/${YOUR_ENV_YAML}" ginkgo tests/e2e/chaos
```
4. Set `contracts_deployed: true` in `networks.yaml`
5. Run tests again