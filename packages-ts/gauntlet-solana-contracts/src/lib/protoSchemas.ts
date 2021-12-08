const header = {
  // state: {
  //   type: 'protoimpl.MessageState',
  // },
  // sizeCache: {
  //   type: 'protoimpl.SizeCache',
  // },
  // unknownFields: {
  //   type: 'protoimpl.UnknownFields',
  // },
}
export const offchainDescriptor = {
  nested: {
    reporting_plugin_config: {
      fields: {
        ...header,
        alphaReportInfinite: {
          type: 'bool',
          id: 1,
        },
        alphaReportPpb: {
          type: 'int64',
          id: 2,
        },
        alphaAcceptInfinite: {
          type: 'bool',
          id: 3,
        },
        alphaAcceptPpb: {
          type: 'int64',
          id: 4,
        },
        deltaCNanoseconds: {
          type: 'int64',
          id: 5,
        },
      },
    },
    shared_secret_encryptions: {
      fields: {
        diffieHellmanPoint: {
          type: 'bytes',
          id: 1,
        },
        sharedSecretHash: {
          type: 'bytes',
          id: 2,
        },
        encryptions: {
          type: 'bytes',
          id: 3,
        },
      },
    },
    offchain_config: {
      fields: {
        ...header,
        deltaProgressNanoseconds: {
          type: 'int64',
          id: 1,
        },
        deltaResendNanoseconds: {
          type: 'int64',
          id: 2,
        },
        deltaRoundNanoseconds: {
          type: 'int64',
          id: 3,
        },
        deltaGraceNanoseconds: {
          type: 'int64',
          id: 4,
        },
        deltaStageNanoseconds: {
          type: 'int64',
          id: 5,
        },
        rMax: {
          type: 'int64',
          id: 6,
        },
        s: {
          type: 'bytes',
          id: 7,
        },
        offchainPublicKeys: {
          type: 'bytes',
          id: 8,
        },
        peerIds: {
          type: 'bytes',
          id: 9,
        },
        reportingPluginConfig: {
          type: 'bytes',
          id: 10,
        },
        maxDurationQueryNanoseconds: {
          type: 'int64',
          id: 11,
        },
        maxDurationObservationNanoseconds: {
          type: 'int64',
          id: 12,
        },
        maxDurationReportNanoseconds: {
          type: 'int64',
          id: 13,
        },
        maxDurationShouldAcceptFinalizedReportNanoseconds: {
          type: 'int64',
          id: 14,
        },
        maxDurationShouldTransmitAcceptedReportNanoseconds: {
          type: 'int64',
          id: 15,
        },
        sharedSecretEncryptions: {
          type: 'bytes',
          id: 16,
        },
      },
    },
  },
}
