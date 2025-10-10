package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/smartcontractkit/chainlink-solana/pkg/solana/config"
)

var outDir = flag.String("o", "", "output directory")

func main() {
	s, err := config.GenerateDocs()
	if err != nil {
		log.Fatalln("Failed to generate docs:", err)
	}
	if err = os.WriteFile(filepath.Join(*outDir, "CONFIG.md"), []byte(s), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write config docs: %v\n", err)
		os.Exit(1)
	}
}
