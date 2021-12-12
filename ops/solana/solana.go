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

	"github.com/gagliardetto/solana-go"
	"github.com/pkg/errors"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	relayUtils "github.com/smartcontractkit/chainlink-relay/ops/utils"
	"github.com/smartcontractkit/libocr/offchainreporting2/confighelper"
	"github.com/smartcontractkit/libocr/offchainreporting2/reportingplugin/median"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

const (
	// program accounts
	AccessController = iota
	OCR2
	Validator

	// program state accounts
	BillingAccessController
	RequesterAccessController
	ValidatorAccount
	OCRFeed
	OCRTransmissions
	LINK
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
	fmt.Println("Deploying Validator...")
	err = d.gauntlet.ExecCommand(
		"deviation_flagging_validator:deploy",
		d.gauntlet.Flag("network", d.network),
	)
	if err != nil {
		return errors.Wrap(err, "validator contract deployment failed")
	}

	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return errors.Wrap(err, "report not available")
	}

	d.Account[Validator] = report.Responses[0].Contract

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

	fmt.Println("Step 3: Init Validator")
	err = d.gauntlet.ExecCommand(
		"deviation_flagging_validator:initialize",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("accessController", d.Account[BillingAccessController]),
	)
	if err != nil {
		return errors.Wrap(err, "Validator initialization failed")
	}
	report, err = d.gauntlet.ReadCommandReport()
	if err != nil {
		return err
	}
	d.Account[ValidatorAccount] = report.Responses[0].Contract

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

	fmt.Println("Step 4: Init OCR 2 Feed")
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
	d.Account[OCRTransmissions] = report.Data["transmissions"]

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
func (d Deployer) InitOCR(keys []map[string]string) error {

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

	S := []int{}
	// offChainPublicKeys := []string{}
	// configPublicKeys := []string{}
	// peerIDs := []string{}
	oracles := []map[string]string{}
	threshold := 1
	operators := []map[string]string{}
	helperOracles := []confighelper.OracleIdentityExtra{}
	for _, k := range keys {
		S = append(S, 1)
		// offChainPublicKeys = append(offChainPublicKeys, k["OCROffchainPublicKey"])
		// configPublicKeys = append(configPublicKeys, k["OCRConfigPublicKey"])
		// peerIDs = append(peerIDs, k["P2PID"])
		// original oracle structure
		// oracles = append(oracles, map[string]string{
		// 	"signer":      k["OCROnchainPublicKey"],
		// 	"transmitter": k["OCRTransmitter"],
		// })
		// oracle := map[string]string{
		// 	"signer":      strings.TrimPrefix(k["OCROnchainPublicKey"], "0x"),  // TODO: temporary parsing of 0x... hex key to hex key
		// 	"transmitter": solana.PublicKeyFromBytes(transmitKeyByte).String(), // TODO: temporary parsing from hex encoded to base58 encoded
		// }
		// oracles = append(oracles, oracle)
		// operators = append(operators, map[string]string{
		// 	"payee":       k["OCRPayeeAddress"],
		// 	"transmitter": k["NodeAddress"],
		// })

		offchainPKByte, err := hex.DecodeString(k["OCROffchainPublicKey"])
		if err != nil {
			return err
		}
		onchainPKByte, err := hex.DecodeString(k["OCROnchainPublicKey"])
		if err != nil {
			return err
		}
		configPKByteTemp, err := hex.DecodeString(k["OCRConfigPublicKey"])
		if err != nil {
			return err
		}
		configPKByte := [32]byte{}
		copy(configPKByte[:], configPKByteTemp)
		helperOracles = append(helperOracles, confighelper.OracleIdentityExtra{
			OracleIdentity: confighelper.OracleIdentity{
				OffchainPublicKey: types.OffchainPublicKey(offchainPKByte),
				OnchainPublicKey:  types.OnchainPublicKey(onchainPKByte),
				PeerID:            k["P2PID"],
				TransmitAccount:   types.Account(k["OCRTransmitter"]),
			},
			ConfigEncryptionPublicKey: types.ConfigEncryptionPublicKey(configPKByte),
		})
	}

	// // TODO: Should this inputs have their own struct?
	// input := map[string]interface{}{
	// 	"deltaProgressNanoseconds": 2 * time.Second,
	// 	"deltaResendNanoseconds":   5 * time.Second,
	// 	"deltaRoundNanoseconds":    1 * time.Second,
	// 	"deltaGraceNanoseconds":    500 * time.Millisecond,
	// 	"deltaStageNanoseconds":    5 * time.Second,
	// 	"rMax":                     3,
	// 	"s":                        S,
	// 	"offchainPublicKeys":       offChainPublicKeys,
	// 	"peerIds":                  peerIDs,
	// 	"reportingPluginConfig": map[string]interface{}{
	// 		"alphaReportInfinite": false,
	// 		"alphaReportPpb":      uint64(1000000),
	// 		"alphaAcceptInfinite": false,
	// 		"alphaAcceptPpb":      uint64(1000000),
	// 		"deltaCNanoseconds":   15 * time.Second,
	// 	},
	// 	"maxDurationQueryNanoseconds":                        2 * time.Second,
	// 	"maxDurationObservationNanoseconds":                  2 * time.Second,
	// 	"maxDurationReportNanoseconds":                       2 * time.Second,
	// 	"maxDurationShouldAcceptFinalizedReportNanoseconds":  2 * time.Second,
	// 	"maxDurationShouldTransmitAcceptedReportNanoseconds": 2 * time.Second,
	// 	"configPublicKeys":                                   configPublicKeys,
	// }
	//
	// jsonInput, err := json.Marshal(input)
	// if err != nil {
	// 	return err
	// }

	// program sorts oracles (need to pre-sort to allow correct onchainConfig generation)
	sort.Slice(helperOracles, func(i, j int) bool {
		return bytes.Compare(helperOracles[i].OracleIdentity.OnchainPublicKey, helperOracles[j].OracleIdentity.OnchainPublicKey) < 0
	})

	alphaPPB := uint64(1000000)
	signers, transmitters, _, _, _, onchainConfig, err := confighelper.ContractSetConfigArgsForTests(
		2*time.Second,        // deltaProgress time.Duration,
		5*time.Second,        // deltaResend time.Duration,
		1*time.Second,        // deltaRound time.Duration,
		500*time.Millisecond, // deltaGrace time.Duration,
		5*time.Second,        // deltaStage time.Duration,
		3,                    // rMax uint8,
		S,                    // s []int,
		helperOracles,        // oracles []OracleIdentityExtra,
		median.OffchainConfig{
			false,
			alphaPPB,
			false,
			alphaPPB,
			0,
		}.Encode(), //reportingPluginConfig []byte,
		500*time.Millisecond, // maxDurationQuery time.Duration,
		500*time.Millisecond, // maxDurationObservation time.Duration,
		500*time.Millisecond, // maxDurationReport time.Duration,
		2*time.Second,        // maxDurationShouldAcceptFinalizedReport time.Duration,
		2*time.Second,        // maxDurationShouldTransmitAcceptedReport time.Duration,
		1,                    // f int,
		[]byte{},             // onchainConfig []byte
	)
	for i := 0; i < len(signers); i++ {
		oracles = append(oracles, map[string]string{
			"signer":      hex.EncodeToString(signers[i]),
			"transmitter": string(transmitters[i]),
		})

		operators = append(operators, map[string]string{
			"payee":       string(transmitters[i]), // payee is the same as transmitter
			"transmitter": string(transmitters[i]),
		})
	}

	fmt.Println("Writing set offchain config...")
	err = d.gauntlet.ExecCommand(
		"ocr2:write_offchain_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[OCRFeed]),
		// d.gauntlet.Flag("input", string(jsonInput)),
		d.gauntlet.Flag("raw", hex.EncodeToString(onchainConfig)),
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

	input := map[string]interface{}{
		"oracles":   oracles,
		"threshold": threshold,
	}

	jsonInput, err := json.Marshal(input)
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
		d.gauntlet.Flag("input", string(jsonInput)),
	)

	if err != nil {
		return errors.Wrap(err, "setting OCR 2 payees failed")
	}

	input = map[string]interface{}{
		"observationPayment":  1,
		"transmissionPayment": 1,
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

	input = map[string]interface{}{
		"validator": d.Account[ValidatorAccount],
		"threshold": 8000,
	}

	jsonInput, err = json.Marshal(input)
	if err != nil {
		return err
	}

	fmt.Println("Setting OCR 2 validator config...")
	err = d.gauntlet.ExecCommand(
		"ocr2:set_validator_config",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[OCRFeed]),
		d.gauntlet.Flag("input", string(jsonInput)),
	)

	if err != nil {
		return errors.Wrap(err, "setting OCR 2 validator config failed")
	}

	fmt.Println("Adding feed to validator access list...")
	seeds := [][]byte{[]byte("validator"), solana.MustPublicKeyFromBase58(d.Account[OCRFeed]).Bytes()}
	validatorAuthority, _, err := solana.FindProgramAddress(seeds, solana.MustPublicKeyFromBase58(d.Account[OCR2]))
	if err != nil {
		return errors.Wrap(err, "fetching validator authority failed")
	}

	err = d.gauntlet.ExecCommand(
		"access_controller:add_access",
		d.gauntlet.Flag("network", d.network),
		d.gauntlet.Flag("state", d.Account[BillingAccessController]),
		d.gauntlet.Flag("address", validatorAuthority.String()),
	)

	if err != nil {
		return errors.Wrap(err, "adding feed to validator access list failed")
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
	return d.Account[OCR2]
}

func (d Deployer) Addresses() map[int]string {
	return d.Account
}
