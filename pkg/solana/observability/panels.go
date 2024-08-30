package observability

import "github.com/smartcontractkit/chainlink-common/observability-lib/grafana"

func GetPanels(dashboardOptions *grafana.DashboardOptions) []*grafana.Panel {
	var panels []*grafana.Panel

	props := &Props{
		Name:              dashboardOptions.Name,
		MetricsDataSource: dashboardOptions.MetricsDataSource,
		PlatformOpts:      PlatformPanelOpts(dashboardOptions.Platform),
		FolderUID:         dashboardOptions.FolderUID,
	}

	panels = append(panels, GetSOLBalancePanel(props))

	return panels
}

func GetSOLBalancePanel(p *Props) *grafana.Panel {
	return grafana.NewTimeSeriesPanel(&grafana.TimeSeriesPanelOptions{
		PanelOptions: &grafana.PanelOptions{
			Datasource: p.MetricsDataSource.Name,
			Title:      "SOL Balance",
			Span:       12,
			Height:     6,
			Decimals:   2,
			Query: []grafana.Query{
				{
					Expr:   `solana_balance{` + p.PlatformOpts.LabelQuery + `}`,
					Legend: `{{` + p.PlatformOpts.LegendString + `}} - {{account}}`,
				},
			},
		},
	})
}
