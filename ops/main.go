package main

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	opsCore "github.com/smartcontractkit/chainlink-relay/ops"
	"github.com/smartcontractkit/solana-integration/ops/solana"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		solanaClient, err := solana.New(ctx)
		if err != nil {
			return err
		}

		// start creating environment and use deployer interface for deploying contracts
		if err := opsCore.New(ctx, &solanaClient, ObservationSource); err != nil {
			return err
		}

		return nil
	})
}

// calculates juelsToX as juelsToLamports (1 LINK = 1e18 juels, 1 SOL = 1e9 lamports)
func ObservationSource(priceAdapter, relay string) string {
	return fmt.Sprintf(`
	 ea  [type=bridge name=%s requestData=<{"data":{"from":"LINK", "to":"USD"}}>]
	 parse [type="jsonparse" path="result"]
	 multiply [type="multiply" times=100000000]

	 link2usd [type=bridge name=%s requestData=<{"data":{"from":"LINK", "to":"USD"}}>]
	 parseL [type="jsonparse" path="result"]

	 luna2usd [type=bridge name=%s requestData=<{"data":{"from":"SOL", "to":"USD"}}>]
	 parseT [type="jsonparse" path="result"]

	 divide [type="divide" input="$(parseL)" divisor="$(parseT)" precision="9"]
   scale [type="multiply" times=1000000000]

	 return [type=bridge name="%s" requestData=<{"jobID":$(jobSpec.externalJobID), "data":{"result":$(multiply), "juelsToX":$(scale)}}>]

	 ea -> parse -> multiply -> return
	 link2usd -> parseL -> divide
	 luna2usd -> parseT -> divide
	 divide -> scale -> return
	 `,
		priceAdapter, priceAdapter, priceAdapter, relay)
}
