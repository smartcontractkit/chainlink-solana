package solclient

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rs/zerolog/log"
	ocr_2 "github.com/smartcontractkit/chainlink-solana/contracts/generated/ocr2"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"
	"github.com/smartcontractkit/integrations-framework/contracts"
	"github.com/smartcontractkit/libocr/offchainreporting2/confighelper"
)

type OCRv2 struct {
	Client        *Client
	State         *solana.Wallet
	Authorities   map[string]*Authority
	Payees        []*solana.Wallet
	ProgramWallet *solana.Wallet
}

func (m *OCRv2) ProgramAddress() string {
	return m.ProgramWallet.PublicKey().String()
}

func (m *OCRv2) writeOffChainConfig(ocConfigBytes []byte) error {
	payer := m.Client.DefaultWallet
	return m.Client.TXSync(
		"Write OffChain config chunk",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			ocr_2.NewWriteOffchainConfigInstruction(
				ocConfigBytes,
				m.Client.Accounts.Proposal.PublicKey(),
				m.Client.Accounts.Owner.PublicKey(),
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(m.Client.Accounts.Owner.PublicKey()) {
				return &m.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			if key.Equals(m.Client.Accounts.Proposal.PublicKey()) {
				return &m.Client.Accounts.Proposal.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
}

func (m *OCRv2) acceptProposal(digest []byte) error {
	payer := m.Client.DefaultWallet
	vaultAuth, err := m.AuthorityAddr("vault")
	if err != nil {
		return err
	}
	va, err := solana.PublicKeyFromBase58(vaultAuth)
	if err != nil {
		return nil
	}
	return m.Client.TXSync(
		"Accept OffChain config proposal",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			ocr_2.NewAcceptProposalInstruction(
				digest,
				m.Client.Accounts.OCR.PublicKey(),
				m.Client.Accounts.Proposal.PublicKey(),
				m.Client.Accounts.Owner.PublicKey(),
				m.Client.Accounts.Owner.PublicKey(),
				m.Client.Accounts.OCRVaultAssociatedPubKey,
				va,
				solana.TokenProgramID,
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(m.Client.Accounts.Owner.PublicKey()) {
				return &m.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			if key.Equals(m.Client.Accounts.Proposal.PublicKey()) {
				return &m.Client.Accounts.Proposal.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
}

func (m *OCRv2) finalizeOffChainConfig() error {
	payer := m.Client.DefaultWallet
	return m.Client.TXSync(
		"Finalize OffChain config",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			ocr_2.NewFinalizeProposalInstruction(
				m.Client.Accounts.Proposal.PublicKey(),
				m.Client.Accounts.Owner.PublicKey(),
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(m.Client.Accounts.Owner.PublicKey()) {
				return &m.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			if key.Equals(m.Client.Accounts.Proposal.PublicKey()) {
				return &m.Client.Accounts.Proposal.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
}

func (m *OCRv2) makeDigest() ([]byte, error) {
	proposal, err := m.fetchProposalAccount()
	if err != nil {
		return nil, err
	}
	hasher := sha256.New()
	hasher.Write(append([]byte{}, uint8(proposal.Oracles.Len)))
	for _, oracle := range proposal.Oracles.Xs[:proposal.Oracles.Len] {
		hasher.Write(oracle.Signer.Key[:])
		hasher.Write(oracle.Transmitter.Bytes())
		hasher.Write(oracle.Payee.Bytes())
	}

	hasher.Write(append([]byte{}, proposal.F))
	hasher.Write(proposal.TokenMint.Bytes())
	header := make([]byte, 8+4)
	binary.BigEndian.PutUint64(header, proposal.OffchainConfig.Version)
	binary.BigEndian.PutUint32(header[8:], uint32(proposal.OffchainConfig.Len))
	hasher.Write(header)
	hasher.Write(proposal.OffchainConfig.Xs[:proposal.OffchainConfig.Len])
	return hasher.Sum(nil), nil
}

func (m *OCRv2) fetchProposalAccount() (*ocr_2.Proposal, error) {
	var proposal ocr_2.Proposal
	err := m.Client.RPC.GetAccountDataInto(
		context.Background(),
		m.Client.Accounts.Proposal.PublicKey(),
		&proposal,
	)
	if err != nil {
		return nil, err
	}
	log.Debug().Interface("Proposal", proposal).Msg("OCR2 Proposal dump")
	return &proposal, nil
}

func (m *OCRv2) createProposal(version uint64) error {
	payer := m.Client.DefaultWallet
	programWallet := m.Client.ProgramWallets["ocr2-keypair.json"]
	proposalAccInstruction, err := m.Client.CreateAccInstr(m.Client.Accounts.Proposal, OCRProposalAccountSize, programWallet.PublicKey())
	if err != nil {
		return err
	}
	return m.Client.TXSync(
		"Create proposal",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			proposalAccInstruction,
			ocr_2.NewCreateProposalInstruction(
				version,
				m.Client.Accounts.Proposal.PublicKey(),
				m.Client.Accounts.Owner.PublicKey(),
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(m.Client.Accounts.Owner.PublicKey()) {
				return &m.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(m.Client.Accounts.Proposal.PublicKey()) {
				return &m.Client.Accounts.Proposal.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
}

// Configure sets offchain config in multiple transactions
func (m *OCRv2) Configure(cfg contracts.OffChainAggregatorV2Config) error {
	_, _, _, _, version, cfgBytes, err := confighelper.ContractSetConfigArgsForTests(
		cfg.DeltaProgress,
		cfg.DeltaResend,
		cfg.DeltaRound,
		cfg.DeltaGrace,
		cfg.DeltaStage,
		cfg.RMax,
		cfg.S,
		cfg.Oracles,
		cfg.ReportingPluginConfig,
		cfg.MaxDurationQuery,
		cfg.MaxDurationObservation,
		cfg.MaxDurationReport,
		cfg.MaxDurationShouldAcceptFinalizedReport,
		cfg.MaxDurationShouldTransmitAcceptedReport,
		cfg.F,
		cfg.OnchainConfig,
	)
	if err != nil {
		return err
	}
	chunks := utils.ChunkSlice(cfgBytes, 1000)
	if err = m.createProposal(version); err != nil {
		return err
	}
	if err = m.proposeConfig(cfg); err != nil {
		return err
	}
	for _, cfgChunk := range chunks {
		if err = m.writeOffChainConfig(cfgChunk); err != nil {
			return err
		}
	}
	if err = m.finalizeOffChainConfig(); err != nil {
		return err
	}
	digest, err := m.makeDigest()
	if err != nil {
		return err
	}
	return m.acceptProposal(digest)
}

// DumpState dumps all OCR accounts state
func (m *OCRv2) DumpState() error {
	var stateDump ocr_2.State
	err := m.Client.RPC.GetAccountDataInto(
		context.Background(),
		m.Client.Accounts.OCR.PublicKey(),
		&stateDump,
	)
	if err != nil {
		return err
	}
	log.Debug().Interface("State", stateDump).Msg("OCR2 State dump")
	return nil
}

// SetBilling sets default billing to oracles
func (m *OCRv2) SetBilling(observationPayment uint32, transmissionPayment uint32, controllerAddr string) error {
	payer := m.Client.DefaultWallet
	billingACPubKey, err := solana.PublicKeyFromBase58(controllerAddr)
	if err != nil {
		return nil
	}
	err = m.Client.TXAsync(
		"Set billing",
		[]solana.Instruction{
			ocr_2.NewSetBillingInstruction(
				observationPayment,
				transmissionPayment,
				m.Client.Accounts.OCR.PublicKey(),
				m.Client.Accounts.Owner.PublicKey(),
				billingACPubKey,
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(m.Client.Accounts.Owner.PublicKey()) {
				return &m.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (m *OCRv2) GetContractData(ctx context.Context) (*contracts.OffchainAggregatorData, error) {
	panic("implement me")
}

// ProposeConfig sets oracles with payee addresses
func (m *OCRv2) proposeConfig(ocConfig contracts.OffChainAggregatorV2Config) error {
	log.Info().Str("Program Address", m.ProgramWallet.PublicKey().String()).Msg("Proposing new config")
	payer := m.Client.DefaultWallet
	oracles := make([]ocr_2.NewOracle, 0)
	for _, oc := range ocConfig.Oracles {
		oracle := oc.OracleIdentity
		var keyArr [20]byte
		copy(keyArr[:], oracle.OnchainPublicKey)
		transmitter, err := solana.PublicKeyFromBase58(string(oracle.TransmitAccount))
		if err != nil {
			return err
		}
		oracles = append(oracles, ocr_2.NewOracle{
			Signer:      keyArr,
			Transmitter: transmitter,
		})
	}
	err := m.Client.TXSync(
		"Propose new config",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			ocr_2.NewProposeConfigInstruction(
				oracles,
				uint8(ocConfig.F),
				m.Client.Accounts.Proposal.PublicKey(),
				m.Client.Accounts.Owner.PublicKey(),
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(m.Client.Accounts.Owner.PublicKey()) {
				return &m.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(m.Client.Accounts.Proposal.PublicKey()) {
				return &m.Client.Accounts.Proposal.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
	if err != nil {
		return err
	}
	// set one payee for all
	instr := make([]solana.Instruction, 0)
	payee := solana.NewWallet()
	if err := m.Client.addNewAssociatedAccInstr(payee, m.Client.Accounts.Owner.PublicKey(), &instr); err != nil {
		return err
	}
	payees := make([]solana.PublicKey, 0)
	for i := 0; i < len(oracles); i++ {
		payees = append(payees, payee.PublicKey())
	}
	instr = append(instr, ocr_2.NewProposePayeesInstruction(
		m.Client.Accounts.Mint.PublicKey(),
		payees,
		m.Client.Accounts.Proposal.PublicKey(),
		m.Client.Accounts.Owner.PublicKey()).Build(),
	)
	return m.Client.TXSync(
		"Set payees",
		rpc.CommitmentFinalized,
		instr,
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(payee.PublicKey()) {
				return &payee.PrivateKey
			}
			if key.Equals(m.Client.Accounts.Owner.PublicKey()) {
				return &m.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(payer.PublicKey()) {
				return &payer.PrivateKey
			}
			return nil
		},
		payer.PublicKey(),
	)
}

func (m *OCRv2) RequestNewRound() error {
	panic("implement me")
}

func (m *OCRv2) AuthorityAddr(s string) (string, error) {
	auth, ok := m.Authorities[s]
	if !ok {
		return "", fmt.Errorf("authority with seed %s not found", s)
	}
	return auth.PublicKey.String(), nil
}

func (m *OCRv2) Address() string {
	return m.State.PublicKey().String()
}

func (m *OCRv2) TransferOwnership(to string) error {
	panic("implement me")
}

func (m *OCRv2) GetLatestConfigDetails() (map[string]interface{}, error) {
	panic("implement me")
}

func (m *OCRv2) GetOwedPayment(transmitterAddr string) (map[string]interface{}, error) {
	panic("implement me")
}
