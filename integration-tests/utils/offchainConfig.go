package utils

type OCR2OnChainConfig struct {
	Oracles    []Operator `json:"oracles"`
	F          int        `json:"f"`
	ProposalId string     `json:"proposalId"`
}

type OffchainConfig struct {
	DeltaProgressNanoseconds                           int64                 `json:"deltaProgressNanoseconds"`
	DeltaResendNanoseconds                             int64                 `json:"deltaResendNanoseconds"`
	DeltaRoundNanoseconds                              int64                 `json:"deltaRoundNanoseconds"`
	DeltaGraceNanoseconds                              int64                 `json:"deltaGraceNanoseconds"`
	DeltaStageNanoseconds                              int64                 `json:"deltaStageNanoseconds"`
	RMax                                               int                   `json:"rMax"`
	S                                                  []int                 `json:"s"`
	OffchainPublicKeys                                 []string              `json:"offchainPublicKeys"`
	PeerIds                                            []string              `json:"peerIds"`
	ReportingPluginConfig                              ReportingPluginConfig `json:"reportingPluginConfig"`
	MaxDurationQueryNanoseconds                        int64                 `json:"maxDurationQueryNanoseconds"`
	MaxDurationObservationNanoseconds                  int64                 `json:"maxDurationObservationNanoseconds"`
	MaxDurationReportNanoseconds                       int64                 `json:"maxDurationReportNanoseconds"`
	MaxDurationShouldAcceptFinalizedReportNanoseconds  int64                 `json:"maxDurationShouldAcceptFinalizedReportNanoseconds"`
	MaxDurationShouldTransmitAcceptedReportNanoseconds int64                 `json:"maxDurationShouldTransmitAcceptedReportNanoseconds"`
	ConfigPublicKeys                                   []string              `json:"configPublicKeys"`
}

type ReportingPluginConfig struct {
	AlphaReportInfinite bool `json:"alphaReportInfinite"`
	AlphaReportPpb      int  `json:"alphaReportPpb"`
	AlphaAcceptInfinite bool `json:"alphaAcceptInfinite"`
	AlphaAcceptPpb      int  `json:"alphaAcceptPpb"`
	DeltaCNanoseconds   int  `json:"deltaCNanoseconds"`
}

// TODO - Decouple all OCR2 config structs to be reusable between chains
type OCROffChainConfig struct {
	ProposalId     string         `json:"proposalId"`
	OffchainConfig OffchainConfig `json:"offchainConfig"`
	UserSecret     string         `json:"userSecret"`
}

type Operator struct {
	Signer      string `json:"signer"`
	Transmitter string `json:"transmitter"`
	Payee       string `json:"payee"`
}

type PayeeConfig struct {
	Operators  []Operator `json:"operators"`
	ProposalId string     `json:"proposalId"`
}

type ProposalAcceptConfig struct {
	ProposalId     string         `json:"proposalId"`
	Version        int            `json:"version"`
	F              int            `json:"f"`
	Oracles        []Operator     `json:"oracles"`
	OffchainConfig OffchainConfig `json:"offchainConfig"`
	RandomSecret   string         `json:"randomSecret"`
}

type StoreFeedConfig struct {
	Store       string `json:"store"`
	Granularity int    `json:"granularity"`
	LiveLength  int    `json:"liveLength"`
	Decimals    int    `json:"decimals"`
	Description string `json:"description"`
}

type OCR2Config struct {
	MinAnswer     string `json:"minAnswer"`
	MaxAnswer     string `json:"maxAnswer"`
	Transmissions string `json:"transmissions"`
}

type OCR2BillingConfig struct {
	ObservationPaymentGjuels  int `json:"ObservationPaymentGjuels"`
	TransmissionPaymentGjuels int `json:"TransmissionPaymentGjuels"`
}

type StoreWriterConfig struct {
	Transmissions string `json:"transmissions"`
}

type Transmission struct {
	LatestTransmissionNo int64  `json:"latestTransmissionNo"`
	RoundId              int64  `json:"roundId"`
	Answer               int64  `json:"answer"`
	Transmitter          string `json:"transmitter"`
}

func ChunkSlice(items []byte, chunkSize int) (chunks [][]byte) {
	for chunkSize < len(items) {
		chunks = append(chunks, items[0:chunkSize])
		items = items[chunkSize:]
	}
	return append(chunks, items)
}
