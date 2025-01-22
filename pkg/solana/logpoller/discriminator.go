package logpoller

import (
	"crypto/sha256"
	"fmt"
)

const DiscriminatorLength = 8

func Discriminator(namespace, name string) [DiscriminatorLength]byte {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%s", namespace, name)))
	return [DiscriminatorLength]byte(h.Sum(nil)[:DiscriminatorLength])
}
