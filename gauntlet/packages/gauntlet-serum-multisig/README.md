# Gauntlet Serum Multisig

Current version 0.7.0

Artifacts: https://github.com/project-serum/multisig/releases/tag/v0.7.0

## Creating a Multisig

Here is an example with 3 owners and a threshold of 2:

`yarn gauntlet serum_multisig:create --network=local 3W37Aopzbtzczi8XWdkFTvBeSyYgXLuUkaodkq59xBCT ETqajtkz4xcsB397qTBPetprR8jMC3JszkjJJp3cjWJS QMaHW2Fpyet4ZVf7jgrGB6iirZLjwZUjN9vPKcpQrHs --threshold=2`

You will get 2 addresses, Multisig Address and Multisig Signer. Please keep them both as they will both be needed. Signer will be used when granting access/ownership to multisig.

## Actions

The rest of the commands will adhere to the following flow:

### Create

Run a regular command and simply append the command name with `:multisig`.
For e.g if you have this command: `yarn gauntlet ocr2:set_billing --network=local --state=k91NrbTgTt4bo86fXN3SXqUzVvoDRiivxf2KcU1p5Gp` 
it becomes: `yarn gauntlet ocr2:set_billing:multisig --network=local --state=k91NrbTgTt4bo86fXN3SXqUzVvoDRiivxf2KcU1p5Gp --execute`

The creator automatically signs/approves the proposal.

You get a message on console with the proposal PublicKey that you need for continuing with next actions.

Always include `--execute` if you want to sign and send the transaction. Otherwise, the command will only print out the message that you must sign and send.

### Approve/Execute

Run a previously created command and simply append `--proposal` flag,
For e.g for the above: `yarn gauntlet ocr2:set_billing:multisig --network=local --state=k91NrbTgTt4bo86fXN3SXqUzVvoDRiivxf2KcU1p5Gp --proposal=CyU1HR7Ebs4aQVQVabT6KeNFusHqov1nwCpCDs9CRZhw --execute`

If the threshold is met, the proposal is executed. Else, you need to repeat the action for the rest of the owners until the threshold is met.

### Setting owners

An example of setting one more owner to the above created multisig:

`yarn gauntlet serum_multisig:set_owners:multisig --network=local QMaHW2Fpyet4ZVf7jgrGB6iirZLjwZUjN9vPKcpQrHs ETqajtkz4xcsB397qTBPetprR8jMC3JszkjJJp3cjWJS 3W37Aopzbtzczi8XWdkFTvBeSyYgXLuUkaodkq59xBCT 8i1ZbY9S7VPV4AKVEL1xewyYFvtkrjAnffqfsCf3FoRB --execute`

### Setting threshold

An example of changing threshold to 3:

`yarn gauntlet serum_multisig:change_threshold:multisig --network=local --threshold=3 --execute`
