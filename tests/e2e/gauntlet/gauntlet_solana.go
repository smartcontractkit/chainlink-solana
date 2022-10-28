package gauntlet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/gagliardetto/solana-go"
	gauntlet "github.com/smartcontractkit/chainlink-testing-framework/gauntlet"
)

var (
	sg *SolanaGauntlet
)

type SolanaGauntlet struct {
	dir     string
	g       *gauntlet.Gauntlet
	gr      *GauntletResponse
	options *gauntlet.ExecCommandOptions
}

// Default response output for starknet gauntlet commands
type GauntletResponse struct {
	Responses []struct {
		Tx struct {
			Hash    string `json:"hash"`
			Address string `json:"address"`
			Status  string `json:"status"`
			Tx      struct {
				Address         string   `json:"address"`
				Code            string   `json:"code"`
				Result          []string `json:"result"`
				TransactionHash string   `json:"transaction_hash"`
			} `json:"tx"`
		} `json:"tx"`
		Contract solana.PublicKey `json:"contract"`
	} `json:"responses"`
}

// Creates a default gauntlet config
func NewSolanaGauntlet(workingDir string) (*SolanaGauntlet, error) {
	g, err := gauntlet.NewGauntlet()
	g.SetWorkingDir(workingDir)
	if err != nil {
		return nil, err
	}
	sg = &SolanaGauntlet{
		dir: workingDir,
		g:   g,
		gr:  &GauntletResponse{},
		options: &gauntlet.ExecCommandOptions{
			ErrHandling:       []string{},
			CheckErrorsInRead: true,
		},
	}
	return sg, nil
}

// Parse gauntlet json response that is generated after yarn gauntlet command execution
func (sg *SolanaGauntlet) FetchGauntletJsonOutput() (*GauntletResponse, error) {
	var payload = &GauntletResponse{}
	gauntletOutput, err := ioutil.ReadFile(sg.dir + "report.json")
	if err != nil {
		return payload, err
	}
	err = json.Unmarshal(gauntletOutput, &payload)
	if err != nil {
		return payload, err
	}
	return payload, nil
}

// Sets up a new network and sets the NODE_URL for Devnet / Solana RPC
func (sg *SolanaGauntlet) SetupNetwork(addr string) {
	sg.g.AddNetworkConfigVar("NODE_URL", addr)
	sg.g.WriteNetworkConfigMap(sg.dir + "gauntlet/packages/gauntlet-solana-contracts/networks/")
}

func (sg *SolanaGauntlet) DeployAccountContract(salt int64, pubKey string) (solana.PublicKey, error) {
	_, err := sg.g.ExecCommand([]string{"account:deploy", fmt.Sprintf("--salt=%d", salt), fmt.Sprintf("--publicKey=%s", pubKey)}, *sg.options)
	if err != nil {
		return solana.PublicKey{}, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return solana.PublicKey{}, err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) DeployLinkTokenContract() (solana.PublicKey, error) {
	_, err := sg.g.ExecCommand([]string{"ERC20:deploy", "--link"}, *sg.options)
	if err != nil {
		return solana.PublicKey{}, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return solana.PublicKey{}, err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) MintLinkToken(token, to, amount string) (solana.PublicKey, error) {
	_, err := sg.g.ExecCommand([]string{"ERC20:mint", fmt.Sprintf("--account=%s", to), fmt.Sprintf("--amount=%s", amount), token}, *sg.options)
	if err != nil {
		return solana.PublicKey{}, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return solana.PublicKey{}, err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) DeployOCR2ControllerContract(minSubmissionValue int64, maxSubmissionValue int64, decimals int, name string, linkTokenAddress string) (solana.PublicKey, error) {
	_, err := sg.g.ExecCommand([]string{"ocr2:deploy", fmt.Sprintf("--minSubmissionValue=%d", minSubmissionValue), fmt.Sprintf("--maxSubmissionValue=%d", maxSubmissionValue), fmt.Sprintf("--decimals=%d", decimals), fmt.Sprintf("--name=%s", name), fmt.Sprintf("--link=%s", linkTokenAddress)}, *sg.options)
	if err != nil {
		return solana.PublicKey{}, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return solana.PublicKey{}, err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) DeployAccessControllerContract() (solana.PublicKey, error) {
	_, err := sg.g.ExecCommand([]string{"access_controller:initialize"}, *sg.options)
	if err != nil {
		return solana.PublicKey{}, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return solana.PublicKey{}, err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) DeployOCR2ProxyContract(aggregator string) (solana.PublicKey, error) {
	_, err := sg.g.ExecCommand([]string{"proxy:deploy", fmt.Sprintf("--address=%s", aggregator)}, *sg.options)
	if err != nil {
		return solana.PublicKey{}, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return solana.PublicKey{}, err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) SetOCRBilling(observationPaymentGjuels int64, transmissionPaymentGjuels int64, ocrAddress string) (solana.PublicKey, error) {
	_, err := sg.g.ExecCommand([]string{"ocr2:set_billing", fmt.Sprintf("--observationPaymentGjuels=%d", observationPaymentGjuels), fmt.Sprintf("--transmissionPaymentGjuels=%d", transmissionPaymentGjuels), ocrAddress}, *sg.options)
	if err != nil {
		return solana.PublicKey{}, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return solana.PublicKey{}, err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) SetConfigDetails(cfg string, ocrAddress string) (solana.PublicKey, error) {
	_, err := sg.g.ExecCommand([]string{"ocr2:set_config", "--input=" + cfg, ocrAddress}, *sg.options)
	if err != nil {
		return solana.PublicKey{}, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return solana.PublicKey{}, err
	}
	return sg.gr.Responses[0].Contract, nil
}

func (sg *SolanaGauntlet) AddAccess(aggregator, address string) (solana.PublicKey, error) {
	_, err := sg.g.ExecCommand([]string{"ocr2:add_access", fmt.Sprintf("--address=%s", address), aggregator}, *sg.options)
	if err != nil {
		return solana.PublicKey{}, err
	}
	sg.gr, err = sg.FetchGauntletJsonOutput()
	if err != nil {
		return solana.PublicKey{}, err
	}
	return sg.gr.Responses[0].Contract, nil
}
