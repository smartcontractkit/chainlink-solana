package solana

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

var _ types.OffchainConfigDigester = (*OffchainConfigDigester)(nil)

type OffchainConfigDigester struct {
	// Solana ID for the OCR2 on-chain program
	ProgramID solana.PublicKey

	// Solana State account address for the OCR2 on-chain program
	StateID solana.PublicKey
}

// ConfigDigest is meant to do the same thing as config_digest_from_data from the program.
func (d OffchainConfigDigester) ConfigDigest(cfg types.ContractConfig) (types.ConfigDigest, error) {
	digest := types.ConfigDigest{}
	buf := sha256.New()

	if _, err := buf.Write(d.ProgramID.Bytes()); err != nil {
		return digest, err
	}

	if _, err := buf.Write(d.StateID.Bytes()); err != nil {
		return digest, err
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(cfg.ConfigCount)); err != nil { //nolint:gosec // max onchain config count is u32
		return digest, err
	}

	if err := binary.Write(buf, binary.BigEndian, uint8(len(cfg.Signers))); err != nil { //nolint:gosec // cannot be negative and protocol does not allow more than 255 signers
		return digest, err
	}

	for _, signer := range cfg.Signers {
		if _, err := buf.Write(signer); err != nil {
			return digest, err
		}
	}

	for _, transmitter := range cfg.Transmitters {
		pubKey, err := solana.PublicKeyFromBase58(string(transmitter))
		if err != nil {
			return digest, fmt.Errorf("error on parsing base58 encoded public key %s: %w", transmitter, err)
		}
		if _, err := buf.Write(pubKey.Bytes()); err != nil {
			return digest, err
		}
	}

	if err := binary.Write(buf, binary.BigEndian, cfg.F); err != nil {
		return digest, err
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(len(cfg.OnchainConfig))); err != nil { //nolint:gosec // cannot be negative and omax u32 exceeds max onchain config length
		return digest, err
	}

	if _, err := buf.Write(cfg.OnchainConfig); err != nil {
		return digest, err
	}

	if err := binary.Write(buf, binary.BigEndian, cfg.OffchainConfigVersion); err != nil {
		return digest, err
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(len(cfg.OffchainConfig))); err != nil { //nolint:gosec // cannot be negative and max u32 exceeds max offchain config length
		return digest, err
	}

	if _, err := buf.Write(cfg.OffchainConfig); err != nil {
		return digest, err
	}

	rawHash := buf.Sum(nil)
	if n := copy(digest[:], rawHash[:]); n != len(digest) {
		return digest, fmt.Errorf("incorrect hash size %d, expected %d", n, len(digest))
	}

	pre, err := d.ConfigDigestPrefix()
	if err != nil {
		return digest, err
	}
	binary.BigEndian.PutUint16(digest[0:2], uint16(pre))

	return digest, nil
}

// This should return the same constant value on every invocation
func (OffchainConfigDigester) ConfigDigestPrefix() (types.ConfigDigestPrefix, error) {
	return types.ConfigDigestPrefixSolana, nil
}
