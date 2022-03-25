package smoke

import (
	"math/big"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rs/zerolog/log"

	"github.com/smartcontractkit/chainlink-solana/tests/e2e"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/common"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/solclient"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/helmenv/tools"
	"github.com/smartcontractkit/integrations-framework/actions"
	"github.com/smartcontractkit/integrations-framework/gauntlet"
)

const CONFIRM_TX_TIMEOUT_SECONDS = "30"
const CONFIRM_MAX_RETRIES = "100"

var _ = Describe("Gauntlet Testing @gauntlet", func() {
	var (
		gd    *e2e.GauntletDeployer
		state *common.OCRv2TestState
	)

	BeforeEach(func() {
		By("Deploying the environment", func() {
			gd = &e2e.GauntletDeployer{
				Version: "local",
			}
			state = &common.OCRv2TestState{}
			state.Env, state.Err = environment.DeployOrLoadEnvironment(
				solclient.NewChainlinkSolOCRv2(1, false),
				tools.ChartsRoot,
			)
			Expect(state.Err).ShouldNot(HaveOccurred())
			err := state.Env.ConnectAll()
			Expect(err).ShouldNot(HaveOccurred())
			state.UploadProgramBinaries(utils.ContractsDir)
		})
		By("Getting the clients", func() {
			state.SetupClients()
			state.OffChainConfig, state.NodeKeysBundle, state.Err = common.DefaultOffChainConfigParamsFromNodes(state.ChainlinkNodes)
			Expect(state.Err).ShouldNot(HaveOccurred())
			state.ContractDeployer, state.Err = solclient.NewContractDeployer(state.Networks.Default, state.Env, utils.ContractsDir)
			Expect(state.Err).ShouldNot(HaveOccurred())
		})
		By("Setup Gauntlet", func() {
			// make the gauntlet solana calls timeout longer for tests, it normally defaults to 60 seconds
			os.Setenv("CONFIRM_TX_TIMEOUT_SECONDS", CONFIRM_TX_TIMEOUT_SECONDS)
			os.Setenv("CONFIRM_TX_COMMITMENT", "confirmed")
			os.Setenv("CONFIRM_MAX_RETRIES", CONFIRM_MAX_RETRIES)

			err := os.Chdir(utils.Gauntlet)
			Expect(err).ShouldNot(HaveOccurred())

			solNodeUrl, err := state.Env.Charts.Connections("solana-validator").LocalURLsByPort("http-rpc", environment.HTTP)
			Expect(err).ShouldNot(HaveOccurred())

			gd.Cli, err = gauntlet.NewGauntlet()
			log.Debug().Str("key", "value").Msg("made it past new gauntlet TATATATATATATATATATATATATATATATATATA")
			Expect(err).ShouldNot(HaveOccurred())
			// log.Debug().Str("key", "value").Msg("made it past new gauntlet TATATATATATATATATATATATATATATATATATA")

			gd.Cli.NetworkConfig = e2e.GetDefaultGauntletConfig(solNodeUrl[0])
			err = gd.Cli.WriteNetworkConfigMap(utils.Networks)
			Expect(err).ShouldNot(HaveOccurred())
		})
		By("Fund Wallets", func() {
			err := common.FundOracles(state.Networks.Default, state.NodeKeysBundle, big.NewFloat(5e4))
			Expect(err).ShouldNot(HaveOccurred())
			err = state.Networks.Default.(*solclient.Client).WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())
		})
	})

	Describe("gauntlet commands", func() {

		It("token", func() {

			// Deploy Link
			// log.Debug().Msg("Deploying LINK Token...")
			// linkAddress := gd.DeployToken()

			lt, err := state.ContractDeployer.DeployLinkTokenContract()
			Expect(err).ShouldNot(HaveOccurred(), "Deploying token failed")
			err = state.Networks.Default.(*solclient.Client).WaitForEvents()
			Expect(err).ShouldNot(HaveOccurred())
			linkAddress := lt.Address()

			readData, err := gd.TokenReadState(linkAddress)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(readData.Data["decimals"]).Should(Equal(float64(18)))

			_, err = gd.AccessControllerInitialize(linkAddress)
			Expect(err).ShouldNot(HaveOccurred())

			// args := []string{
			// 	"token:deploy",
			// 	gd.gauntlet.Flag("network", network),
			// }

			// report, output, err := gd.gauntlet.ExecuteAndRead(args, solanaCommandError, RETRY_COUNT)
			// linkAddress := state.ValidateFailedGauntletCommand(output, report, err)

			// Read the token state
			// log.Debug().Msg("Read the state of the token.")
			// acArgs := []string{
			// 	"token:read_state",
			// 	linkAddress,
			// }

			// _, _, err = gd.gauntlet.ExecuteAndRead(acArgs, solanaCommandError, RETRY_COUNT)
			// Expect(err).ShouldNot(HaveOccurred(), "Reading the token state failed")
			// Expect(strings.Contains(output, "supply: <BN: de0b6b3a7640000>")).To(Equal(true), "We should have the expected supply of tokens in the deployed address")

		})

		// XIt("token2", func() {
		// 	network := "blarg"
		// 	networkConfig, err := utils.GetDefaultGauntletConfig(network, state.Env)
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	// Deploy Link
		// 	log.Debug().Msg("Deploying LINK Token...")
		// 	lt, err := cd.DeployLinkTokenContract()
		// 	Expect(err).ShouldNot(HaveOccurred(), "Deploying token failed")
		// 	err = state.Networks.Default.(*solclient.Client).WaitForEvents()
		// 	Expect(err).ShouldNot(HaveOccurred())
		// 	linkAddress := lt.Address()
		// 	networkConfig["LINK"] = linkAddress
		// 	err = utils.WriteNetworkConfigMap(fmt.Sprintf("networks/.env.%s", network), networkConfig)
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	// Read the token state
		// 	log.Debug().Msg("Read the state of the token.")
		// 	args := []string{
		// 		"token:read_state",
		// 		gd.gauntlet.Flag("network", network),
		// 		linkAddress,
		// 	}

		// 	_, output, err := gd.gauntlet.ExecuteAndRead(args, solanaCommandError, RETRY_COUNT)
		// 	Expect(err).ShouldNot(HaveOccurred(), "Reading the token state failed")
		// 	Expect(strings.Contains(output, "supply: <BN: de0b6b3a7640000>")).To(Equal(true), "We should have the expected supply of tokens in the deployed address")

		// 	// token:transfer
		// 	log.Debug().Msg("Transfer token.")
		// 	args = []string{
		// 		"token:transfer",
		// 		gd.gauntlet.Flag("network", network),
		// 		"--to=7xBSFPrRhXdZW3BmJpa5tydtFngDhapnh8SzihtFKd2U",
		// 		"--amount=100",
		// 		linkAddress,
		// 	}

		// 	_, _, err = gd.gauntlet.ExecuteAndRead(args, solanaCommandError, RETRY_COUNT)
		// 	Expect(err).ShouldNot(HaveOccurred())
		// })

		// It("access_controller", func() {
		// 	network := "blarg"
		// 	networkConfig, err := GetDefaultGauntletConfig(network, e)
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	// Deploy Link
		// 	log.Debug().Msg("Deploying LINK Token...")
		// 	args := []string{
		// 		"token:deploy",
		// 		gd.gauntlet.Flag("network", network),
		// 	}

		// 	errHandling := []g.ExecError{
		// 		solanaCommandError,
		// 	}
		// 	report, err := gd.gauntlet.ExecuteAndRead(args, errHandling)
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	linkAddress := report.Responses[0].Contract
		// 	networkConfig["LINK"] = linkAddress

		// 	// Create Billing and Requester Access Controllers
		// 	log.Debug().Msg("Deploying Access Controller for Requester...")
		// 	acArgs := []string{
		// 		"access_controller:initialize",
		// 		gd.gauntlet.Flag("network", network),
		// 		linkAddress,
		// 	}

		// 	acErrHandling := []g.ExecError{
		// 		solanaCommandError,
		// 	}
		// 	report, err = gd.gauntlet.ExecuteAndRead(acArgs, acErrHandling)
		// 	Expect(err).ShouldNot(HaveOccurred())
		// 	requesterAccessController := report.Responses[0].Contract

		// 	log.Debug().Msg("Deploying Access Controller for Billing...")
		// 	report, err = gd.gauntlet.ExecuteAndRead(acArgs, acErrHandling)
		// 	Expect(err).ShouldNot(HaveOccurred())
		// 	billingAccessController := report.Responses[0].Contract

		// 	networkConfig["REQUESTER_ACCESS_CONTROLLER"] = requesterAccessController
		// 	networkConfig["BILLING_ACCESS_CONTROLLER"] = billingAccessController
		// 	err = WriteNetworkConfigMap(fmt.Sprintf("networks/.env.%s", network), networkConfig)
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	// Create Store
		// 	log.Debug().Msg("Deploying Store...")
		// 	storeArgs := []string{
		// 		"store:initialize",
		// 		gd.gauntlet.Flag("network", network),
		// 	}

		// 	storeErrHandling := []g.ExecError{
		// 		solanaCommandError,
		// 	}
		// 	report, err = gd.gauntlet.ExecuteAndRead(storeArgs, storeErrHandling)
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	storeAccount := report.Responses[0].Contract

		// 	// Create store feed
		// 	input := map[string]interface{}{
		// 		"store":       storeAccount,
		// 		"granularity": 30,
		// 		"liveLength":  1024,
		// 		"decimals":    8,
		// 		"description": "Test LINK/USD",
		// 	}
		// 	jsonInput, err := json.Marshal(input)
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	storeCreateFeedArgs := []string{
		// 		"store:create_feed",
		// 		gd.gauntlet.Flag("network", network),
		// 		gd.gauntlet.Flag("state", storeAccount),
		// 		gd.gauntlet.Flag("input", string(jsonInput)),
		// 	}
		// 	report, err = gd.gauntlet.ExecuteAndRead(storeCreateFeedArgs, storeErrHandling)
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	// log.Debug().Msg("Deploying OCR2...")
		// 	// ocr2Args := []string{
		// 	// 	"ocr2:initialize",
		// 	// 	gd.gauntlet.Flag("network", network),
		// 	// }

		// 	// ocr2ErrHandling := []g.ExecError{
		// 	// 	solanaCommandError,
		// 	// }
		// 	// _, err = gd.gauntlet.ExecCommand(ocr2Args, ocr2ErrHandling)
		// 	// // if we got an error we can check to see if it just didn't finish in 60 seconds by parsing the output or error for the tx signature
		// 	// Expect(err).ShouldNot(HaveOccurred())

		// 	// report, err = gd.gauntlet.ReadCommandReport()
		// 	// Expect(err).ShouldNot(HaveOccurred())

		// 	// access_controller:initialize

		// 	// access_controller:add_access

		// 	// access_controller:read_state
		// })
		// XIt("deploy ocr2", func() {
		// 	network := "deployocr"
		// 	networkConfig, err := utils.GetDefaultGauntletConfig(network, state.Env)
		// 	Expect(err).ShouldNot(HaveOccurred(), "Writing the gauntlet config failed")

		// 	// Deploy Link
		// 	log.Debug().Msg("Deploying LINK Token...")
		// 	lt, err := cd.DeployLinkTokenContract()
		// 	Expect(err).ShouldNot(HaveOccurred(), "Deploying token failed")
		// 	err = state.Networks.Default.(*solclient.Client).WaitForEvents()
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	linkAddress := lt.Address()
		// 	log.Info().Msg(fmt.Sprintf("LINK Address is: %v", linkAddress))

		// 	// ocr2:initialize
		// 	// ocr2:initialize:flow
		// 	// ocr2:set_billing
		// 	// ocr2:pay_remaining
		// 	// ocr2:set_payees
		// 	// ocr2:set_config
		// 	// ocr2:set_validator_config
		// 	// ocr2:read_state
		// 	// ocr2:set_offchain_config:flow
		// 	// ocr2:begin_offchain_config
		// 	// ocr2:write_offchain_config
		// 	// ocr2:commit_offchain_config
		// 	// ocr2:inspect
		// 	// ocr2:transmit
		// 	// ocr2:setup:flow
		// 	// ocr2:setup:rdd:flow
		// 	log.Info().Msg("Deploying OCR Feed")
		// 	log.Info().Msg("Init Requester Access Controller")
		// 	accessControllerArgs := []string{
		// 		"access_controller:initialize",
		// 		gd.gauntlet.Flag("network", network),
		// 	}
		// 	report, output, err := gd.gauntlet.ExecuteAndRead(
		// 		accessControllerArgs,
		// 		solanaCommandError,
		// 		RETRY_COUNT,
		// 	)
		// 	requesterAccessController := state.ValidateFailedGauntletCommand(output, report, err)

		// 	log.Info().Msg("Init Billing Access Controller")
		// 	report, _, err = gd.gauntlet.ExecuteAndRead(
		// 		accessControllerArgs,
		// 		solanaCommandError,
		// 		RETRY_COUNT,
		// 	)
		// 	billingAccessController := state.ValidateFailedGauntletCommand(output, report, err)

		// 	log.Info().Msg("Init Store")
		// 	report, output, err = gd.gauntlet.ExecuteAndRead(
		// 		[]string{"store:initialize",
		// 			gd.gauntlet.Flag("network", network),
		// 			gd.gauntlet.Flag("accessController", billingAccessController)},
		// 		solanaCommandError,
		// 		RETRY_COUNT,
		// 	)
		// 	// Expect(err).ShouldNot(HaveOccurred(), "Store initialization failed")
		// 	storeAccount := state.ValidateFailedGauntletCommand(output, report, err)

		// 	log.Info().Msg("Create Feed in Store")
		// 	input := map[string]interface{}{
		// 		"store":       storeAccount,
		// 		"granularity": 30,
		// 		"liveLength":  1024,
		// 		"decimals":    8,
		// 		"description": "Test LINK/USD",
		// 	}

		// 	jsonInput, err := json.Marshal(input)
		// 	Expect(err).ShouldNot(HaveOccurred(), "Marshaling the stores feed input failed")

		// 	report, _, err = gd.gauntlet.ExecuteAndRead(
		// 		[]string{"store:create_feed",
		// 			gd.gauntlet.Flag("network", network),
		// 			gd.gauntlet.Flag("state", storeAccount), // why is this needed in gauntlet?
		// 			gd.gauntlet.Flag("input", string(jsonInput))},
		// 		solanaCommandError,
		// 		RETRY_COUNT,
		// 	)
		// 	Expect(err).ShouldNot(HaveOccurred(), "Creating a feed for the store failed")
		// 	OCRTransmissions := report.Data["transmissions"]

		// 	log.Info().Msg("Set Validator Config in Store")
		// 	report, _, err = gd.gauntlet.ExecuteAndRead(
		// 		[]string{"store:set_validator_config",
		// 			gd.gauntlet.Flag("network", network),
		// 			gd.gauntlet.Flag("state", storeAccount),
		// 			gd.gauntlet.Flag("feed", OCRTransmissions),
		// 			gd.gauntlet.Flag("threshold", "8000")},
		// 		solanaCommandError,
		// 		RETRY_COUNT,
		// 	)
		// 	Expect(err).ShouldNot(HaveOccurred(), "Setting the stores validator config failed")

		// 	log.Info().Msg("Init OCR 2 Feed")
		// 	input = map[string]interface{}{
		// 		"minAnswer":     "0",
		// 		"maxAnswer":     "10000000000",
		// 		"transmissions": OCRTransmissions,
		// 	}

		// 	jsonInput, err = json.Marshal(input)
		// 	Expect(err).ShouldNot(HaveOccurred(), "Marshalling the ocr2 input failed")

		// 	networkConfig["LINK"] = linkAddress
		// 	networkConfig["REQUESTER_ACCESS_CONTROLLER"] = requesterAccessController
		// 	networkConfig["BILLING_ACCESS_CONTROLLER"] = billingAccessController
		// 	err = utils.WriteNetworkConfigMap(fmt.Sprintf("networks/.env.%s", network), networkConfig)
		// 	Expect(err).ShouldNot(HaveOccurred())

		// 	// TODO: command doesn't throw an error in go if it fails
		// 	// time.Sleep(30 * time.Second) // give time for everything else to complete
		// 	report, _, err = gd.gauntlet.ExecuteAndRead(
		// 		[]string{"ocr2:initialize",
		// 			gd.gauntlet.Flag("network", network),
		// 			gd.gauntlet.Flag("requesterAccessController", requesterAccessController),
		// 			gd.gauntlet.Flag("billingAccessController", billingAccessController),
		// 			gd.gauntlet.Flag("link", linkAddress),
		// 			gd.gauntlet.Flag("input", string(jsonInput))},
		// 		solanaCommandError,
		// 		RETRY_COUNT,
		// 	)
		// 	Expect(err).ShouldNot(HaveOccurred(), "Initializing ocr2 failed")

		// 	// OCRFeed := report.Data["state"]
		// 	// StoreAuthority := report.Data["storeAuthority"]
		// })
		// It("deviation_flagging_validator", func() {
		// 	Expect("abc").To(Equal("abc"))
		// 	// deviation_flagging_validator:initialize
		// })
	})

	AfterEach(func() {
		By("Tearing down the environment", func() {
			err := actions.TeardownSuite(state.Env, nil, "logs", nil)
			Expect(err).ShouldNot(HaveOccurred())
		})
	})
})
