package gauntlet

import (
	"encoding/json"
	"fmt"
	"os"

	ocr2_config "github.com/smartcontractkit/chainlink-solana/integration-tests/config"

	"github.com/smartcontractkit/chainlink-testing-framework/gauntlet"
)

var (
	sg *SolanaGauntlet
)

type SolanaGauntlet struct {
	Dir                      string
	NetworkFilePath          string
	G                        *gauntlet.Gauntlet
	gr                       *GauntletResponse
	options                  *gauntlet.ExecCommandOptions
	AccessControllerAddress  string
	BillingControllerAddress string
	StoreAddress             string
	FeedAddress              string
	OcrAddress               string
	ProposalAddress          string
	OCR2Config               *ocr2_config.OCR2Config
	LinkAddress              string
	VaultAddress             string
}

// GauntletResponse Default response output for starknet gauntlet commands
type GauntletResponse struct {
	Responses []struct {
		Tx struct {
			Hash    string `json:"hash"`
			Address string `json:"address"`
			Status  string `json:"status"`

			Tx struct {
				Address         string   `json:"address"`
				Code            string   `json:"code"`
				Result          []string `json:"result"`
				TransactionHash string   `json:"transaction_hash"`
			} `json:"tx"`
		} `json:"tx"`
		Contract string `json:"contract"`
	} `json:"responses"`
	Data struct {
		Proposal            *string         `json:"proposal,omitempty"`
		LatestTransmissions *[]Transmission `json:"latestTransmissions,omitempty"`
		Vault               *string         `json:"vault,omitempty"`
	}
}

type Transmission struct {
	LatestTransmissionNo int64  `json:"latestTransmissionNo"`
	RoundId              int64  `json:"roundId"`
	Answer               int64  `json:"answer"`
	Transmitter          string `json:"transmitter"`
}

// NewSolanaGauntlet Creates a default gauntlet config
func NewSolanaGauntlet(workingDir string) (*SolanaGauntlet, error) {
	g, err := gauntlet.NewGauntlet()
	g.SetWorkingDir(workingDir)
	if err != nil {
		return nil, err
	}
	sg = &SolanaGauntlet{
		Dir:             workingDir,
		NetworkFilePath: workingDir + "/packages/gauntlet-solana-contracts/networks",
		G:               g,
		gr:              &GauntletResponse{},
		options: &gauntlet.ExecCommandOptions{
			ErrHandling:       []string{},
			CheckErrorsInRead: true,
		},
		OCR2Config: &ocr2_config.OCR2Config{
			OnChainConfig:        &ocr2_config.OCR2OnChainConfig{},
			OffChainConfig:       &ocr2_config.OCROffChainConfig{},
			PayeeConfig:          &ocr2_config.PayeeConfig{},
			ProposalAcceptConfig: &ocr2_config.ProposalAcceptConfig{},
		},
	}
	return sg, nil
}

// FetchGauntletJsonOutput Parse gauntlet json response that is generated after yarn gauntlet command execution
func (sg *SolanaGauntlet) FetchGauntletJsonOutput() (*GauntletResponse, error) {
	var payload = &GauntletResponse{}
	gauntletOutput, err := os.ReadFile(sg.Dir + "/report.json")
	if err != nil {
		return payload, err
	}
	err = json.Unmarshal(gauntletOutput, &payload)
	if err != nil {
		return payload, err
	}
	return payload, nil
}

// SetupNetwork Sets up a new network and sets the NODE_URL for Devnet / Starknet RPC
func (sg *SolanaGauntlet) SetupNetwork(args map[string]string) error {
	for key, arg := range args {
		sg.G.AddNetworkConfigVar(key, arg)
	}
	err := sg.G.WriteNetworkConfigMap(sg.NetworkFilePath)
	if err != nil {
		return err
	}

	return nil
}

func (sg *SolanaGauntlet) InstallDependencies() error {
	sg.G.Command = "yarn"
	_, err := sg.G.ExecCommand([]string{"install"}, *sg.options)
	if err != nil {
		return err
	}
	sg.G.Command = "gauntlet"
	return nil
}

func (sg *SolanaGauntlet) InitializeAccessController() (string, error) {
	_, err := sg.G.ExecCommand([]string{"access_controller:initialize"}, *sg.options)
	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) DeployLinkToken() error {
	_, err := sg.G.ExecCommand([]string{"token:deploy"}, *sg.options)
	if err != nil {
		return err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return err
	}
	sg.VaultAddress = *sg.gr.Data.Vault
	sg.LinkAddress = *&sg.gr.Responses[0].Contract

	return nil
}

