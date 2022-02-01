package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/smartcontractkit/integrations-framework/client"
)

const (
	ChainName        = "solana"
	PlaceholderName  = "insert display name here"
	PlaceholderAdmin = "insert admin address here"
)

type NodeOutput struct {
	AdminAddress          string   `json:"adminAddress"`
	CSAKeys               []CSAKey `json:"csaKeys,omitempty"`
	DisplayName           string   `json:"displayName"`
	OCR2ConfigPublicKey   []string `json:"ocr2ConfigPublicKey"`
	OCR2OffchainPublicKey []string `json:"ocr2OffchainPublicKey"`
	OCR2OnchainPublicKey  []string `json:"ocr2OnchainPublicKey"`
	OCRNodeAddress        []string `json:"ocrNodeAddress"`
	PeerID                []string `json:"peerId"`
	Status                string   `json:"status"`
}

type CSAKey struct {
	NodeAddress string `json:"nodeAddress"`
	NodeName    string `json:"nodeName"`
	PublicKey   string `json:"publicKey"`
}

func main() {
	cl, err := client.NewChainlink(&client.ChainlinkConfig{
		URL:      "https://localhost:6688",
		Email:    "admin@chain.link",
		Password: "twoChains",
	}, http.DefaultClient)
	if err != nil {
		log.Fatal(err)
	}

	ocr2Keys, err := cl.ReadOCR2Keys()
	txKeys, err := cl.ReadTxKeys(ChainName)
	p2pKeys, err := cl.ReadP2PKeys()
	csaKeys, err := cl.ReadCSAKeys()

	var ocr2Key client.OCR2KeyData
	for _, k := range ocr2Keys.Data {
		if k.Attributes.ChainType == ChainName {
			ocr2Key = k
			break
		}
	}

	output := NodeOutput{
		AdminAddress: PlaceholderAdmin,
		CSAKeys: []CSAKey{CSAKey{
			NodeAddress: txKeys.Data[0].Attributes.PublicKey,
			NodeName:    PlaceholderName,
			PublicKey:   csaKeys.Data[0].Attributes.PublicKey,
		}},
		DisplayName:           PlaceholderName,
		OCR2ConfigPublicKey:   []string{ocr2Key.Attributes.ConfigPublicKey},
		OCR2OffchainPublicKey: []string{ocr2Key.Attributes.OffChainPublicKey},
		OCR2OnchainPublicKey:  []string{ocr2Key.Attributes.OnChainPublicKey},
		OCRNodeAddress:        []string{txKeys.Data[0].Attributes.PublicKey},
		PeerID:                []string{p2pKeys.Data[0].Attributes.PeerID},
		Status:                "active",
	}

	out, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatal(errors.New("failed to marshal output"))
	}
	fmt.Printf("\n\nKeys output:\n%s\n", string(out))
}
