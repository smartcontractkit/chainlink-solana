package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"solutil/utils"

	"github.com/gagliardetto/solana-go"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:        "solutil",
		Description: "A collection of helper commands for Solana",
		Commands: []*cli.Command{
			{
				Name:  "print-private-key",
				Flags: []cli.Flag{&cli.StringFlag{Name: "path", Aliases: []string{"p"}, Required: false}},
				Action: func(ctx *cli.Context) error {
					keypairPath := ctx.String("path")
					if keypairPath == "" {
						keypairPath = filepath.Join(os.Getenv("HOME"), ".config", "solana", "id.json")
					}

					if _, err := os.Stat(keypairPath); err != nil {
						return cli.Exit(err, 1)
					}

					if privKey, err := solana.PrivateKeyFromSolanaKeygenFile(keypairPath); err != nil {
						return cli.Exit(err, 1)
					} else {
						fmt.Fprint(ctx.App.Writer, privKey.String())
					}

					return nil
				},
			},
			{
				Name: "download-contract-artifacts",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "outdir", Aliases: []string{"o"}, Required: false},
					&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Required: false},
					&cli.StringFlag{Name: "tag", Aliases: []string{"t"}, Required: false},
				},
				Action: func(ctx *cli.Context) error {
					const owner = "smartcontractkit"
					const repo = "chainlink-ccip"
					const name = "artifacts.tar.gz"

					dir := ctx.String("outdir")
					if dir == "" {
						if cwd, err := os.Getwd(); err != nil {
							return cli.Exit(err, 1)
						} else {
							dir = cwd
						}
					}

					tag := ctx.String("tag")
					if tag == "" {
						if t, err := utils.GetLatestReleaseFromGithub(ctx, owner, repo); err != nil {
							return err
						} else {
							tag = t
						}
					}

					verbose := ctx.Bool("verbose")
					if err := utils.DownloadTarGzReleaseAssetFromGithub(ctx, owner, repo, name, tag, dir, verbose); err != nil {
						return cli.Exit(err, 1)
					} else {
						return nil
					}
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
