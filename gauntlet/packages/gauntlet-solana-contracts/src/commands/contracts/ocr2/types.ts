export type OffchainConfig = {
  deltaProgressNanoseconds: number
  deltaResendNanoseconds: number
  deltaRoundNanoseconds: number
  deltaGraceNanoseconds: number
  deltaStageNanoseconds: number
  rMax: number
  s: number[]
  offchainPublicKeys: string[]
  peerIds: string[]
  reportingPluginConfig: {
    alphaReportInfinite: boolean
    alphaReportPpb: number
    alphaAcceptInfinite: boolean
    alphaAcceptPpb: number
    deltaCNanoseconds: number
  }
  maxDurationQueryNanoseconds: number
  maxDurationObservationNanoseconds: number
  maxDurationReportNanoseconds: number
  maxDurationShouldAcceptFinalizedReportNanoseconds: number
  maxDurationShouldTransmitAcceptedReportNanoseconds: number
  configPublicKeys: string[]
}

export type InspectionInput = {
  description: string
  decimals: string | number
  minAnswer: string | number
  maxAnswer: string | number
  transmitters: string[]
  payees: string[]
  signers: string[]
  offchainConfig: OffchainConfig
  billingAccessController: string
  requesterAccessController: string
  billing: {
    observationPaymentGjuels: string
    transmissionPaymentGjuels: string
  }
}

export const emptyInspectionInput: InspectionInput = {
  description: '',
  decimals: '',
  minAnswer: '',
  maxAnswer: '',
  transmitters: [],
  payees: [],
  signers: [],
  billingAccessController: '',
  requesterAccessController: '',
  offchainConfig: {
    deltaProgressNanoseconds: 0,
    deltaResendNanoseconds: 0,
    deltaRoundNanoseconds: 0,
    deltaGraceNanoseconds: 0,
    deltaStageNanoseconds: 0,
    rMax: 0,
    s: [],
    offchainPublicKeys: [],
    peerIds: [],
    reportingPluginConfig: {
      alphaReportInfinite: false,
      alphaReportPpb: 0,
      alphaAcceptInfinite: false,
      alphaAcceptPpb: 0,
      deltaCNanoseconds: 0,
    },
    maxDurationQueryNanoseconds: 0,
    maxDurationObservationNanoseconds: 0,
    maxDurationReportNanoseconds: 0,
    maxDurationShouldAcceptFinalizedReportNanoseconds: 0,
    maxDurationShouldTransmitAcceptedReportNanoseconds: 0,
    configPublicKeys: [],
  },
  billing: {
    observationPaymentGjuels: '',
    transmissionPaymentGjuels: '',
  },
}
