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
	Proposal
)

const (
	testingSecret = "this is an testing only secret"
)

type Deployer struct {
	gauntlet relayUtils.Gauntlet
	network  string
	Account  map[int]string
}

func New(ctx *pulumi.Context) (Deployer, error) {
	// TODO: Should come from pulumi context
	os.Setenv("SKIP_PROMPTS", "true")

	cwd, _ := os.Getwd()
	path := filepath.Join(cwd, "../gauntlet")
	gauntlet, err := relayUtils.NewGauntlet(path)
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

	fmt.Println("Step 3: Create Store")
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

	fmt.Println("Step 4: Create Feed")
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

	fmt.Println("Step 5: Set Validator Config in Feed")
	err = d.gauntlet.ExecCommand(
		"store:set_validator_config",
		d.gauntlet.Flag("network", d.network),
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
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	err = d.gauntlet.ExecCommand(
		"store:set_writer",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("input", string(jsonInput)),
		d.Account[OCRFeed],
	)

	if err != nil {
		return errors.Wrap(err, "setting writer on store failed")
	}

	fmt.Println("Step 8: Transfer feed ownership to store")
	if err = d.gauntlet.ExecCommand(
		"store:transfer_feed_ownership",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[OCRTransmissions]),
		d.gauntlet.Flag("to", d.Account[StoreAccount]),
	); err != nil {
		return errors.Wrap(err, "failed to transfer feed ownership")
	}

	if err = d.gauntlet.ExecCommand(
		"store:accept_feed_ownership",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[OCRTransmissions]),
		d.gauntlet.Flag("to", d.Account[StoreAccount]),
	); err != nil {
		return errors.Wrap(err, "failed to accept feed ownership")
	}

	return nil
}

func (d Deployer) TransferLINK() error {
	err := d.gauntlet.ExecCommand(
		"ocr2:fund",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("amount", "10000"),
		d.gauntlet.Flag("link", d.Account[LINK]),
		d.Account[OCRFeed],
	)
	if err != nil {
		return errors.Wrap(err, "LINK transfer failed")
	}

	return nil
}

// TODO: InitOCR should cover almost the whole workflow of the OCR setup, including inspection
func (d Deployer) InitOCR(keys []opsChainlink.NodeKeys) error {

	fmt.Println("Setting up OCR Feed:")

	fmt.Println("Begin offchain proposal...")
	if err := d.gauntlet.ExecCommand(
		"ocr2:create_proposal",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("version", "2"),
	); err != nil {
		return errors.Wrap(err, "create proposal failed")
	}

	report, err := d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}
	d.Account[Proposal] = report.Data["proposal"]

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
	// operators := []map[string]string{}
	for _, k := range keys {
		S = append(S, 1)
		offChainPublicKeys = append(offChainPublicKeys, k.OCR2OffchainPublicKey)
		configPublicKeys = append(configPublicKeys, k.OCR2ConfigPublicKey)
		peerIDs = append(peerIDs, k.P2PID)
		// original oracle structure
		oracles = append(oracles, map[string]string{
			"signer":      k.OCR2OnchainPublicKey,
			"transmitter": k.OCR2Transmitter,
			"payee":       k.OCR2Transmitter, // payee is the same as transmitter
		})

		// operators = append(operators, map[string]string{
		// 	"payee":       k.OCR2Transmitter, // payee is the same as transmitter
		// 	"transmitter": k.OCR2Transmitter,
		// })
	}

	fmt.Println("Proposing config...")
	input := map[string]interface{}{
		"oracles":    oracles,
		"f":          threshold,
		"proposalId": d.Account[Proposal],
	}

	jsonInput, err := json.Marshal(input)
	if err != nil {
		return err
	}

	err = d.gauntlet.ExecCommand(
		"ocr2:propose_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("proposalId", d.Account[Proposal]),
		d.gauntlet.Flag("input", string(jsonInput)),
		d.Account[OCRFeed],
	)

	if err != nil {
		return errors.Wrap(err, "setting OCR 2 config failed")
	}

	fmt.Println("Proposing offchain config...")
	offchainConfig := map[string]interface{}{
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

	input = map[string]interface{}{
		"proposalId": d.Account[Proposal],
		"offchainConfig": offchainConfig,
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	if err = d.gauntlet.ExecCommand(
		"ocr2:propose_offchain_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("proposalId", d.Account[Proposal]),
		d.gauntlet.Flag("secret", testingSecret),
		d.gauntlet.Flag("input", string(jsonInput)),
		d.Account[OCRFeed],
	); err != nil {
		return errors.Wrap(err, "proposing OCR2 offchain config failed")
	}

	fmt.Println("Proposing Payees...")
	input = map[string]interface{}{
		"operators":          oracles,
		"proposalId":         d.Account[Proposal],
		"allowFundRecipient": true,
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	if err = d.gauntlet.ExecCommand(
		"ocr2:propose_payees",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("link", d.Account[LINK]),
		d.gauntlet.Flag("proposalId", d.Account[Proposal]),
		d.gauntlet.Flag("input", string(jsonInput)),
		d.Account[OCRFeed],
	); err != nil {
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

	fmt.Println("Finalize proposal...")
	if err = d.gauntlet.ExecCommand(
		"ocr2:finalize_proposal",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("proposalId", d.Account[Proposal]),
	); err != nil {
		return errors.Wrap(err, "committing OCR 2 set offchain config failed")
	}

	fmt.Println("Accept proposal...")
	input = map[string]interface{}{
		"version":  2,
		"f": threshold,
		"oracles": oracles,
		"offchainConfig": offchainConfig,
		"randomSecret": testingSecret,
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}
	if err = d.gauntlet.ExecCommand(
		"ocr2:accept_proposal",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("proposalId", d.Account[Proposal]),
		d.gauntlet.Flag("secret", testingSecret),
		d.gauntlet.Flag("input", string(jsonInput)),
		d.Account[OCRFeed],
	); err != nil {
		return errors.Wrap(err, "failed to accept proposal")
	}

	fmt.Println("Setting Billing...")
	if err = d.gauntlet.ExecCommand(
		"ocr2:set_billing",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("input", string(jsonInput)),
		d.Account[OCRFeed],
	); err != nil {
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
