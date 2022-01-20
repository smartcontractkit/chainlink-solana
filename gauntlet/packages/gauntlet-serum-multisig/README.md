# Gauntlet Serum Multisig

Current version 0.7.0

Artifacts: https://github.com/project-serum/multisig/releases/tag/v0.7.0

## Creating a Multisig

Example with 3 owners and threshold=2

`yarn gauntlet-serum-multisig create --network=local 3W37Aopzbtzczi8XWdkFTvBeSyYgXLuUkaodkq59xBCT ETqajtkz4xcsB397qTBPetprR8jMC3JszkjJJp3cjWJS QMaHW2Fpyet4ZVf7jgrGB6iirZLjwZUjN9vPKcpQrHs --threshold=2`

## Actions

Rest of the commands will adhere to the following flow:

### Create

You run a regular command, and you just replace `gauntlet` with `gauntlet-serum-multisig`.
For e.g if you have this command: `yarn gauntlet ocr2:set_billing --network=local --state=k91NrbTgTt4bo86fXN3SXqUzVvoDRiivxf2KcU1p5Gp`
it becomes:
`yarn gauntlet-serum-multisig ocr2:set_billing --network=local --state=k91NrbTgTt4bo86fXN3SXqUzVvoDRiivxf2KcU1p5Gp`

The creator automatically signs/approves the proposal.
You get a message on console with the proposal PublicKey that you need for continuing on next actions

### Approve/Execute

You run a previously created command and you just append `--proposal` flag,
For e.g for the above: `yarn gauntlet-serum-multisig ocr2:set_billing --network=local --state=k91NrbTgTt4bo86fXN3SXqUzVvoDRiivxf2KcU1p5Gp --proposal=CyU1HR7Ebs4aQVQVabT6KeNFusHqov1nwCpCDs9CRZhw`

If the threshold is met, the proposal is executed. Else, you need to repeat the action for the rest of the owners until the threshold is met.

### Setting owners

Example of setting one more owner to the above created multisig

`yarn gauntlet-serum-multisig set:owners --network=local QMaHW2Fpyet4ZVf7jgrGB6iirZLjwZUjN9vPKcpQrHs ETqajtkz4xcsB397qTBPetprR8jMC3JszkjJJp3cjWJS 3W37Aopzbtzczi8XWdkFTvBeSyYgXLuUkaodkq59xBCT 8i1ZbY9S7VPV4AKVEL1xewyYFvtkrjAnffqfsCf3FoRB`

### Setting threshold

Example of setting threshold to 3

`yarn gauntlet-serum-multisig set:threshold --network=local --threshold=1`
