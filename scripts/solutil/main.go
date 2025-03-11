package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/gagliardetto/solana-go"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:        "solutil",
		Description: "A collection of helper commands for Solana",
		Commands: []*cli.Command{
			{
				Name:  "private-key",
				Flags: []cli.Flag{&cli.StringFlag{Name: "path", Aliases: []string{"p"}, Required: false}},
				Action: func(ctx *cli.Context) error {
					keypairPath := ctx.String("path")
					if keypairPath == "" {
						keypairPath = filepath.Join(os.Getenv("HOME"), ".config", "solana", "id.json")
					}

					privKey, err := solana.PrivateKeyFromSolanaKeygenFile(keypairPath)
					if err != nil {
						return cli.Exit(err, 1)
					}

					fmt.Fprint(ctx.App.Writer, privKey.String())
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
