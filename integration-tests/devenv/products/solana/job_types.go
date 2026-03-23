package solana

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	"github.com/lib/pq"
	"github.com/pelletier/go-toml/v2"
	"github.com/pkg/errors"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink-common/pkg/types"
)

// TaskJobSpec represents an OCR2 job spec.
// Adapted from chainlink/devenv/products/ocr2/core.go -- these types should move to CTF
// once the extraction from devenv happens.
type TaskJobSpec struct {
	OCR2OracleSpec    OracleSpec
	Name              string `toml:"name"`
	JobType           string `toml:"type"`
	MaxTaskDuration   string `toml:"maxTaskDuration"`
	ObservationSource string `toml:"observationSource"`
	ForwardingAllowed bool   `toml:"forwardingAllowed"`
}

type OracleSpec struct {
	UpdatedAt                         time.Time            `toml:"-"`
	CreatedAt                         time.Time            `toml:"-"`
	OnchainSigningStrategy            JSONConfig           `toml:"onchainSigningStrategy"`
	PluginConfig                      JSONConfig           `toml:"pluginConfig"`
	RelayConfig                       JSONConfig           `toml:"relayConfig"`
	PluginType                        types.OCR2PluginType `toml:"pluginType"`
	ChainID                           string               `toml:"chainID"`
	ContractID                        string               `toml:"contractID"`
	Relay                             string               `toml:"relay"`
	P2PV2Bootstrappers                pq.StringArray       `toml:"p2pv2Bootstrappers"`
	OCRKeyBundleID                    null.String          `toml:"ocrKeyBundleID"`
	TransmitterID                     null.String          `toml:"transmitterID"`
	MonitoringEndpoint                null.String          `toml:"monitoringEndpoint"`
	ContractConfigTrackerPollInterval Interval             `toml:"contractConfigTrackerPollInterval"`
	BlockchainTimeout                 Interval             `toml:"blockchainTimeout"`
	ID                                int32                `toml:"-"`
	ContractConfigConfirmations       uint16               `toml:"contractConfigConfirmations"`
	CaptureEATelemetry                bool                 `toml:"captureEATelemetry"`
}

type JSONConfig map[string]any

type Interval time.Duration

func (o *TaskJobSpec) Type() string { return o.JobType }

