package solclient

import (
	"fmt"
	"math/big"
	"path/filepath"
	"strings"

	ag_binary "github.com/gagliardetto/binary"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/token"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rs/zerolog/log"
	access_controller2 "github.com/smartcontractkit/chainlink-solana/contracts/generated/access_controller"
	ocr_2 "github.com/smartcontractkit/chainlink-solana/contracts/generated/ocr2"
	store2 "github.com/smartcontractkit/chainlink-solana/contracts/generated/store"
	utils2 "github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"
	"github.com/smartcontractkit/helmenv/environment"
	"github.com/smartcontractkit/integrations-framework/client"
	"github.com/smartcontractkit/integrations-framework/contracts"
	"golang.org/x/sync/errgroup"
)

// All account sizes are calculated from Rust structures, ex. programs/access-controller/src/lib.rs:L80
// there is some wrapper in "anchor" that creates accounts for programs automatically, but we are doing that explicitly
const (
	Discriminator = 8
	// TokenMintAccountSize default size of data required for a new mint account
	TokenMintAccountSize             = uint64(82)
	TokenAccountSize                 = uint64(165)
	AccessControllerStateAccountSize = uint64(Discriminator + solana.PublicKeyLength + solana.PublicKeyLength + 8 + 32*64)
	StoreAccountSize                 = uint64(Discriminator + solana.PublicKeyLength*3)
	OCRTransmissionsAccountSize      = uint64(Discriminator + 192 + 8192*48)
	OCRProposalAccountSize           = Discriminator + 1 + 32 + 1 + 1 + (1 + 4) + 32 + ProposedOraclesSize + OCROffChainConfigSize
	ProposedOracleSize               = uint64(solana.PublicKeyLength + 20 + 4 + solana.PublicKeyLength)
	ProposedOraclesSize              = ProposedOracleSize*19 + 8
	OCROracle                        = uint64(solana.PublicKeyLength + 20 + solana.PublicKeyLength + solana.PublicKeyLength + 4 + 8)
	OCROraclesSize                   = OCROracle*19 + 8
	OCROffChainConfigSize            = uint64(8 + 4096 + 8)
	OCRConfigSize                    = 32 + 32 + 32 + 32 + 32 + 32 + 16 + 16 + (1 + 1 + 2 + 4 + 4 + 32) + (4 + 32 + 8) + (4 + 4)
	OCRAccountSize                   = Discriminator + 1 + 1 + 2 + 4 + solana.PublicKeyLength + OCRConfigSize + OCROffChainConfigSize + OCROraclesSize
)

type Authority struct {
	PublicKey solana.PublicKey
	Nonce     uint8
}

type ContractDeployer struct {
	Client *Client
	Env    *environment.Environment
}

