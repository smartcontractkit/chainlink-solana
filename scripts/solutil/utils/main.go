package utils

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"golang.org/x/mod/modfile"
)

type GithubRelease struct {
	TagName string `json:"tag_name"`
}

type GithubCommit struct {
	Sha string `json:"sha"`
}

func withGetRequest[T any](ctx context.Context, url string, cb func(res *http.Response) (T, error)) (T, error) {
	var empty T

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return empty, err
	}

	res, err := (&http.Client{}).Do(req)
	if err != nil {
		return empty, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return empty, fmt.Errorf("GET request failed with status code %d", res.StatusCode)
	}

	return cb(res)
}

func GetLatestReleaseFromGithub(
	ctx context.Context,
	owner string,
	repo string,
) (string, error) {
	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/%s/releases/latest",
		owner,
		repo,
	)

	return withGetRequest(ctx, url, func(res *http.Response) (string, error) {
		var release GithubRelease
		if err := json.NewDecoder(res.Body).Decode(&release); err != nil {
			return "", err
		}
		return release.TagName, nil
	})
}

func GetLongShaFromGithub(
	ctx context.Context,
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

	return withGetRequest(ctx, url, func(res *http.Response) (string, error) {
		var parsed []GithubCommit
		if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
			return "", err
		}

		if len(parsed) == 0 {
			return "", errors.New("failed to get long SHA")
		}
		return parsed[0].Sha, nil
	})
}

func DownloadTarGzReleaseAssetFromGithub(
	ctx context.Context,
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

	_, err := withGetRequest(ctx, url, func(res *http.Response) (any, error) {
		gzipReader, err := gzip.NewReader(res.Body)
		if err != nil {
			return nil, err
		}
		defer gzipReader.Close()

		tarReader := tar.NewReader(gzipReader)
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if err := cb(tarReader, header); err != nil {
				return nil, err
			}
		}

		return nil, nil
	})

	return err
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