func (sg *SolanaGauntlet) InitializeStore(billingController string) (string, error) {
	_, err := sg.G.ExecCommand([]string{"store:initialize", fmt.Sprintf("--accessController=%s", billingController)}, *sg.options)
	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) StoreCreateFeed(length int, feedConfig *ocr2_config.StoreFeedConfig) (string, error) {
	config, err := json.Marshal(feedConfig)
	if err != nil {
		return "", err
	}
	_, err = sg.G.ExecCommand([]string{"store:create_feed", fmt.Sprintf("--length=%d", length), fmt.Sprintf("--input=%v", string(config))}, *sg.options)
	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return sg.gr.Responses[0].Tx.Address, nil
}

func (sg *SolanaGauntlet) StoreSetValidatorConfig(feedAddress string, threshold int) (string, error) {
	_, err := sg.G.ExecCommand([]string{"store:set_validator_config", fmt.Sprintf("--feed=%s", feedAddress), fmt.Sprintf("--threshold=%d", threshold)}, *sg.options)
	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) InitializeOCR2(requesterAccessController string, billingAccessController string, ocrConfig *ocr2_config.OCR2TransmitConfig) (string, error) {
	config, err := json.Marshal(ocrConfig)
	if err != nil {
		return "", err
	}
	_, err = sg.G.ExecCommand([]string{
		"ocr2:initialize",
		fmt.Sprintf("--requesterAccessController=%s", requesterAccessController),
		fmt.Sprintf("--billingAccessController=%s", billingAccessController),
		fmt.Sprintf("--input=%v", string(config))}, *sg.options)
	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) StoreSetWriter(storeConfig *ocr2_config.StoreWriterConfig, ocrAddress string) (string, error) {
	config, err := json.Marshal(storeConfig)
	if err != nil {
		return "", err
	}
	_, err = sg.G.ExecCommand([]string{
		"store:set_writer",
		fmt.Sprintf("--input=%v", string(config)),
		ocrAddress,
	},
		*sg.options,
	)

	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) OCR2SetBilling(ocr2BillingConfig *ocr2_config.OCR2BillingConfig, ocrAddress string) (string, error) {
	config, err := json.Marshal(ocr2BillingConfig)
	if err != nil {
		return "", err
	}
	_, err = sg.G.ExecCommand([]string{
		"ocr2:set_billing",
		fmt.Sprintf("--input=%v", string(config)),
		ocrAddress,
	},
		*sg.options,
	)

	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) OCR2CreateProposal(version int) (string, error) {
	_, err := sg.G.ExecCommand([]string{
		"ocr2:create_proposal",
		fmt.Sprintf("--version=%d", version),
	},
		*sg.options,
	)

	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return *sg.gr.Data.Proposal, nil
}

func (sg *SolanaGauntlet) ProposeOnChainConfig(proposalId string, onChainConfig ocr2_config.OCR2OnChainConfig, ocrFeedAddress string) (string, error) {
	config, err := json.Marshal(onChainConfig)
	if err != nil {
		return "", err
	}
	_, err = sg.G.ExecCommand([]string{
		"ocr2:propose_config",
		fmt.Sprintf("--proposalId=%s", proposalId),
		fmt.Sprintf("--input=%v", string(config)),
		ocrFeedAddress,
	},
		*sg.options,
	)

	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) ProposeOffChainConfig(proposalId string, offChainConfig ocr2_config.OCROffChainConfig, ocrFeedAddress string) (string, error) {
	config, err := json.Marshal(offChainConfig)
	if err != nil {
		return "", err
	}

	_, err = sg.G.ExecCommand([]string{
		"ocr2:propose_offchain_config",
		fmt.Sprintf("--proposalId=%s", proposalId),
		fmt.Sprintf("--input=%v", string(config)),
		ocrFeedAddress,
	},
		*sg.options,
	)

	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) ProposePayees(proposalId string, payeesConfig ocr2_config.PayeeConfig, ocrFeedAddress string) (string, error) {
	config, err := json.Marshal(payeesConfig)
	if err != nil {
		return "", err
	}

	_, err = sg.G.ExecCommand([]string{
		"ocr2:propose_payees",
		fmt.Sprintf("--proposalId=%s", proposalId),
		fmt.Sprintf("--input=%v", string(config)),
		ocrFeedAddress,
	},
		*sg.options,
	)

	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) FinalizeProposal(proposalId string) (string, error) {
	_, err := sg.G.ExecCommand([]string{
		"ocr2:finalize_proposal",
		fmt.Sprintf("--proposalId=%s", proposalId),
	},
		*sg.options,
	)

	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) AcceptProposal(proposalId string, secret string, proposalAcceptConfig ocr2_config.ProposalAcceptConfig, ocrFeedAddres string) (string, error) {
	config, err := json.Marshal(proposalAcceptConfig)
	if err != nil {
		return "", err
	}

	_, err = sg.G.ExecCommand([]string{
		"ocr2:accept_proposal",
		fmt.Sprintf("--proposalId=%s", proposalId),
		fmt.Sprintf("--secret=%s", secret),
		fmt.Sprintf("--input=%s", string(config)),
		ocrFeedAddres,
	},
		*sg.options,
	)

	if err != nil {
		return "", err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return "", err
	}

	return sg.gr.Responses[0].Contract, nil
}

