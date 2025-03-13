package utils

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

type GithubRelease struct {
	TagName string `json:"tag_name"`
}

func GetLatestReleaseFromGithub(
	ctx *cli.Context,
	owner string,
	repo string,
) (string, error) {
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/releases/latest",
		owner,
		repo,
	)

	req, err := http.NewRequestWithContext(ctx.Context, "GET", url, nil)
	if err != nil {
		return "", err
	}

	res, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download release asset - request failed with status code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var release GithubRelease
	if err = json.Unmarshal(body, &release); err != nil {
		return "", err
	} else {
		return release.TagName, nil
	}
}

func DownloadTarGzReleaseAssetFromGithub(
	ctx *cli.Context,
	owner string,
	repo string,
	name string,
	tag string,
	dir string,
	vrb bool,
) error {
	url := fmt.Sprintf(
		"https://github.com/%s/%s/releases/download/%s/%s",
		owner,
		repo,
		tag,
		name,
	)

	req, err := http.NewRequestWithContext(ctx.Context, "GET", url, nil)
	if err != nil {
		return err
	}

	res, err := (&http.Client{}).Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download release asset - request failed with status code %d", res.StatusCode)
	}

	gzipReader, err := gzip.NewReader(res.Body)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if header.Typeflag == tar.TypeReg {
			pth := filepath.Join(dir, header.Name)

			if err := os.MkdirAll(filepath.Dir(pth), os.ModePerm); err != nil {
				return err
			}

			outFile, err := os.Create(pth)
			if err != nil {
				return err
			}

			if _, err = func() (int64, error) {
				defer outFile.Close()
				return io.Copy(outFile, tarReader)
			}(); err != nil {
				return err
			}

			if vrb {
				fmt.Fprintf(ctx.App.Writer, "Extracted %s\n", pth)
			}
		}
	}

	return nil
}
