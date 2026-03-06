package main

import (
	"fmt"

	"github.com/smartcontractkit/chainlink-testing-framework/framework/components/fake"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/devenv/fakes"
)

func main() {
	_, err := fake.NewFakeDataProvider(&fake.Input{Port: fakes.FakeServicePort})
	if err != nil {
		panic(fmt.Sprintf("failed to start fake data provider: %v", err))
	}
	if err := fakes.RegisterRoutes(); err != nil {
		panic(fmt.Sprintf("failed to register routes: %v", err))
	}
	select {}
}
