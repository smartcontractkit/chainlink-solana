package solana

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	opsChainlink "github.com/smartcontractkit/chainlink-relay/ops/chainlink"
	relayUtils "github.com/smartcontractkit/chainlink-relay/ops/utils"
)

const (
	// program accounts
	AccessController = iota
	OCR2
	Store

	// program state accounts
	BillingAccessController
	RequesterAccessController
	StoreAccount
	OCRFeed
	OCRTransmissions
	LINK
	StoreAuthority
)

type Deployer struct {
	gauntlet relayUtils.Gauntlet
	network  string
	Account  map[int]string
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
		gauntlet: gauntlet,
		network:  "local",
		Account:  make(map[int]string),
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
		return errors.Wrap(err, "access controller contract deployment failed")
	}

	report, err := d.gauntlet.ReadCommandReport()
	if err != nil {
		return errors.Wrap(err, "report not available")
	}

	d.Account[AccessController] = report.Responses[0].Contract

	// Access Controller contract deployment
	fmt.Println("Deploying Store...")
	err = d.gauntlet.ExecCommand(
		"store:deploy",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return errors.Wrap(err, "store contract deployment failed")
	}

	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return errors.Wrap(err, "report not available")
	}

	d.Account[Store] = report.Responses[0].Contract

	// OCR2 contract deployment
	fmt.Println("Deploying OCR 2...")
	err = d.gauntlet.ExecCommand(
		"ocr2:deploy",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return errors.Wrap(err, "ocr 2 contract deployment failed")
	}

	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return errors.Wrap(err, "report not available")
	}
	d.Account[OCR2] = report.Responses[0].Contract

	return nil
}

func (d *Deployer) DeployLINK() error {
	fmt.Println("Deploying LINK Token...")
	err := d.gauntlet.ExecCommand(
		"token:deploy",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return errors.Wrap(err, "LINK contract deployment failed")
	}

	report, err := d.gauntlet.ReadCommandReport()
	if err != nil {
		return errors.Wrap(err, "report not available")
	}

	linkAddress := report.Responses[0].Contract
	d.Account[LINK] = linkAddress

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
		return errors.Wrap(err, "Request AC initialization failed")
	}
	report, err := d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}
	d.Account[RequesterAccessController] = report.Responses[0].Contract

	fmt.Println("Step 2: Init Billing Access Controller")
	err = d.gauntlet.ExecCommand(
		"access_controller:initialize",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return errors.Wrap(err, "Billing AC initialization failed")
	}
	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}
	d.Account[BillingAccessController] = report.Responses[0].Contract

	fmt.Println("Step 3: Init Store")
	err = d.gauntlet.ExecCommand(
		"store:initialize",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("accessController", d.Account[BillingAccessController]),
	)
	if err != nil {
		return errors.Wrap(err, "Store initialization failed")
	}
	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}
	d.Account[StoreAccount] = report.Responses[0].Contract

	fmt.Println("Step 4: Create Feed in Store")
	input := map[string]interface{}{
		"store":       d.Account[StoreAccount],
		"granularity": 30,
		"liveLength":  1024,
		"decimals":    8,
		"description": "Test LINK/USD",
	}

	jsonInput, err := json.Marshal(input)
	if err != nil {
		return err
	}

	err = d.gauntlet.ExecCommand(
		"store:create_feed",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[StoreAccount]), // why is this needed in gauntlet?
		d.gauntlet.Flag("input", string(jsonInput)),
	)
	if err != nil {
		return errors.Wrap(err, "feed creation failed")
	}

	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}
	d.Account[OCRTransmissions] = report.Data["transmissions"]

	fmt.Println("Step 5: Set Validator Config in Store")
	err = d.gauntlet.ExecCommand(
		"store:set_validator_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[StoreAccount]),
		d.gauntlet.Flag("feed", d.Account[OCRTransmissions]),
		d.gauntlet.Flag("threshold", "8000"),
	)
	if err != nil {
		return errors.Wrap(err, "set validator config failed")
	}

	fmt.Println("Step 6: Init OCR 2 Feed")
	input = map[string]interface{}{
		"minAnswer":     "0",
		"maxAnswer":     "10000000000",
		"transmissions": d.Account[OCRTransmissions],
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	// TODO: command doesn't throw an error in go if it fails
	err = d.gauntlet.ExecCommand(
		"ocr2:initialize",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("requesterAccessController", d.Account[RequesterAccessController]),
		d.gauntlet.Flag("billingAccessController", d.Account[BillingAccessController]),
		d.gauntlet.Flag("link", d.Account[LINK]),
		d.gauntlet.Flag("input", string(jsonInput)),
	)
	if err != nil {
		return errors.Wrap(err, "feed initialization failed")
	}

	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}

	d.Account[OCRFeed] = report.Data["state"]
	d.Account[StoreAuthority] = report.Data["storeAuthority"]

	fmt.Println("Step 7: Add writer to feed")
	input = map[string]interface{}{
		"transmissions": d.Account[OCRTransmissions],
		"store":         d.Account[StoreAccount],
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	err = d.gauntlet.ExecCommand(
		"store:set_writer",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("ocrState", d.Account[OCRFeed]),
		d.gauntlet.Flag("input", string(jsonInput)),
	)

	if err != nil {
		return errors.Wrap(err, "setting writer on store failed")
	}
	return nil
}

