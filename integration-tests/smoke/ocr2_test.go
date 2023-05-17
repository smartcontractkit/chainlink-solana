package smoke

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/smartcontractkit/chainlink-solana/integration-tests/utils"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-solana/integration-tests/common"
)

func TestSolanaOCRV2Smoke(t *testing.T) {
	state := common.NewOCRv2State(t, 1, "smoke")
	state.DeployCluster(utils.ContractsDir)
	t.Logf("urls %+v", state.Common.Env.URLs)
	testPromEndpoints(t, state.Common.Env.URLs["chainlink_local"])
	if state.Common.Env.WillUseRemoteRunner() {
		return
	}
	state.SetAllAdapterResponsesToTheSameValue(10)
	state.ValidateRoundsAfter(time.Now(), common.NewRoundCheckTimeout, 1)
}

func testPromEndpoints(t *testing.T, urls []string) {
	t.Logf("testing prom endpoints")
	require.Greater(t, len(urls), 0, "expected non-empty slice of node urls %v", urls)
	for _, url := range urls {

		r := resty.New()

		// discovery endpoint exists independent of whether LOOPP is enabled
		var expectedResponse []targetgroup.Group
		t.Logf("calling disco url %q", url+"/discovery")
		resp, err := r.R().SetResult(&expectedResponse).Get(url + "/discovery")
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, resp.StatusCode())
		t.Logf("node discovery targets %+v", expectedResponse)

		usingLoop := loopRuntimeEnabled(t)
		t.Logf("using LOOPP runtime: %v", usingLoop)
		if !usingLoop {
			require.Len(t, expectedResponse, 0)
		} else {
			require.Greater(t, len(expectedResponse), 0)
		}

		for _, target := range expectedResponse {
			p, ok := target.Labels[model.MetricsPathLabel]
			require.True(t, ok)
			t.Logf(" target %s meticPathLabel %s = %s", target.Source, model.MetricsPathLabel, p)
			resp, err := r.R().SetDoNotParseResponse(true).Get(string(p))
			require.NoError(t, err)
			defer resp.RawBody().Close()
			b, err := io.ReadAll(resp.RawBody())
			require.NoError(t, err)
			t.Logf("metrics response for %s,%s: %s", url, p, string(b))
		}
	}
}

// taken from CI matrix
const loop_version_tag = "plugins"

func loopRuntimeEnabled(t *testing.T) bool {
	// example of expected env var taken from CI logging
	// CHAINLINK_VERSION: 9d0e4362696e337f4b03412592582c5d90e42bf9-plugins
	v, exists := os.LookupEnv("CHAINLINK_VERSION")
	require.True(t, exists, "expected CHAINLINK_VERSION to exist in the env. Needed to infer whether or not LOOPP is enabled")
	return strings.HasSuffix(v, loop_version_tag)
}
