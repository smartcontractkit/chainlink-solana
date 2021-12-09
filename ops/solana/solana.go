package solana

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/gagliardetto/solana-go"
	ghErrors "github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	relayUtils "github.com/smartcontractkit/chainlink-relay/ops/utils"
)

// Programs
const (
	AccessController = iota
	OCR2
)

// Program accounts
const (
	BillingAccessController = iota
	RequesterAccessController
	OCRFeed
	OCRTransmissions
	LINK
)

type Deployer struct {
	gauntlet  relayUtils.Gauntlet
	network   string
	Contracts map[int]string
	States    map[int]string
}

func New(ctx *pulumi.Context) (Deployer, error) {

	yarn, err := exec.LookPath("yarn")
	if err != nil {
		return Deployer{}, errors.New("'yarn' is not installed")
	}
	fmt.Printf("yarn is available at %s\n", yarn)

	// Change path to root directory
	cwd, _ := os.Getwd()
	os.Chdir(filepath.Join(cwd, "../gauntlet"))

	fmt.Println("Installing dependencies")
	if _, err = exec.Command(yarn).Output(); err != nil {
		return Deployer{}, errors.New("error install dependencies")
	}

	// Generate Gauntlet Binary
	fmt.Println("Generating Gauntlet binary...")
	_, err = exec.Command(yarn, "bundle").Output()
	if err != nil {
		return Deployer{}, errors.New("error generating gauntlet binary")
	}

	// TODO: Should come from pulumi context
	os.Setenv("SKIP_PROMPTS", "true")

	version := "linux"
	if config.Get(ctx, "VERSION") == "MACOS" {
		version = "macos"
	}

	// Check gauntlet works
	os.Chdir(cwd) // move back into ops folder
	gauntletBin := filepath.Join(cwd, "../gauntlet/bin/gauntlet-") + version
	gauntlet, err := relayUtils.NewGauntlet(gauntletBin)

	if err != nil {
		return Deployer{}, err
	}

	return Deployer{
		gauntlet:  gauntlet,
		network:   "local",
		Contracts: make(map[int]string),
		States:    make(map[int]string),
	}, nil
}

func (d *Deployer) Load() error {
	// TODO: remove this - temporarily needed as artifacts are read directly from the root directory
	// won't be needed once it reads from release artifacts?
	cwd, _ := os.Getwd()
	os.Chdir(filepath.Join(cwd, "../gauntlet")) // go from ops folder to gauntlet folder

	// Access Controller contract deployment
	fmt.Println("Deploying Access Controller...")
	err := d.gauntlet.ExecCommand(
		"access_controller:deploy",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return ghErrors.Wrap(err, "access controller contract deployment failed")
	}

	report, err := d.gauntlet.ReadCommandReport()
	if err != nil {
		return ghErrors.Wrap(err, "report not available")
	}

	d.Contracts[AccessController] = report.Responses[0].Contract

	// OCR2 contract deployment
	fmt.Println("Deploying OCR 2...")
	err = d.gauntlet.ExecCommand(
		"ocr2:deploy",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return ghErrors.Wrap(err, "ocr 2 contract deployment failed")
	}

	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return ghErrors.Wrap(err, "report not available")
	}
	d.Contracts[OCR2] = report.Responses[0].Contract

	return nil
}

func (d *Deployer) DeployLINK() error {
	fmt.Println("Deploying LINK Token...")
	err := d.gauntlet.ExecCommand(
		"token:deploy",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return ghErrors.Wrap(err, "LINK contract deployment failed")
	}

	report, err := d.gauntlet.ReadCommandReport()
	if err != nil {
		return ghErrors.Wrap(err, "report not available")
	}

	linkAddress := report.Responses[0].Contract
	d.States[LINK] = linkAddress

	return nil
}

func (d *Deployer) DeployOCR() error {
	fmt.Println("Deploying OCR Feed:")
	fmt.Println("Step 1: Init Requester Access Controller")
	err := d.gauntlet.ExecCommand(
		"access_controller:initialize",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return ghErrors.Wrap(err, "AC initialization failed")
	}
	report, err := d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}
	d.States[RequesterAccessController] = report.Responses[0].Contract

	fmt.Println("Step 2: Init Billing Access Controller")
	err = d.gauntlet.ExecCommand(
		"access_controller:initialize",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return ghErrors.Wrap(err, "AC initialization failed")
	}
	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}
	d.States[BillingAccessController] = report.Responses[0].Contract

	input := map[string]interface{}{
		"minAnswer":   "0",
		"maxAnswer":   "10000000000",
		"decimals":    9,
		"description": "Test",
	}

	jsonInput, err := json.Marshal(input)
	if err != nil {
		return err
	}

	fmt.Println("Step 3: Init OCR 2 Feed")
	// TODO: command doesn't throw an error in go if it fails
	err = d.gauntlet.ExecCommand(
		"ocr2:initialize",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("requesterAccessController", d.States[RequesterAccessController]),
		d.gauntlet.Flag("billingAccessController", d.States[BillingAccessController]),
		d.gauntlet.Flag("link", d.States[LINK]),
		d.gauntlet.Flag("input", string(jsonInput)),
	)
	if err != nil {
		return ghErrors.Wrap(err, "feed initialization failed")
	}

	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}

	d.States[OCRFeed] = report.Data["state"]
	d.States[OCRTransmissions] = report.Data["transmissions"]

	return nil
}

