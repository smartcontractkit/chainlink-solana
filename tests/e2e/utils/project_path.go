package utils

import (
	"path/filepath"
	"runtime"
)

var (
	_, b, _, _ = runtime.Caller(0)
	// ProjectRoot Root folder of this project
	ProjectRoot = filepath.Join(filepath.Dir(b), "/../../..")
	// ContractsDir path to our contracts
	ContractsDir = filepath.Join(ProjectRoot, "contracts", "target", "deploy")
	// TestsDir path to e2e tests dir
	TestsDir = filepath.Join(ProjectRoot, "tests", "e2e")
	// ChartsRoot helm charts root
	ChartsRoot = filepath.Join(ProjectRoot, "ops", "k8s", "charts")
)
