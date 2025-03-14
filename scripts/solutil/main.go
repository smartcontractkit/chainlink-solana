package main

import (
	"archive/tar"
	_ "embed"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"solutil/utils"
	"strings"

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
						if t, err := utils.GetLatestReleaseFromGithub(ctx.Context, owner, repo); err != nil {
							return err
						} else {
							tag = t
						}
					}

					err := utils.DownloadTarGzReleaseAssetFromGithub(ctx.Context, owner, repo, name, tag, func(r *tar.Reader, h *tar.Header) error {
						if h.Typeflag == tar.TypeReg && filepath.Ext(h.Name) == ".so" {
							outPath := filepath.Join(dir, filepath.Base(h.Name))
							if err := os.MkdirAll(filepath.Dir(outPath), os.ModePerm); err != nil {
								return err
							}

							outFile, err := os.Create(outPath)
							if err != nil {
								return err
							}
							defer outFile.Close()

							if _, err := io.Copy(outFile, r); err != nil {
								return err
							}

							if ctx.Bool("verbose") {
								fmt.Fprintf(ctx.App.Writer, "Extracted %s\n", outPath)
							}
						}
						return nil
					})

					if err != nil {
						return cli.Exit(err, 1)
					} else {
						return nil
					}
				},
			},
			{
				Name: "get-dependency-version",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "dependency", Aliases: []string{"d"}, Required: true},
					&cli.StringFlag{Name: "path", Aliases: []string{"p"}, Required: false},
				},
				Action: func(ctx *cli.Context) error {
					path := ctx.String("path")
					if path == "" {
						if cwd, err := os.Getwd(); err != nil {
							return cli.Exit(err, 1)
						} else {
							path = filepath.Join(cwd, "go.mod")
						}
					}

					dep, err := utils.GetDependencyVersion(path, ctx.String("dependency"))
					if err != nil {
						return cli.Exit(err, 1)
					}

					tokens := strings.Split(dep.Mod.Version, "-")
					if len(tokens) == 3 {
						fmt.Fprintln(ctx.App.Writer, tokens[2])
					} else {
						fmt.Fprintln(ctx.App.Writer, dep.Mod.Version)
					}

					return nil
				},
			},
			{
				Name: "get-long-sha",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "owner", Aliases: []string{"o"}, Required: true},
					&cli.StringFlag{Name: "repo", Aliases: []string{"r"}, Required: true},
					&cli.StringFlag{Name: "sha", Aliases: []string{"s"}, Required: true},
				},
				Action: func(ctx *cli.Context) error {
					if sha, err := utils.GetLongShaFromGithub(ctx.Context, ctx.String("owner"), ctx.String("repo"), ctx.String("sha")); err != nil {
						return cli.Exit(err, 1)
					} else {
						fmt.Fprintln(ctx.App.Writer, sha)
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