func (d Deployer) TransferLINK() error {
	err := d.gauntlet.ExecCommand(
		"token:transfer",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("to", d.States[OCRFeed]),
		d.gauntlet.Flag("amount", "10000"),
		d.States[LINK],
	)
	if err != nil {
		return ghErrors.Wrap(err, "LINK transfer failed")
	}

	return nil
}

// TODO: InitOCR should cover almost the whole workflow of the OCR setup, including inspection
func (d Deployer) InitOCR(keys []map[string]string) error {

	fmt.Println("Setting up OCR Feed:")

	fmt.Println("Begin set offchain config...")
	err := d.gauntlet.ExecCommand(
		"ocr2:begin_offchain_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("version", "1"),
		d.gauntlet.Flag("state", d.States[OCRFeed]),
	)
	if err != nil {
		return ghErrors.Wrap(err, "begin OCR 2 set offchain config failed")
	}

	S := []int{}
	offChainPublicKeys := []string{}
	peerIDs := []string{}
	oracles := []map[string]string{}
	threshold := 1
	// operators := []map[string]string{}
	for _, k := range keys {
		S = append(S, 1)
		offChainPublicKeys = append(offChainPublicKeys, k["OCROffchainPublicKey"])
		peerIDs = append(peerIDs, k["P2PID"])
		oracles = append(oracles, map[string]string{
			"signer":      k["OCROnchainPublicKey"],
			"transmitter": k["NodeAddress"],
		})
		// operators = append(operators, map[string]string{
		// 	"payee":       k["OCRPayeeAddress"],
		// 	"transmitter": k["NodeAddress"],
		// })
	}

	// TODO: Should this inputs have their own struct?
	input := map[string]interface{}{
		"deltaProgressNanoseconds": 2 * time.Second,
		"deltaResendNanoseconds":   5 * time.Second,
		"deltaRoundNanoseconds":    1 * time.Second,
		"deltaGraceNanoseconds":    500 * time.Millisecond,
		"deltaStageNanoseconds":    5 * time.Second,
		"rMax":                     3,
		"s":                        S,
		"offchainPublicKeys":       offChainPublicKeys,
		"peerIds":                  peerIDs,
		"reportingPluginConfig": map[string]interface{}{
			"alphaReportInfinite": false,
			"alphaReportPpb":      uint64(1000000),
			"alphaAcceptInfinite": false,
			"alphaAcceptPpb":      uint64(1000000),
			"deltaCNanoseconds":   15 * time.Second,
		},
		"maxDurationQueryNanoseconds":                        2 * time.Second,
		"maxDurationObservationNanoseconds":                  2 * time.Second,
		"maxDurationReportNanoseconds":                       2 * time.Second,
		"maxDurationShouldAcceptFinalizedReportNanoseconds":  2 * time.Second,
		"maxDurationShouldTransmitAcceptedReportNanoseconds": 2 * time.Second,
	}

	jsonInput, err := json.Marshal(input)
	if err != nil {
		return err
	}

	fmt.Println("Writing set offchain config...")
	err = d.gauntlet.ExecCommand(
		"ocr2:write_offchain_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.States[OCRFeed]),
		d.gauntlet.Flag("input", string(jsonInput)),
	)

	if err != nil {
		return ghErrors.Wrap(err, "writing OCR 2 set offchain config failed")
	}

	fmt.Println("Committing set offchain config...")
	err = d.gauntlet.ExecCommand(
		"ocr2:commit_offchain_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.States[OCRFeed]),
	)

	if err != nil {
		return ghErrors.Wrap(err, "committing OCR 2 set offchain config failed")
	}

	input = map[string]interface{}{
		"oracles":   oracles,
		"threshold": threshold,
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	fmt.Println("Setting config...")
	err = d.gauntlet.ExecCommand(
		"ocr2:set_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.States[OCRFeed]),
		d.gauntlet.Flag("input", string(jsonInput)),
	)

	if err != nil {
		return ghErrors.Wrap(err, "setting OCR 2 config failed")
	}

	// SET PAYEES
	// TODO: Uncomment if needed
	// input = map[string]interface{}{
	// 	"operators": operators,
	// }

	// jsonInput, err = json.Marshal(input)
	// if err != nil {
	// 	return err
	// }

	// fmt.Println("Setting Payees...")
	// err = d.gauntlet.ExecCommand(
	// 	"ocr2:set_payees",
	// 	d.gauntlet.Flag("network", d.network),
	// 	d.gauntlet.Flag("state", d.States[OCRFeed]),
	// 	d.gauntlet.Flag("input", string(jsonInput)),
	// )

	// if err != nil {
	// 	return ghErrors.Wrap(err, "setting OCR 2 payees failed")
	// }

	return nil
}

func (d Deployer) Fund(addresses []string) error {
	if _, err := exec.LookPath("solana"); err != nil {
		return errors.New("'solana' is not available in commandline")
	}
	for _, a := range addresses {
		// parse hex encoded pubkey to base58 encoded account
		keyByte, err := hex.DecodeString(a)
		if err != nil {
			return err
		}
		a = solana.PublicKeyFromBytes(keyByte).String()

		msg := relayUtils.LogStatus(fmt.Sprintf("funded %s", a))
		if _, err := exec.Command("solana", "airdrop", "100", a).Output(); msg.Check(err) != nil {
			return err
		}
	}
	return nil
}

func (d Deployer) OCR2Address() string {
	return d.States[OCRFeed]
}

func (d Deployer) Addresses() map[int]string {
	return d.States
}
