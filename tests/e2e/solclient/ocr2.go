package solclient

import (
	"context"
	"fmt"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/rs/zerolog/log"
	"github.com/smartcontractkit/chainlink-solana/contracts/generated/ocr2"
	"github.com/smartcontractkit/chainlink-solana/tests/e2e/utils"
	"github.com/smartcontractkit/integrations-framework/contracts"
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
	err := m.Client.TXSync(
		"Write OffChain config chunk",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			ocr_2.NewWriteOffchainConfigInstruction(
				ocConfigBytes,
				m.State.PublicKey(),
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
			return nil
		},
		payer.PublicKey(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (m *OCRv2) commitOffChainConfig() error {
	payer := m.Client.DefaultWallet
	err := m.Client.TXSync(
		"Commit OffChain config",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			ocr_2.NewCommitOffchainConfigInstruction(
				m.State.PublicKey(),
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
			return nil
		},
		payer.PublicKey(),
	)
	if err != nil {
		return err
	}
	return nil
}

func (m *OCRv2) beginOffChainConfig(version uint64) error {
	payer := m.Client.DefaultWallet
	err := m.Client.TXSync(
		"Begin OffChain config",
		rpc.CommitmentFinalized,
		[]solana.Instruction{
			ocr_2.NewBeginOffchainConfigInstruction(
				version,
				m.State.PublicKey(),
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
			return nil
		},
		payer.PublicKey(),
	)
	if err != nil {
		return err
	}
	return nil
}

// SetOffChainConfig sets offchain config in multiple transactions
func (m *OCRv2) SetOffChainConfig(ocParams contracts.OffChainAggregatorV2Config) error {
	version, cfgChunks, err := utils.NewOCR2ConfigChunks(ocParams)
	if err != nil {
		return err
	}
	if err := m.beginOffChainConfig(version); err != nil {
		return err
	}
	for _, cfgChunk := range cfgChunks {
		if err := m.writeOffChainConfig(cfgChunk); err != nil {
			return err
		}
	}
	if err := m.commitOffChainConfig(); err != nil {
		return err
	}
	return nil
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

// // SetValidatorConfig sets validator config
// func (m *OCRv2) SetValidatorConfig(flaggingThreshold uint32, validatorAddr string) error {
// 	payer := m.Client.DefaultWallet
// 	validatorPubKey, err := solana.PublicKeyFromBase58(validatorAddr)
// 	if err != nil {
// 		return err
// 	}
// 	err = m.Client.TXAsync(
// 		"Set validator config",
// 		[]solana.Instruction{
// 			ocr_2.NewSetValidatorConfigInstruction(
// 				flaggingThreshold,
// 				m.Client.Accounts.OCR.PublicKey(),
// 				m.Client.Accounts.Owner.PublicKey(),
// 				validatorPubKey,
// 			).Build(),
// 		},
// 		func(key solana.PublicKey) *solana.PrivateKey {
// 			if key.Equals(m.Client.Accounts.Owner.PublicKey()) {
// 				return &m.Client.Accounts.Owner.PrivateKey
// 			}
// 			if key.Equals(payer.PublicKey()) {
// 				return &payer.PrivateKey
// 			}
// 			return nil
// 		},
// 		payer.PublicKey(),
// 	)
// 	if err != nil {
// 		return err
// 	}
// 	return nil
// }

// TODO: doesn't exist anymore
func (m *OCRv2) SetValidatorConfig(flaggingThreshold uint32, validatorAddr string) error {
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

// SetOracles sets oracles with payee addresses
func (m *OCRv2) SetOracles(ocConfig contracts.OffChainAggregatorV2Config) error {
	log.Info().Str("Program Address", m.ProgramWallet.PublicKey().String()).Msg("Setting oracles")
	payer := m.Client.DefaultWallet
	instr := make([]solana.Instruction, 0)

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
	// set one payee for all
	payee := solana.NewWallet()
	if err := m.Client.addNewAssociatedAccInstr(payee, m.Client.Accounts.Owner.PublicKey(), &instr); err != nil {
		return err
	}
	payees := make([]solana.PublicKey, 0)
	for i := 0; i < len(oracles); i++ {
		payees = append(payees, payee.PublicKey())
	}
	instr = append(instr, ocr_2.NewSetConfigInstruction(
		oracles,
		uint8(ocConfig.F),
		m.Client.Accounts.OCR.PublicKey(),
		m.Client.Accounts.Owner.PublicKey(),
	).Build())
	instr = append(instr, ocr_2.NewSetPayeesInstruction(
		payees,
		m.State.PublicKey(),
		m.Client.Accounts.Owner.PublicKey()).Build(),
	)
	err := m.Client.TXAsync(
		"Set oracles with associated payees",
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
	if err != nil {
		return err
	}
	return nil
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
