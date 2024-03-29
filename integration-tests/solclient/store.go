package solclient

import (
	"context"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/smartcontractkit/chainlink-solana/contracts/generated/store"
	relaySol "github.com/smartcontractkit/chainlink-solana/pkg/solana"
)

type Store struct {
	Client        *Client
	Store         solana.PublicKey
	Feed          solana.PublicKey
	Owner         *solana.Wallet
	ProgramWallet solana.PublicKey
}

func (m *Store) GetLatestRoundData() (uint64, uint64, uint64, error) {
	a, _, err := relaySol.GetLatestTransmission(context.Background(), m.Client.RPC, m.Feed, rpc.CommitmentConfirmed)
	if err != nil {
		return 0, 0, 0, err
	}
	return a.Data.Uint64(), uint64(a.Timestamp), 0, nil
}

func (m *Store) TransmissionsAddress() string {
	return m.Feed.String()
}

func (m *Store) SetValidatorConfig(flaggingThreshold uint32) error {
	payer := m.Client.DefaultWallet
	err := m.Client.TXAsync(
		"Set validator config",
		[]solana.Instruction{
			store.NewSetValidatorConfigInstruction(
				flaggingThreshold,
				m.Feed,
				m.Owner.PublicKey(),
				m.Owner.PublicKey(),
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(m.Owner.PublicKey()) {
				return &m.Owner.PrivateKey
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
				m.Feed,
				m.Owner.PublicKey(),
				m.Owner.PublicKey(),
			).Build(),
		},
		func(key solana.PublicKey) *solana.PrivateKey {
			if key.Equals(m.Owner.PublicKey()) {
				return &m.Owner.PrivateKey
			}
			// TODO: is this needed?
			// if key.Equals(m.Feed)) {
			// 	return &m.Feed.PrivateKey
			// }
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
	return m.ProgramWallet.String()
}

func (m *Store) Address() string {
	return m.Store.String()
}