func (c *ContractDeployer) DeployOCRv2Store(billingAC string) (*Store, error) {
	programWallet := c.Client.ProgramWallets["store-keypair.json"]
	payer := c.Client.DefaultWallet
	accInstruction, err := c.Client.CreateAccInstr(c.Client.Accounts.Store, StoreAccountSize, programWallet.PublicKey())
	if err != nil {
		return nil, err
	}
	bacPublicKey, err := solana.PublicKeyFromBase58(billingAC)
	if err != nil {
		return nil, err
	}
	err = c.Client.TXSync(
		"Deploy store",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			accInstruction,
			store2.NewInitializeInstruction(
				c.Client.Accounts.Store.PublicKey(),
				c.Client.Accounts.Owner.PublicKey(),
				bacPublicKey,
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(c.Client.Accounts.Owner.PublicKey()) {
				return &c.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(c.Client.Accounts.Store.PublicKey()) {
				return &c.Client.Accounts.Store.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
	if err != nil {
		return nil, err
	}
	return &Store{
		Client:        c.Client,
		Store:         c.Client.Accounts.Store,
		Feed:          c.Client.Accounts.Feed,
		ProgramWallet: programWallet,
	}, nil
}

func (c *ContractDeployer) addMintToAccInstr(instr *[]solana.Instruction, dest *solana.Wallet, amount uint64) error {
	*instr = append(*instr, token.NewMintToInstruction(
		amount,
		c.Client.Accounts.Mint.PublicKey(),
		dest.PublicKey(),
		c.Client.Accounts.MintAuthority.PublicKey(),
		nil,
	).Build())
	return nil
}

func (c *ContractDeployer) DeployLinkTokenContract() (*LinkToken, error) {
	var err error
	payer := c.Client.DefaultWallet

	instr := make([]solana.Instruction, 0)
	if err = c.Client.addMintInstr(&instr); err != nil {
		return nil, err
	}
	vaultAuthority := c.Client.Accounts.Authorities["vault"]
	c.Client.Accounts.OCRVaultAssociatedPubKey, _, err = solana.FindAssociatedTokenAddress(vaultAuthority.PublicKey, c.Client.Accounts.Mint.PublicKey())
	if err != nil {
		return nil, err
	}
	if err = c.Client.addNewAssociatedAccInstr(c.Client.Accounts.OCRVault, vaultAuthority.PublicKey, &instr); err != nil {
		return nil, err
	}
	if err = c.addMintToAccInstr(&instr, c.Client.Accounts.OCRVault, 1e18); err != nil {
		return nil, err
	}
	err = c.Client.TXAsync(
		"Createing LINK Token and associated accounts",
		instr,
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(c.Client.Accounts.OCRVault.PublicKey()) {
				return &c.Client.Accounts.OCRVault.PrivateKey
			}
			if key.Equals(c.Client.Accounts.Mint.PublicKey()) {
				return &c.Client.Accounts.Mint.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			if key.Equals(c.Client.Accounts.MintAuthority.PublicKey()) {
				return &c.Client.Accounts.MintAuthority.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
	if err != nil {
		return nil, err
	}
	return &LinkToken{
		Client: c.Client,
		State:  c.Client.Accounts.Mint,
	}, nil
}

func (c *ContractDeployer) DeployOCRv2(billingControllerAddr string, requesterControllerAddr string, linkTokenAddr string) (*OCRv2, error) {
	programWallet := c.Client.ProgramWallets["ocr2-keypair.json"]
	payer := c.Client.DefaultWallet
	ocrAccInstruction, err := c.Client.CreateAccInstr(c.Client.Accounts.OCR, OCRAccountSize, programWallet.PublicKey())
	if err != nil {
		return nil, err
	}
	bacPubKey, err := solana.PublicKeyFromBase58(billingControllerAddr)
	if err != nil {
		return nil, err
	}
	racPubKey, err := solana.PublicKeyFromBase58(requesterControllerAddr)
	if err != nil {
		return nil, err
	}
	linkTokenMintPubKey, err := solana.PublicKeyFromBase58(linkTokenAddr)
	if err != nil {
		return nil, err
	}
	vault := c.Client.Accounts.Authorities["vault"]
	err = c.Client.TXSync(
		"Initializing OCRv2",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			ocrAccInstruction,
			ocr_2.NewInitializeInstructionBuilder().
				SetMinAnswer(ag_binary.Int128{
					Lo: 1,
					Hi: 0,
				}).
				SetMaxAnswer(ag_binary.Int128{
					Lo: 1000000,
					Hi: 0,
				}).
				SetStateAccount(c.Client.Accounts.OCR.PublicKey()).
				SetFeedAccount(c.Client.Accounts.Feed.PublicKey()).
				SetPayerAccount(payer.PublicKey()).
				SetOwnerAccount(c.Client.Accounts.Owner.PublicKey()).
				SetTokenMintAccount(linkTokenMintPubKey).
				SetTokenVaultAccount(c.Client.Accounts.OCRVaultAssociatedPubKey).
				SetVaultAuthorityAccount(vault.PublicKey).
				SetRequesterAccessControllerAccount(racPubKey).
				SetBillingAccessControllerAccount(bacPubKey).
				SetRentAccount(solana.SysVarRentPubkey).
				SetSystemProgramAccount(solana.SystemProgramID).
				SetTokenProgramAccount(solana.TokenProgramID).
				SetAssociatedTokenProgramAccount(solana.SPLAssociatedTokenAccountProgramID).
				Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			if key.Equals(c.Client.Accounts.OCR.PublicKey()) {
				return &c.Client.Accounts.OCR.PrivateKey
			}
			if key.Equals(c.Client.Accounts.Owner.PublicKey()) {
				return &c.Client.Accounts.Owner.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
	if err != nil {
		return nil, err
	}
	return &OCRv2{
		Client:        c.Client,
		State:         c.Client.Accounts.OCR,
		Authorities:   c.Client.Accounts.Authorities,
		ProgramWallet: programWallet,
	}, nil
}

func (c *ContractDeployer) DeployProgramRemote(programName string) error {
	log.Debug().Str("Program", programName).Msg("Deploying program")
	connections := c.Env.Charts.Connections("solana-validator")
	cc, err := connections.Load("sol", "0", "sol-val")
	if err != nil {
		return err
	}
	chart := c.Env.Charts["solana-validator"]

	programPath := filepath.Join("programs", programName)
	programKeyFileName := strings.Replace(programName, ".so", "-keypair.json", -1)
	programKeyFilePath := filepath.Join("programs", programKeyFileName)
	cmd := fmt.Sprintf("solana deploy %s %s", programPath, programKeyFilePath)
	stdOutBytes, stdErrBytes, _ := chart.ExecuteInPod(cc.PodName, "sol-val", strings.Split(cmd, " "))
	log.Debug().Str("STDOUT", string(stdOutBytes)).Str("STDERR", string(stdErrBytes)).Str("CMD", cmd).Send()
	return nil
}

func (c *ContractDeployer) DeployOCRv2AccessController() (*AccessController, error) {
	programWallet := c.Client.ProgramWallets["access_controller-keypair.json"]
	payer := c.Client.DefaultWallet
	stateAcc := solana.NewWallet()
	accInstruction, err := c.Client.CreateAccInstr(stateAcc, AccessControllerStateAccountSize, programWallet.PublicKey())
	if err != nil {
		return nil, err
	}
	err = c.Client.TXAsync(
		"Initializing access controller",
		[]solana.Instruction{
			accInstruction,
			access_controller2.NewInitializeInstruction(
				stateAcc.PublicKey(),
				c.Client.Accounts.Owner.PublicKey(),
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(c.Client.Accounts.Owner.PublicKey()) {
				return &c.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(stateAcc.PublicKey()) {
				return &stateAcc.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
	if err != nil {
		return nil, err
	}
	return &AccessController{
		State:         stateAcc,
		Client:        c.Client,
		ProgramWallet: programWallet,
	}, nil
}

func (c *ContractDeployer) DeployOffChainAggregator(linkAddr string, offchainOptions contracts.OffchainOptions) (contracts.OffchainAggregator, error) {
	panic("implement me")
}

func (c *ContractDeployer) DeployVRFContract() (contracts.VRF, error) {
	panic("implement me")
}

func (c *ContractDeployer) DeployMockETHLINKFeed(answer *big.Int) (contracts.MockETHLINKFeed, error) {
	panic("implement me")
}

func (c *ContractDeployer) DeployMockGasFeed(answer *big.Int) (contracts.MockGasFeed, error) {
	panic("implement me")
}

func (c *ContractDeployer) DeployUpkeepRegistrationRequests(linkAddr string, minLinkJuels *big.Int) (contracts.UpkeepRegistrar, error) {
	panic("implement me")
}

func (c *ContractDeployer) DeployKeeperRegistry(opts *contracts.KeeperRegistryOpts) (contracts.KeeperRegistry, error) {
	panic("implement me")
}

func (c *ContractDeployer) DeployKeeperConsumer(updateInterval *big.Int) (contracts.KeeperConsumer, error) {
	panic("implement me")
}

func (c *ContractDeployer) DeployVRFConsumer(linkAddr string, coordinatorAddr string) (contracts.VRFConsumer, error) {
	panic("implement me")
}

func (c *ContractDeployer) DeployVRFCoordinator(linkAddr string, bhsAddr string) (contracts.VRFCoordinator, error) {
	panic("implement me")
}

func (c *ContractDeployer) DeployBlockhashStore() (contracts.BlockHashStore, error) {
	panic("implement me")
}

func (c *ContractDeployer) registerAnchorPrograms() {
	access_controller2.SetProgramID(c.Client.ProgramWallets["access_controller-keypair.json"].PublicKey())
	store2.SetProgramID(c.Client.ProgramWallets["store-keypair.json"].PublicKey())
	ocr_2.SetProgramID(c.Client.ProgramWallets["ocr2-keypair.json"].PublicKey())
}

func (c *ContractDeployer) deployAnchorProgramsRemote() error {
	contractBinaries, err := c.Client.ListDirFilenamesByExt(utils2.ContractsDir, ".so")
	if err != nil {
		return err
	}
	log.Debug().Interface("Binaries", contractBinaries).Msg("Program binaries")
	keyFiles, err := c.Client.ListDirFilenamesByExt(utils2.ContractsDir, ".json")
	if err != nil {
		return err
	}
	log.Debug().Interface("Files", keyFiles).Msg("Program key files")
	for _, kfn := range keyFiles {
		pk, err := solana.PrivateKeyFromSolanaKeygenFile(filepath.Join(utils2.ContractsDir, kfn))
		if err != nil {
			return err
		}
		w, err := c.Client.LoadWallet(pk.String())
		if err != nil {
			return err
		}
		c.Client.ProgramWallets[kfn] = w
	}
	log.Debug().Interface("Keys", c.Client.ProgramWallets).Msg("Program wallets")
	g := errgroup.Group{}
	for _, bin := range contractBinaries {
		bin := bin
		g.Go(func() error {
			return c.DeployProgramRemote(bin)
		})
	}
	return g.Wait()
}

// generateOCRAuthorities generates authorities so other contracts can access OCR with on-chain calls when signer needed
func (c *Client) generateOCRAuthorities(seeds []string) (map[string]*Authority, error) {
	authorities := make(map[string]*Authority)
	for _, seed := range seeds {
		auth, nonce, err := c.FindAuthorityAddress(seed, c.Accounts.OCR.PublicKey(), c.ProgramWallets["ocr2-keypair.json"].PublicKey())
		if err != nil {
			return nil, err
		}
		authorities[seed] = &Authority{
			PublicKey: auth,
			Nonce:     nonce,
		}
	}
	return authorities, nil
}

func (c *Client) FindAuthorityAddress(seed string, statePubKey solana.PublicKey, progPubKey solana.PublicKey) (solana.PublicKey, uint8, error) {
	log.Debug().
		Str("Seed", seed).
		Str("StatePubKey", statePubKey.String()).
		Str("ProgramPubKey", progPubKey.String()).
		Msg("Trying to find program authority")
	auth, nonce, err := solana.FindProgramAddress([][]byte{[]byte(seed), statePubKey.Bytes()}, progPubKey)
	if err != nil {
		return solana.PublicKey{}, 0, err
	}
	log.Debug().Str("Authority", auth.String()).Uint8("Nonce", nonce).Msg("Found authority addr")
	return auth, nonce, err
}

func NewContractDeployer(client client.BlockchainClient, e *environment.Environment) (*ContractDeployer, error) {
	cd := &ContractDeployer{
		Env:    e,
		Client: client.(*Client),
	}
	if err := cd.deployAnchorProgramsRemote(); err != nil {
		return nil, err
	}
	cd.registerAnchorPrograms()
	authorities, err := cd.Client.generateOCRAuthorities([]string{"vault", "store"})
	if err != nil {
		return nil, err
	}
	cd.Client.Accounts.Authorities = authorities
	cd.Client.Accounts.Owner = cd.Client.DefaultWallet
	return cd, nil
}