func (o *TaskJobSpec) String() (string, error) {
	relayConfig, err := toml.Marshal(struct {
		RelayConfig JSONConfig `toml:"relayConfig"`
	}{RelayConfig: o.OCR2OracleSpec.RelayConfig})
	if err != nil {
		return "", fmt.Errorf("failed to marshal relay config: %w", err)
	}
	specWrap := struct {
		PluginConfig             map[string]any
		RelayConfig              string
		OCRKeyBundleID           string
		ObservationSource        string
		ContractID               string
		Relay                    string
		PluginType               string
		Name                     string
		MaxTaskDuration          string
		JobType                  string
		TransmitterID            string
		MonitoringEndpoint       string
		P2PV2Bootstrappers       []string
		BlockchainTimeout        time.Duration
		TrackerSubscribeInterval time.Duration
		TrackerPollInterval      time.Duration
		ContractConfirmations    uint16
		ForwardingAllowed        bool
	}{
		Name:                  o.Name,
		JobType:               o.JobType,
		ForwardingAllowed:     o.ForwardingAllowed,
		MaxTaskDuration:       o.MaxTaskDuration,
		ContractID:            o.OCR2OracleSpec.ContractID,
		Relay:                 o.OCR2OracleSpec.Relay,
		PluginType:            string(o.OCR2OracleSpec.PluginType),
		RelayConfig:           string(relayConfig),
		PluginConfig:          o.OCR2OracleSpec.PluginConfig,
		P2PV2Bootstrappers:    o.OCR2OracleSpec.P2PV2Bootstrappers,
		OCRKeyBundleID:        o.OCR2OracleSpec.OCRKeyBundleID.String,
		MonitoringEndpoint:    o.OCR2OracleSpec.MonitoringEndpoint.String,
		TransmitterID:         o.OCR2OracleSpec.TransmitterID.String,
		BlockchainTimeout:     o.OCR2OracleSpec.BlockchainTimeout.Duration(),
		ContractConfirmations: o.OCR2OracleSpec.ContractConfigConfirmations,
		TrackerPollInterval:   o.OCR2OracleSpec.ContractConfigTrackerPollInterval.Duration(),
		ObservationSource:     o.ObservationSource,
	}
	ocr2TemplateString := `
type                                   = "{{ .JobType }}"
name                                   = "{{.Name}}"
forwardingAllowed                      = {{.ForwardingAllowed}}
{{- if .MaxTaskDuration}}
maxTaskDuration                        = "{{ .MaxTaskDuration }}" {{end}}
{{- if .PluginType}}
pluginType                             = "{{ .PluginType }}" {{end}}
relay                                  = "{{.Relay}}"
schemaVersion                          = 1
contractID                             = "{{.ContractID}}"
{{- if eq .JobType "offchainreporting2" }}
ocrKeyBundleID                         = "{{.OCRKeyBundleID}}" {{end}}
{{- if eq .JobType "offchainreporting2" }}
transmitterID                          = "{{.TransmitterID}}" {{end}}
{{- if .BlockchainTimeout}}
blockchainTimeout                      = "{{.BlockchainTimeout}}"
{{end}}
{{- if .ContractConfirmations}}
contractConfigConfirmations            = {{.ContractConfirmations}}
{{end}}
{{- if .TrackerPollInterval}}
contractConfigTrackerPollInterval      = "{{.TrackerPollInterval}}"
{{end}}
{{- if .TrackerSubscribeInterval}}
contractConfigTrackerSubscribeInterval = "{{.TrackerSubscribeInterval}}"
{{end}}
{{- if .P2PV2Bootstrappers}}
p2pv2Bootstrappers                     = [{{range .P2PV2Bootstrappers}}"{{.}}",{{end}}]{{end}}
{{- if .MonitoringEndpoint}}
monitoringEndpoint                     = "{{.MonitoringEndpoint}}" {{end}}
{{- if .ObservationSource}}
observationSource                      = """
{{.ObservationSource}}
"""{{end}}
{{if eq .JobType "offchainreporting2" }}
[pluginConfig]{{range $key, $value := .PluginConfig}}
{{$key}} = {{$value}}{{end}}
{{end}}
{{.RelayConfig}}
`
	return marshallTemplate(specWrap, "OCR2 Job", ocr2TemplateString)
}

func marshallTemplate(jobSpec any, name, templateString string) (string, error) {
	var buf bytes.Buffer
	tmpl, err := template.New(name).Parse(templateString)
	if err != nil {
		return "", err
	}
	err = tmpl.Execute(&buf, jobSpec)
	if err != nil {
		return "", err
	}
	return buf.String(), err
}

func (r JSONConfig) Bytes() []byte {
	b, _ := json.Marshal(r)
	return b
}

func (r JSONConfig) Value() (driver.Value, error) {
	return json.Marshal(r)
}

func (r *JSONConfig) Scan(value any) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.Errorf("expected bytes got %T", b)
	}
	return json.Unmarshal(b, &r)
}

func NewInterval(d time.Duration) *Interval {
	i := new(Interval)
	*i = Interval(d)
	return i
}

func (i Interval) Duration() time.Duration {
	return time.Duration(i)
}

func (i Interval) MarshalText() ([]byte, error) {
	return []byte(time.Duration(i).String()), nil
}

func (i *Interval) UnmarshalText(input []byte) error {
	v, err := time.ParseDuration(string(input))
	if err != nil {
		return err
	}
	*i = Interval(v)
	return nil
}

func (i *Interval) Scan(v any) error {
	if v == nil {
		*i = Interval(time.Duration(0))
		return nil
	}
	asInt64, is := v.(int64)
	if !is {
		return errors.Errorf("models.Interval#Scan() wanted int64, got %T", v)
	}
	*i = Interval(time.Duration(asInt64) * time.Nanosecond)
	return nil
}

func (i Interval) ValueDB() (driver.Value, error) {
	return time.Duration(i).Nanoseconds(), nil
}

func (i Interval) IsZero() bool {
	return time.Duration(i) == time.Duration(0)
}
