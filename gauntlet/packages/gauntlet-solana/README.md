# Gauntlet Solana

### Priority Fees Flags
To use historical estimate for best priority fees to use add flag: `--priorityFeesHistorical="percentile,n_historical_blocks"`

For example if you want to use the 80th percentile priority fee within the last 5 blocks you would pass in `--priorityFeesHistorical="0.8,5"` and median you would use `--priorityFeesHistorical="0.5,5"`

To use a constant priority fee so you set it yourself add flag: `--priorityFeesConstant=AMOUNT`
Example: `--priorityFeesConstant=1000`

You cannot use both flags at one time
