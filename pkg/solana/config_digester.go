package solana

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/smartcontractkit/libocr/offchainreporting2/types"
)

// TODO move this in with the Terra constant
const ConfigDigestPrefixSolana types.ConfigDigestPrefix = 3

// ConfigDigest is meant to do the same thing as config_digest_from_data from the program.
func (c ContractTracker) ConfigDigest(cfg types.ContractConfig) (types.ConfigDigest, error) {
	configDigest := types.ConfigDigest{}
	buf := sha256.New()

	if _, err := buf.Write(c.programAccount[:]); err != nil {
		return configDigest, err
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(cfg.ConfigCount)); err != nil {
		return configDigest, err
	}

	if err := binary.Write(buf, binary.BigEndian, uint8(len(cfg.Signers))); err != nil {
		return configDigest, err
	}
	for _, signer := range cfg.Signers {
		if _, err := buf.Write(signer); err != nil {
			return configDigest, err
		}
	}

	for _, transmitter := range cfg.Transmitters {
		pubKey, err := solana.PublicKeyFromBase58(string(transmitter))
		if err != nil {
			return configDigest, fmt.Errorf("unable to parse base58 encoded public key (%s): %w", transmitter, err)
		}
		if _, err := buf.Write(pubKey[:]); err != nil {
			return configDigest, err
		}
	}

	if err := binary.Write(buf, binary.BigEndian, byte(cfg.F)); err != nil {
		return configDigest, err
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(len(cfg.OnchainConfig))); err != nil {
		return configDigest, err
	}
	if _, err := buf.Write(cfg.OnchainConfig); err != nil {
		return configDigest, err
	}

	if err := binary.Write(buf, binary.BigEndian, cfg.OffchainConfigVersion); err != nil {
		return configDigest, err
	}

	if err := binary.Write(buf, binary.BigEndian, uint32(len(cfg.OffchainConfig))); err != nil {
		return configDigest, err
	}
	if _, err := buf.Write(cfg.OffchainConfig); err != nil {
		return configDigest, err
	}

	rawHash := buf.Sum(nil)
	if n := copy(configDigest[:], rawHash[:]); n != len(configDigest) {
		return configDigest, fmt.Errorf("incorrect hash size %d, expected %d", n, len(configDigest))
	}

	configDigest[0] = 0x00
	configDigest[1] = uint8(ConfigDigestPrefixSolana)

	return configDigest, nil
}

// This should return the same constant value on every invocation
func (c ContractTracker) ConfigDigestPrefix() types.ConfigDigestPrefix {
	return ConfigDigestPrefixSolana
}
