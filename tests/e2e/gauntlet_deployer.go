package e2e

import (
	"encoding/json"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"

	. "github.com/onsi/gomega"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"
	"github.com/smartcontractkit/integrations-framework/gauntlet"
)

const SOLANA_COMMAND_ERROR = "Solana Command execution error"
const RETRY_COUNT = 5

type GauntletDeployer struct {
	Cli                        *gauntlet.Gauntlet
	Version                    string
	LinkToken                  string
	BillingAccessController    string
	RequesterAccessController  string
	Flags                      string
	DeviationFlaggingValidator string
	OCR                        string
	RddPath                    string
	ProposalId                 string
	ProposalDigest             string
	OffchainProposalSecret     string
}

type GauntletReport struct {
	Responses []struct {
		Tx struct {
			Hash    string `json:"hash"`
			Address string `json:"address"`
		}
		Contract string `json:"contract"`
	} `json:"responses"`
	Data map[string]interface{} `json:"data"`
}

type TokenReadData struct {
	MintAuthorityOption string `json:"mintAuthorityOption"`
	MintAuthority       struct {
		Bn string `json:"_bn"`
	} `json:"mintAuthority"`
	Supply                string `json:"supply"`
	Decimals              string `json:"decimals"`
	IsInitialized         string `json:"isInitialized"`
	FreezeAuthorityOption string `json:"freezeAuthorityOption"`
	FreezeAuthority       struct {
		Bn string `json:"_bn"`
	} `json:"freezeAuthority"`
}

// GetDefaultGauntletConfig gets the default config gauntlet will need to start making commands
// 	against the environment
func GetDefaultGauntletConfig(nodeUrl *url.URL) map[string]string {
	networkConfig := map[string]string{
		"NETWORK":                      "local",
		"NODE_URL":                     nodeUrl.String(),
		"PROGRAM_ID_OCR2":              "CF13pnKGJ1WJZeEgVAtFdUi4MMndXm9hneiHs8azUaZt",
		"PROGRAM_ID_ACCESS_CONTROLLER": "2F5NEkMnCRkmahEAcQfTQcZv1xtGgrWFfjENtTwHLuKg",
		"PROGRAM_ID_STORE":             "A7Jh2nb1hZHwqEofm4N8SXbKTj82rx7KUfjParQXUyMQ",
		"PRIVATE_KEY":                  "[82,252,248,116,175,84,117,250,95,209,157,226,79,186,119,203,91,102,11,93,237,3,147,113,49,205,35,71,74,208,225,183,24,204,237,135,197,153,100,220,237,111,190,58,211,186,148,129,219,173,188,168,137,129,84,192,188,250,111,167,151,43,111,109]",
		"SECRET":                       "[only,unfair,fiction,favorite,sudden,strategy,rotate,announce,rebuild,keep,violin,nuclear]",
	}

	return networkConfig
}

// UpdateReportName updates the report name to be used by gauntlet on completion
func UpdateReportName(reportName string, g *gauntlet.Gauntlet) {
	g.NetworkConfig["REPORT_NAME"] = filepath.Join(utils.Reports, reportName)
	err := g.WriteNetworkConfigMap(utils.Networks)
	Expect(err).ShouldNot(HaveOccurred(), "Failed to write the updated .env file")
}

// LoadReportJson loads a gauntlet report into a generic map
func LoadReportJson(file string) (map[string]interface{}, error) {
	jsonFile, err := os.Open(filepath.Join(utils.Reports, file))
	if err != nil {
		return map[string]interface{}{}, err
	}
	defer jsonFile.Close()

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return map[string]interface{}{}, err
	}

	var data map[string]interface{}
	err = json.Unmarshal([]byte(byteValue), &data)

	return data, err
}

func LoadReport(file string) (GauntletReport, error) {
	jsonFile, err := os.Open(filepath.Join(utils.Reports, file))
	if err != nil {
		return GauntletReport{}, err
	}
	defer jsonFile.Close()

	var report GauntletReport
	byteValue, _ := ioutil.ReadAll(jsonFile)
	json.Unmarshal(byteValue, &report)

	return report, nil
}

// GetTxAddressFromReport gets the address from the typical place in the json report data
func GetTxAddressFromReport(report map[string]interface{}) string {
	return report["responses"].([]interface{})[0].(map[string]interface{})["tx"].(map[string]interface{})["address"].(string)
}

// DeployToken deploys the link token
func (gd *GauntletDeployer) DeployToken() string {
	reportName := "deploy_token"
	UpdateReportName(reportName, gd.Cli)
	_, err := gd.Cli.ExecCommandWithRetries(
		[]string{
			"token:deploy",
		},
		gauntlet.ExecCommandOptions{
			ErrHandling: []string{
				SOLANA_COMMAND_ERROR,
			},
			RetryCount:        RETRY_COUNT,
			CheckErrorsInRead: true,
		})
	Expect(err).ShouldNot(HaveOccurred(), "Failed to deploy link token")
	report, err := LoadReportJson(reportName + ".json")
	Expect(err).ShouldNot(HaveOccurred())
	return GetTxAddressFromReport(report)
}

// DeployToken deploys the link token
func (gd *GauntletDeployer) TokenReadState(linkAddress string) (GauntletReport, error) {
	reportName := "token_read_state"
	UpdateReportName(reportName, gd.Cli)
	_, err := gd.Cli.ExecCommandWithRetries([]string{
		"token:read_state",
		linkAddress,
	}, gauntlet.ExecCommandOptions{
		ErrHandling: []string{
			SOLANA_COMMAND_ERROR,
		},
		RetryCount:        RETRY_COUNT,
		CheckErrorsInRead: true,
	})
	Expect(err).ShouldNot(HaveOccurred(), "Failed to read link token")
	report, err := LoadReport(reportName + ".json")
	Expect(err).ShouldNot(HaveOccurred())
	return report, err
}

// DeployToken deploys the link token
func (gd *GauntletDeployer) AccessControllerInitialize(linkAddress string) (GauntletReport, error) {
	reportName := "access_controller_initialize"
	UpdateReportName(reportName, gd.Cli)
	_, err := gd.Cli.ExecCommandWithRetries([]string{
		"access_controller:initialize",
		linkAddress,
	}, gauntlet.ExecCommandOptions{
		ErrHandling: []string{
			SOLANA_COMMAND_ERROR,
		},
		RetryCount:        RETRY_COUNT,
		CheckErrorsInRead: true,
	})
	Expect(err).ShouldNot(HaveOccurred(), "Failed to initialize access controller")
	report, err := LoadReport(reportName + ".json")
	Expect(err).ShouldNot(HaveOccurred())
	return report, err
}