func (d Deployer) TransferLINK() error {
	err := d.gauntlet.ExecCommand(
		"token:transfer",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("to", d.Account[OCRFeed]),
		d.gauntlet.Flag("amount", "10000"),
		d.Account[LINK],
	)
	if err != nil {
		return errors.Wrap(err, "LINK transfer failed")
	}

	return nil
}

// TODO: InitOCR should cover almost the whole workflow of the OCR setup, including inspection
func (d Deployer) InitOCR(keys []opsChainlink.NodeKeys) error {

	fmt.Println("Setting up OCR Feed:")

	fmt.Println("Begin set offchain config...")
	err := d.gauntlet.ExecCommand(
		"ocr2:begin_offchain_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("version", "2"),
		d.gauntlet.Flag("state", d.Account[OCRFeed]),
	)
	if err != nil {
		return errors.Wrap(err, "begin OCR 2 set offchain config failed")
	}

	// program sorts oracles (need to pre-sort to allow correct onchainConfig generation)
	keys = keys
	sort.Slice(keys, func(i, j int) bool {
		hI, _ := hex.DecodeString(keys[i].OCR2OnchainPublicKey)
		hJ, _ := hex.DecodeString(keys[j].OCR2OnchainPublicKey)
		return bytes.Compare(hI, hJ) < 0
	})

	S := []int{}
	offChainPublicKeys := []string{}
	configPublicKeys := []string{}
	peerIDs := []string{}
	oracles := []map[string]string{}
	threshold := 1 // corresponds to F
	operators := []map[string]string{}
	for _, k := range keys {
		S = append(S, 1)
		offChainPublicKeys = append(offChainPublicKeys, k.OCR2OffchainPublicKey)
		configPublicKeys = append(configPublicKeys, k.OCR2ConfigPublicKey)
		peerIDs = append(peerIDs, k.P2PID)
		// original oracle structure
		oracles = append(oracles, map[string]string{
			"signer":      k.OCR2OnchainPublicKey,
			"transmitter": k.OCR2Transmitter,
		})

		operators = append(operators, map[string]string{
			"payee":       k.OCR2Transmitter, // payee is the same as transmitter
			"transmitter": k.OCR2Transmitter,
		})
	}

	// TODO: Should this inputs have their own struct?
	input := map[string]interface{}{
		"deltaProgressNanoseconds": 2 * time.Second,        // pacemaker (timeout rotating leaders, can't be too short)
		"deltaResendNanoseconds":   5 * time.Second,        // resending epoch (help nodes rejoin system)
		"deltaRoundNanoseconds":    1 * time.Second,        // round time (polling data source)
		"deltaGraceNanoseconds":    400 * time.Millisecond, // timeout for waiting observations beyond minimum
		"deltaStageNanoseconds":    5 * time.Second,        // transmission schedule (just for calling transmit)
		"rMax":                     3,                      // max rounds prior to rotating leader (longer could be more reliable with good leader)
		"s":                        S,
		"offchainPublicKeys":       offChainPublicKeys,
		"peerIds":                  peerIDs,
		"reportingPluginConfig": map[string]interface{}{
			"alphaReportInfinite": false,
			"alphaReportPpb":      uint64(0), // always send report
			"alphaAcceptInfinite": false,
			"alphaAcceptPpb":      uint64(0),       // accept all reports (if deviation matches number)
			"deltaCNanoseconds":   0 * time.Second, // heartbeat
		},
		"maxDurationQueryNanoseconds":                        0 * time.Millisecond,
		"maxDurationObservationNanoseconds":                  300 * time.Millisecond,
		"maxDurationReportNanoseconds":                       300 * time.Millisecond,
		"maxDurationShouldAcceptFinalizedReportNanoseconds":  1 * time.Second,
		"maxDurationShouldTransmitAcceptedReportNanoseconds": 1 * time.Second,
		"configPublicKeys":                                   configPublicKeys,
	}

	jsonInput, err := json.Marshal(input)
	if err != nil {
		return err
	}

	fmt.Println("Writing set offchain config...")
	err = d.gauntlet.ExecCommand(
		"ocr2:write_offchain_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[OCRFeed]),
		d.gauntlet.Flag("input", string(jsonInput)),
	)

	if err != nil {
		return errors.Wrap(err, "writing OCR 2 set offchain config failed")
	}

	fmt.Println("Committing set offchain config...")
	err = d.gauntlet.ExecCommand(
		"ocr2:commit_offchain_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[OCRFeed]),
	)

	if err != nil {
		return errors.Wrap(err, "committing OCR 2 set offchain config failed")
	}

	input = map[string]interface{}{
		"oracles": oracles,
		"f":       threshold,
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	fmt.Println("Setting config...")
	err = d.gauntlet.ExecCommand(
		"ocr2:set_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[OCRFeed]),
		d.gauntlet.Flag("input", string(jsonInput)),
	)

	if err != nil {
		return errors.Wrap(err, "setting OCR 2 config failed")
	}

	input = map[string]interface{}{
		"operators": operators,
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	fmt.Println("Setting Payees...")
	err = d.gauntlet.ExecCommand(
		"ocr2:set_payees",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[OCRFeed]),
		d.gauntlet.Flag("link", d.Account[LINK]),
		d.gauntlet.Flag("TESTING_ONLY_IGNORE_PAYEE_VALIDATION", "true"),
		d.gauntlet.Flag("input", string(jsonInput)),
	)

	if err != nil {
		return errors.Wrap(err, "setting OCR 2 payees failed")
	}

	input = map[string]interface{}{
		"observationPaymentGjuels":  1,
		"transmissionPaymentGjuels": 1,
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	fmt.Println("Setting Billing...")
	err = d.gauntlet.ExecCommand(
		"ocr2:set_billing",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[OCRFeed]),
		d.gauntlet.Flag("input", string(jsonInput)),
	)

	if err != nil {
		return errors.Wrap(err, "setting OCR 2 billing failed")
	}

	return nil
}

func (d Deployer) Fund(addresses []string) error {
	if _, err := exec.LookPath("solana"); err != nil {
		return errors.New("'solana' is not available in commandline")
	}
	for _, a := range addresses {
		msg := relayUtils.LogStatus(fmt.Sprintf("funded %s", a))
		if _, err := exec.Command("solana", "airdrop", "100", a).Output(); msg.Check(err) != nil {
			return err
		}
	}
	return nil
}

func (d Deployer) OCR2Address() string {
	return d.Account[OCRFeed]
}

func (d Deployer) Addresses() map[int]string {
	return d.Account
}
