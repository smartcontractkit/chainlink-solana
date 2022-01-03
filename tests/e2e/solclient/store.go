package solclient

import (
	"context"
	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/chainlink-solana/contracts/generated/store"
	relaySol "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

type Store struct {
	Client        *Client
	State         *solana.Wallet
	Transmissions *solana.Wallet
	ProgramWallet *solana.Wallet
}

func (m *Store) GetLatestRoundData() (uint64, error) {
	a, _, err := relaySol.GetLatestTransmission(context.Background(), m.Client.RPC, m.Transmissions.PublicKey())
	if err != nil {
		return 0, err
	}
	return a.Data.Uint64(), nil
}

func (m *Store) TransmissionsAddress() string {
	return m.Transmissions.PublicKey().String()
}

func (m *Store) SetValidatorConfig(flaggingThreshold uint32) error {
	panic("implement me")
}

func (m *Store) SetWriter(writerAuthority string) error {
	payer := m.Client.DefaultWallet
	writerAuthPubKey, err := solana.PublicKeyFromBase58(writerAuthority)
	if err != nil {
		return nil
	}
	err = m.Client.TXAsync(
		"Set writer",
		[]solana.Instruction{
			store.NewSetWriterInstruction(
				writerAuthPubKey,
				m.State.PublicKey(),
				m.Client.Accounts.Owner.PublicKey(),
				m.Transmissions.PublicKey(),
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

func (m *Store) CreateFeed(granularity int, liveLength int) error {
	payer := m.Client.DefaultWallet
	programWallet := m.Client.ProgramWallets["store-keypair.json"]
	ocrTransmissionsAccInstruction, err := m.Client.CreateAccInstr(m.Client.Accounts.Transmissions, OCRTransmissionsAccountSize, programWallet.PublicKey())
	if err != nil {
		return err
	}
	err = m.Client.TXAsync(
		"Create feed",
		[]solana.Instruction{
			ocrTransmissionsAccInstruction,
			store.NewCreateFeedInstruction(
				uint8(granularity),
				uint32(liveLength),
				m.State.PublicKey(),
				m.Transmissions.PublicKey(),
				m.Client.Accounts.Owner.PublicKey(),
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(m.Client.Accounts.Owner.PublicKey()) {
				return &m.Client.Accounts.Owner.PrivateKey
			}
			if key.Equals(m.Transmissions.PublicKey()) {
				return &m.Transmissions.PrivateKey
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

func (m *Store) ProgramAddress() string {
	return m.ProgramWallet.PublicKey().String()
}

func (m *Store) Address() string {
	return m.State.PublicKey().String()
}
