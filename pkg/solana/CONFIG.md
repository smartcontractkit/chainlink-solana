[//]: # (Documentation generated from docs.toml - DO NOT EDIT.)
This document describes the TOML format for configuration.
## Example

```toml
[[Solana]]
ChainID = "mainnet"

[[Solana.Nodes]]
Name = 'primary'
URL = 'http://solana.web'

```

## Global
```toml
ChainID = 'mainnet' # Example
Enabled = true # Default
BlockTime = '500ms' # Default
BalancePollPeriod = '5s' # Default
ConfirmPollPeriod = '500ms' # Default
OCR2CachePollPeriod = '1s' # Default
OCR2CacheTTL = '1m' # Default
TxTimeout = '1m' # Default
TxRetryTimeout = '10s' # Default
TxConfirmTimeout = '30s' # Default
TxExpirationRebroadcast = false # Default
TxRetentionTimeout = '0s' # Default
SkipPreflight = true # Default
Commitment = 'confirmed' # Default
MaxRetries = 0 # Default
FeeEstimatorMode = 'fixed' # Default
ComputeUnitPriceMax = 1000 # Default
ComputeUnitPriceMin = 0 # Default
ComputeUnitPriceDefault = 0 # Default
FeeBumpPeriod = '3s' # Default
BlockHistoryPollPeriod = '5s' # Default
BlockHistorySize = 1 # Default
BlockHistoryBatchLoadSize = 20 # Default
ComputeUnitLimitDefault = 200_000 # Default
EstimateComputeUnitLimit = false # Default
LogPollerStartingLookback = '24h' # Default
LogPollerCPIEventsEnabled = true # Default
```


### ChainID
```toml
ChainID = 'mainnet' # Example
```
ChainID is the Solana chain ID. Must be one of: mainnet, testnet, devnet, localnet. Mandatory.

### Enabled
```toml
Enabled = true # Default
```
Enabled enables this chain.

### BlockTime
```toml
BlockTime = '500ms' # Default
```
BlockTime specifies the average time between blocks on this chain

### BalancePollPeriod
```toml
BalancePollPeriod = '5s' # Default
```
BalancePollPeriod is the rate to poll for SOL balance and update Prometheus metrics.

### ConfirmPollPeriod
```toml
ConfirmPollPeriod = '500ms' # Default
```
ConfirmPollPeriod is the rate to poll for signature confirmation.

### OCR2CachePollPeriod
```toml
OCR2CachePollPeriod = '1s' # Default
```
OCR2CachePollPeriod is the rate to poll for the OCR2 state cache.

### OCR2CacheTTL
```toml
OCR2CacheTTL = '1m' # Default
```
OCR2CacheTTL is the stale OCR2 cache deadline.

### TxTimeout
```toml
TxTimeout = '1m' # Default
```
TxTimeout is the timeout for sending txes to an RPC endpoint.

### TxRetryTimeout
```toml
TxRetryTimeout = '10s' # Default
```
TxRetryTimeout is the duration for tx manager to attempt rebroadcasting to RPC, before giving up.

### TxConfirmTimeout
```toml
TxConfirmTimeout = '30s' # Default
```
TxConfirmTimeout is the duration to wait when confirming a tx signature, before discarding as unconfirmed.

### TxExpirationRebroadcast
```toml
TxExpirationRebroadcast = false # Default
```
TxExpirationRebroadcast enables or disables transaction rebroadcast if expired. Expiration check is performed every `ConfirmPollPeriod`
A transaction is considered expired if the blockhash it was sent with is 150 blocks older than the latest blockhash.

### TxRetentionTimeout
```toml
TxRetentionTimeout = '0s' # Default
```
TxRetentionTimeout is the duration to retain transactions in storage after being marked as finalized or errored. Set to 0 to immediately drop transactions.

### SkipPreflight
```toml
SkipPreflight = true # Default
```
SkipPreflight enables or disables preflight checks when sending txs.

### Commitment
```toml
Commitment = 'confirmed' # Default
```
Commitment is the confirmation level for solana state and transactions. ([documentation](https://docs.solana.com/developing/clients/jsonrpc-api#configuring-state-commitment))

### MaxRetries
```toml
MaxRetries = 0 # Default
```
MaxRetries is the maximum number of times the RPC node will automatically rebroadcast a tx.
The default is 0 for custom txm rebroadcasting method, set to -1 to use the RPC node's default retry strategy.

### FeeEstimatorMode
```toml
FeeEstimatorMode = 'fixed' # Default
```
FeeEstimatorMode is the method used to determine the base fee

### ComputeUnitPriceMax
```toml
ComputeUnitPriceMax = 1000 # Default
```
ComputeUnitPriceMax is the maximum price per compute unit that a transaction can be bumped to

### ComputeUnitPriceMin
```toml
ComputeUnitPriceMin = 0 # Default
```
ComputeUnitPriceMin is the minimum price per compute unit that transaction can have

### ComputeUnitPriceDefault
```toml
ComputeUnitPriceDefault = 0 # Default
```
ComputeUnitPriceDefault is the default price per compute unit price, and the starting base fee when FeeEstimatorMode = 'fixed'

### FeeBumpPeriod
```toml
FeeBumpPeriod = '3s' # Default
```
FeeBumpPeriod is the amount of time before a tx is retried with a fee bump. WARNING: If FeeBumpPeriod is shorter than blockhash expiration, multiple valid transactions can exist in parallel. This can result in higher costs and can cause unexpected behaviors if contracts do not de-dupe txs

### BlockHistoryPollPeriod
```toml
BlockHistoryPollPeriod = '5s' # Default
```
BlockHistoryPollPeriod is the rate to poll for blocks in the block history fee estimator

### BlockHistorySize
```toml
BlockHistorySize = 1 # Default
```
BlockHistorySize is the number of blocks to take into consideration when using FeeEstimatorMode = 'blockhistory' to determine compute unit price.
If set to 1, the compute unit price will be determined by the median of the last block's compute unit prices.
If set N > 1, the compute unit price will be determined by the average of the medians of the last N blocks' compute unit prices.
DISCLAIMER: If set to a value greater than BlockHistoryBatchLoadSize, initial estimations during startup would be over smaller block ranges until the cache is filled.

### BlockHistoryBatchLoadSize
```toml
BlockHistoryBatchLoadSize = 20 # Default
```
BlockHistoryBatchLoadSize is the number of latest blocks to fetch from the chain to store in the cache every BlockHistoryPollPeriod.
This config is only relevant if BlockHistorySize > 1 and if BlockHistorySize is greater than BlockHistoryBatchLoadSize.
Ensure the value is greater than the number of blocks that would be produced between each BlockHistoryPollPeriod to avoid gaps in block history.

### ComputeUnitLimitDefault
```toml
ComputeUnitLimitDefault = 200_000 # Default
```
ComputeUnitLimitDefault is the compute units limit applied to transactions unless overriden during the txm enqueue

### EstimateComputeUnitLimit
```toml
EstimateComputeUnitLimit = false # Default
```
EstimateComputeUnitLimit enables or disables compute unit limit estimations per transaction. If estimations return 0 used compute, the ComputeUnitLimitDefault value is used, if set.

### LogPollerStartingLookback
```toml
LogPollerStartingLookback = '24h' # Default
```
LogPollerStartingLookback

### LogPollerCPIEventsEnabled
```toml
LogPollerCPIEventsEnabled = true # Default
```
LogPollerCPIEventsEnabled - Flag for LogPoller listening to CPI Events.
CPI events require LOOPP mode to function correctly.

## MultiNode
```toml
[MultiNode]
Enabled = false # Default
PollFailureThreshold = 5 # Default
PollInterval = '15s' # Default
SelectionMode = 'PriorityLevel' # Default
SyncThreshold = 10 # Default
NodeIsSyncingEnabled = false # Default
LeaseDuration = '1m' # Default
NewHeadsPollInterval = '5s' # Default
FinalizedBlockPollInterval = '5s' # Default
EnforceRepeatableRead = true # Default
DeathDeclarationDelay = '20s' # Default
VerifyChainID = true # Default
NodeNoNewHeadsThreshold = '20s' # Default
NoNewFinalizedHeadsThreshold = '20s' # Default
FinalityDepth = 0 # Default
FinalityTagEnabled = true # Default
FinalizedBlockOffset = 50 # Default
```


### Enabled
```toml
Enabled = false # Default
```
Enabled enables the multinode feature.

### PollFailureThreshold
```toml
PollFailureThreshold = 5 # Default
```
PollFailureThreshold is the number of consecutive poll failures before a node is considered unhealthy.

### PollInterval
```toml
PollInterval = '15s' # Default
```
PollInterval is the rate to poll for node health.

### SelectionMode
```toml
SelectionMode = 'PriorityLevel' # Default
```
SelectionMode is the method used to select the next best node to use.

### SyncThreshold
```toml
SyncThreshold = 10 # Default
```
SyncThreshold is the number of blocks behind the best node that a node can be before it is considered out of sync.

### NodeIsSyncingEnabled
```toml
NodeIsSyncingEnabled = false # Default
```
NodeIsSyncingEnabled enables the feature to avoid sending transactions to nodes that are syncing. Not relavant for Solana.

### LeaseDuration
```toml
LeaseDuration = '1m' # Default
```
LeaseDuration is the max duration a node can be leased for.

### NewHeadsPollInterval
```toml
NewHeadsPollInterval = '5s' # Default
```
NewHeadsPollInterval is the rate to poll for new heads.

### FinalizedBlockPollInterval
```toml
FinalizedBlockPollInterval = '5s' # Default
```
FinalizedBlockPollInterval is the rate to poll for the finalized block.

### EnforceRepeatableRead
```toml
EnforceRepeatableRead = true # Default
```
EnforceRepeatableRead enforces the repeatable read guarantee for multinode.

### DeathDeclarationDelay
```toml
DeathDeclarationDelay = '20s' # Default
```
DeathDeclarationDelay is the duration to wait before declaring a node dead.

### VerifyChainID
```toml
VerifyChainID = true # Default
```
VerifyChainID enforces RPC Client ChainIDs to match configured ChainID

### NodeNoNewHeadsThreshold
```toml
NodeNoNewHeadsThreshold = '20s' # Default
```
NodeNoNewHeadsThreshold is the duration to wait before declaring a node unhealthy due to no new heads.

### NoNewFinalizedHeadsThreshold
```toml
NoNewFinalizedHeadsThreshold = '20s' # Default
```
NoNewFinalizedHeadsThreshold is the duration to wait before declaring a node unhealthy due to no new finalized heads.

### FinalityDepth
```toml
FinalityDepth = 0 # Default
```
FinalityDepth is not used when finality tags are enabled.

### FinalityTagEnabled
```toml
FinalityTagEnabled = true # Default
```
FinalityTagEnabled enables the use of finality tags.

### FinalizedBlockOffset
```toml
FinalizedBlockOffset = 50 # Default
```
FinalizedBlockOffset is the offset from the finalized block to use for finality tags.

## Workflow
```toml
[Workflow]
AcceptanceTimeout = '45s' # Default
ForwarderAddress = '14grJpemFaf88c8tiVb77W7TYg2W3ir6pfkKz3YjhhZ5' # Example
ForwarderState = '14grJpemFaf88c8tiVb77W7TYg2W3ir6pfkKz3YjhhZ5' # Example
FromAddress = '4BJXYkfvg37zEmBbsacZjeQDpTNx91KppxFJxRqrz48e' # Example
GasLimitDefault = 300_000 # Default
Local = false # Default
PollPeriod = '3s' # Default
TxAcceptanceState = 3 # Default
```


### AcceptanceTimeout
```toml
AcceptanceTimeout = '45s' # Default
```
AcceptanceTimeout is the default timeout for a tranmission to be accepted on chain

### ForwarderAddress
```toml
ForwarderAddress = '14grJpemFaf88c8tiVb77W7TYg2W3ir6pfkKz3YjhhZ5' # Example
```
ForwarderAddress is the keystone forwarder program address on chain.

### ForwarderState
```toml
ForwarderState = '14grJpemFaf88c8tiVb77W7TYg2W3ir6pfkKz3YjhhZ5' # Example
```
ForwarderState is the keystone forwarder program state account on chain.

### FromAddress
```toml
FromAddress = '4BJXYkfvg37zEmBbsacZjeQDpTNx91KppxFJxRqrz48e' # Example
```
FromAddress is Address of the transmitter key to use for workflow writes.

### GasLimitDefault
```toml
GasLimitDefault = 300_000 # Default
```
GasLimitDefault is the default gas limit for workflow transactions.

### Local
```toml
Local = false # Default
```
Local defines if relayer runs against local devnet

### PollPeriod
```toml
PollPeriod = '3s' # Default
```
PollPeriod is the default poll period for checking transmission state

### TxAcceptanceState
```toml
TxAcceptanceState = 3 # Default
```
TxAcceptanceState is the default acceptance state for writer DON tranmissions.

## Nodes
```toml
[[Nodes]]
Name = 'primary' # Example
URL = 'http://solana.web' # Example
SendOnly = false # Default
Order = 100 # Default
IsLoadBalancedRPC = false # Default
```


### Name
```toml
Name = 'primary' # Example
```
Name is a unique (per-chain) identifier for this node.

### URL
```toml
URL = 'http://solana.web' # Example
```
URL is the HTTP(S) endpoint for this node.

### SendOnly
```toml
SendOnly = false # Default
```
SendOnly is a multinode config that only sends transactions to a node and does not read state

### Order
```toml
Order = 100 # Default
```
Order specifies the priority for each node. 1 is highest priority down to 100 being the lowest.

### IsLoadBalancedRPC
```toml
IsLoadBalancedRPC = false # Default
```
IsLoadBalancedRPC indicates whether the http/ws url above has multiple rpc's behind it.
If true, we should try reconnecting to the node even when its the only node in the Nodes list.
If false and its the only node in the nodes list, we will mark it alive even when its out of sync, because it might still be able to send txs.

