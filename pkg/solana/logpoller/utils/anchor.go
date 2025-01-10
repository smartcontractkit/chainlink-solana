package utils

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

const DiscriminatorLength = 8

func Discriminator(namespace, name string) [DiscriminatorLength]byte {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%s", namespace, name)))
	return [DiscriminatorLength]byte(h.Sum(nil)[:DiscriminatorLength])
}

func IsEvent(event string, data []byte) bool {
	if len(data) < 8 {
		return false
	}
	d := Discriminator("event", event)
	return bytes.Equal(d[:], data[:8])
}

func GetBlockTime(ctx context.Context, client *rpc.Client, commitment rpc.CommitmentType) (*solana.UnixTimeSeconds, error) {
	block, err := client.GetBlockHeight(ctx, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to get block height: %w", err)
	}

	blockTime, err := client.GetBlockTime(ctx, block)
	if err != nil {
		return nil, fmt.Errorf("failed to get block time: %w", err)
	}

	return blockTime, nil
}
