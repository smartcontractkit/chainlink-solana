package main

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	opsCore "github.com/smartcontractkit/chainlink-relay/ops"
	"github.com/smartcontractkit/chainlink-solana/ops/solana"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		solanaClient, err := solana.New(ctx)
		if err != nil {
			return err
		}

		// start creating environment and use deployer interface for deploying contracts
		if err := opsCore.New(ctx, &solanaClient, ObservationSource, JuelsSource, RelayConfig); err != nil {
			return err
		}

		return nil
	})
}

func RelayConfig(ctx *pulumi.Context, addresses map[int]string) (map[string]string, error) {
	return map[string]string{
		"nodeEndpointHTTP": config.Require(ctx, "CL-RELAY_HTTP"),
		"nodeEndpointWS":   config.Require(ctx, "CL-RELAY_WS"),
		"ocr2ProgramID":    addresses[solana.OCR2],
		"transmissionsID":  addresses[solana.OCRTransmissions],
		"storeProgramID":   addresses[solana.Store],
	}, nil
}

func ObservationSource(priceAdapter string) string {
	return fmt.Sprintf(`
	 ea  [type=bridge name=%s requestData=<{"data":{"from":"LINK", "to":"USD"}}>]
	 parse [type="jsonparse" path="result"]
	 multiply [type="multiply" times=100000000]

	 ea -> parse -> multiply
	 `,
		priceAdapter)
}

// calculates juelsToX as juelsToLamports (1 LINK = 1e18 juels, 1 SOL = 1e9 lamports)
func JuelsSource(priceAdapter string) string {
	return fmt.Sprintf(`
	 link2usd [type=bridge name=%s requestData=<{"data":{"from":"LINK", "to":"USD"}}>]
	 parseL [type="jsonparse" path="result"]

	 sol2usd [type=bridge name=%s requestData=<{"data":{"from":"SOL", "to":"USD"}}>]
	 parseT [type="jsonparse" path="result"]

	 divide [type="divide" input="$(parseL)" divisor="$(parseT)" precision="9"]
   scale [type="multiply" times=1000000000]

	 link2usd -> parseL -> divide
	 sol2usd -> parseT -> divide
	 divide -> scale
	 `,
		priceAdapter, priceAdapter)
}
