package utils

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/urfave/cli/v2"
	"golang.org/x/mod/modfile"
)

type GithubRelease struct {
	TagName string `json:"tag_name"`
}

type GithubCommit struct {
	Sha string `json:"sha"`
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

func GetLongShaFromGithub(
	ctx *cli.Context,
	owner string,
	repo string,
	sha string,
) (string, error) {
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/commits?sha=%s&per_page=1",
		owner,
		repo,
		sha,
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
		return "", fmt.Errorf("failed to retrieve long SHA - request failed with status code %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}

	var parsed []GithubCommit
	if err = json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}

	if len(parsed) == 0 {
		return "", errors.New("failed to get long SHA")
	} else {
		return parsed[0].Sha, nil
	}
}

func DownloadTarGzReleaseAssetFromGithub(
	ctx *cli.Context,
	owner string,
	repo string,
	name string,
	tag string,
	cb func(r *tar.Reader, h *tar.Header) error,
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
		if err := cb(tarReader, header); err != nil {
			return err
		}
	}

	return nil
}

func GetDependencyVersion(gomodPath string, dependency string) (*modfile.Require, error) {
	gomod, err := os.ReadFile(gomodPath)
	if err != nil {
		return nil, err
	}

	modFile, err := modfile.ParseLax("go.mod", gomod, nil)
	if err != nil {
		return nil, err
	}

	for _, dep := range modFile.Require {
		if dep.Mod.Path == dependency {
			return dep, nil
		}
	}

	return nil, fmt.Errorf("dependency %s not found", dependency)
}