// FetchTransmissions returns the last 10 transmissions
func (sg *SolanaGauntlet) FetchTransmissions(ocrState string) ([]Transmission, error) {
	_, err := sg.G.ExecCommand([]string{
		"ocr2:inspect:responses",
		ocrState,
	},
		*sg.options,
	)

	if err != nil {
		return nil, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return nil, err
	}

	return *sg.gr.Data.LatestTransmissions, nil
}

func (sg *SolanaGauntlet) DeployOCR2() (string, error) {
	var err error
	err = sg.InstallDependencies()
	if err != nil {
		return "", err
	}

	sg.AccessControllerAddress, err = sg.InitializeAccessController()
	if err != nil {
		return "", err
	}

	sg.BillingControllerAddress, err = sg.InitializeAccessController()
	if err != nil {
		return "", err
	}

	sg.StoreAddress, err = sg.InitializeStore(sg.BillingControllerAddress)
	if err != nil {
		return "", err
	}
	storeConfig := &ocr2_config.StoreFeedConfig{
		Store:       sg.StoreAddress,
		Granularity: 1,
		LiveLength:  10,
		Decimals:    8,
		Description: "Test feed",
	}

	sg.FeedAddress, err = sg.StoreCreateFeed(10, storeConfig)
	if err != nil {
		return "", err
	}

	_, err = sg.StoreSetValidatorConfig(sg.FeedAddress, 8000)
	if err != nil {
		return "", err
	}

	ocr2Config := &ocr2_config.OCR2TransmitConfig{
		MinAnswer:     "0",
		MaxAnswer:     "10000000000",
		Transmissions: sg.FeedAddress,
	}

	sg.OcrAddress, err = sg.InitializeOCR2(sg.AccessControllerAddress, sg.BillingControllerAddress, ocr2Config)
	if err != nil {
		return "", err
	}

	storeWriter := &ocr2_config.StoreWriterConfig{Transmissions: sg.FeedAddress}

	_, err = sg.StoreSetWriter(storeWriter, sg.OcrAddress)
	if err != nil {
		return "", err
	}

	ocr2BillingConfig := &ocr2_config.OCR2BillingConfig{
		ObservationPaymentGjuels:  1,
		TransmissionPaymentGjuels: 1,
	}

	_, err = sg.OCR2SetBilling(ocr2BillingConfig, sg.OcrAddress)
	if err != nil {
		return "", err
	}

	sg.ProposalAddress, err = sg.OCR2CreateProposal(2)
	if err != nil {
		return "", err
	}
	sg.OCR2Config.OnChainConfig.ProposalId = sg.ProposalAddress
	sg.OCR2Config.OffChainConfig.ProposalId = sg.ProposalAddress
	sg.OCR2Config.PayeeConfig.ProposalId = sg.ProposalAddress
	sg.OCR2Config.ProposalAcceptConfig.ProposalId = sg.ProposalAddress

	return "", nil
}
func (sg *SolanaGauntlet) ConfigureOCR2() error {
	_, err := sg.ProposeOnChainConfig(sg.ProposalAddress, *sg.OCR2Config.OnChainConfig, sg.OcrAddress)
	if err != nil {
		return err
	}
	_, err = sg.ProposeOffChainConfig(sg.ProposalAddress, *sg.OCR2Config.OffChainConfig, sg.OcrAddress)
	if err != nil {
		return err
	}
	_, err = sg.ProposePayees(sg.ProposalAddress, *sg.OCR2Config.PayeeConfig, sg.OcrAddress)
	if err != nil {
		return err
	}
	_, err = sg.FinalizeProposal(sg.ProposalAddress)
	if err != nil {
		return err
	}
	_, err = sg.AcceptProposal(sg.ProposalAddress, sg.OCR2Config.OffChainConfig.UserSecret, *sg.OCR2Config.ProposalAcceptConfig, sg.OcrAddress)
	if err != nil {
		return err
	}
	return nil
}
