import { FlowCommand } from '@chainlink/gauntlet-core'
import { TransactionResponse, waitExecute } from '@chainlink/gauntlet-solana'
import { CONTRACT_LIST } from '../../../lib/contracts'
import { makeAbstractCommand } from '../../abstract'
import Initialize from './initialize'
import InitializeAC from '../accessController/initialize'
import DeployToken from '../token/deploy'
import SetConfigDeployer from './setConfig.deployer'
import { Keypair } from '@solana/web3.js'
import SetPayees from './setPayees'

const transmitters = new Array(5).fill(0).map(() => Keypair.generate().publicKey.toString())
const operators = [
  {
    adminAddress: '0xD9459cc85E78e0336aDb349EAbF257Dbaf9d5a2B',
    displayName: '01Node',
    NodeAddress: '4K64YFRdyUhpJfkZYZTH7EnuPDieiFV9zRpWLCxPA2eJ',
    OCRConfigPublicKey: 'a1fb5358719191e5b18dd2de182dea175aa5e45bf520014d121c3353879d5365',
    OCROnchainPublicKey: '0xCF4Be57aA078Dc7568C631BE7A73adc1cdA992F8',
    OCROffchainPublicKey: '23592f9c86d4ed6f10d07169e32a8896ce4cb1374b2e4d09f388699a5a4ed061',
    ocrSigningAddress: '0x080D263FAA8CBd848f0b9B24B40e1f23EA06b3A3',
    oracleAddress: '0x64Fd0aBd4d8F5A0A7502C0078d959E57ca1207aB',
    payeeAddress: '4K64YFRdyUhpJfkZYZTH7EnuPDieiFV9zRpWLCxPA2eJ',
    P2PID: '12D3KooWJGF9SBY2HG5TLHfJrMv8bom8DGjL21MAziR6tBa6z1jr',
    status: 'active',
    website: 'https://01node.com',
  },
  {
    adminAddress: '0x89177B9c203bA0A9294aecf2f3806d98907bec6f',
    displayName: 'T-Systems',
    NodeAddress: '2daKj8WrEBJCw2ZMGtbmbkAbHRXTFhkWBEGgHB4sDqRH',
    OCRConfigPublicKey: 'e63a1cdc87fef95ca4fa5920040f629e260ad098383865a78656226a60231371',
    OCROnchainPublicKey: '0xddEB598fe902A13Cc523aaff5240e9988eDCE170',
    OCROffchainPublicKey: '753a9d2674e251ed9bfd415bf984de11f0b6a5cf6cc12ee30fc76d715338ad88',
    ocrSigningAddress: '0xd54DDB3A256a40061C41Eb6ADF4f412ca8e17c25',
    oracleAddress: '0xc1D750678cCeDE3E794EE092cD4c913C40935360',
    payeeAddress: '2daKj8WrEBJCw2ZMGtbmbkAbHRXTFhkWBEGgHB4sDqRH',
    P2PID: '12D3KooWPRpU5d8NuEuUyRU2tSfG2HFZZTD9PmHLgcQBjixMKvaG',
    status: 'active',
    website: 'https://www.t-systems.com',
  },
  {
    adminAddress: '0xa5D0084A766203b463b3164DFc49D91509C12daB',
    displayName: 'Alpha Chain',
    NodeAddress: 'HEjF9jw281uWzhtUgMGsibzdWH7BTgNje2nxCby5batN',
    payeeAddress: 'HEjF9jw281uWzhtUgMGsibzdWH7BTgNje2nxCby5batN',
    OCRConfigPublicKey: 'b115a9612d6f94d4499588db04d659f79168804285e13cbefe6c3f446d77244e',
    OCROnchainPublicKey: '0x5a8216a9c47ee2E8Df1c874252fDEe467215C25b',
    OCROffchainPublicKey: '85bc5950f1bef38bf8fb19df44d23a6d8e65e22c3ba2a544cd1f8e3f19c4d86e',
    ocrSigningAddress: '0x55048BC9f3a3f373031fB32C0D0d5C1Bc6E10B3b',
    oracleAddress: '0x72f3dFf4CD17816604dd2df6C2741e739484CA62',
    P2PID: '12D3KooWSL6bvujABpNab94fMRuwDmateP9uBmhQDi6iUkm2oQoR',
    status: 'active',
    website: 'https://alphachain.io',
  },
  {
    adminAddress: '0x3615Fa045f00ae0eD60Dc0141911757c2AdC5E03',
    displayName: 'Anyblock',
    NodeAddress: 'BoSqV27jEQq5X6V4CqEa5ERvDJRgxvrVpNtKpyG4ZK5Z',
    payeeAddress: 'BoSqV27jEQq5X6V4CqEa5ERvDJRgxvrVpNtKpyG4ZK5Z',
    OCRConfigPublicKey: 'c4f7e80d1f51a1e80dedf936016871e4c0a388b0b51b77b6838900c157ccc928',
    OCROnchainPublicKey: '0x57CD4848b12469618b689163f507817940AccA02',
    OCROffchainPublicKey: 'da7f7b87602198cb9931dac86e2d63cb48b1408257d128201ba533aebb75a1b0',
    ocrSigningAddress: '0x8d4AE8b06701f53f7a34421461441E4492E1C578',
    oracleAddress: '0x9308B0Bd23794063423f484Cd21c59eD38898108',
    P2PID: '12D3KooWKKq7NqMiFUX3FA4RYrscr9n1dWxoURfFFVPMtVLcWsVs',
    status: 'active',
    website: 'https://www.anyblockanalytics.com',
  },
  {
    adminAddress: '0x7c9998a91AEA813Ea8340b47B27259D74896d136',
    displayName: 'Armanino',
    NodeAddress: 'CHMFsjLLykYDVTXBSCESo6Sejjx4rHRjEHEJs4ZA1czK',
    payeeAddress: 'CHMFsjLLykYDVTXBSCESo6Sejjx4rHRjEHEJs4ZA1czK',
    OCRConfigPublicKey: '26ba429e38bb34dd07c882666780970274644f7a91de349109998f1c84b5b04b',
    OCROnchainPublicKey: '0xD084c90d0e486ade2c045374dB447b99f94811Ee',
    OCROffchainPublicKey: 'c5e014feba0d48df117d71d39c4c4da0af70758a121b31ceae7dddf1c9c77a6c',
    ocrSigningAddress: '0x713BdE3B77539beAA7f48ee566bD5d5321755913',
    P2PID: '12D3KooWDg69v9x4ADjJg3998DrfjZDusaZ8PS9QFaEcsxmeDRjk',
    status: 'active',
    website: 'https://www.armaninollp.com/',
  },
]

// TODO: Remove. Not necessary. Useful for dev testing
export default class SetupFlow extends FlowCommand<TransactionResponse> {
  static id = 'ocr2:setup:flow'
  static category = CONTRACT_LIST.OCR_2
  static examples = ['yarn gauntlet ocr2:setup:flow --network=local']

  constructor(flags, args) {
    super(flags, args, waitExecute, makeAbstractCommand)

    this.stepIds = {
      BILLING_ACCESS_CONTROLLER: 1,
      REQUEST_ACCESS_CONTROLLER: 2,
      OCR_2: 3,
      TOKEN: 4,
    }

    this.flow = [
      {
        name: 'Deploy AC',
        command: 'access_controller:deploy',
      },
      {
        name: 'Deploy OCR',
        command: 'ocr2:deploy',
      },
      {
        name: 'Deploy LINK',
        command: DeployToken,
        id: this.stepIds.TOKEN,
      },
      {
        name: 'Initialize Billing AC',
        command: InitializeAC,
        id: this.stepIds.BILLING_ACCESS_CONTROLLER,
      },
      {
        name: 'Initialize Request AC',
        command: InitializeAC,
        id: this.stepIds.REQUEST_ACCESS_CONTROLLER,
      },
      {
        name: 'Initialize OCR 2',
        command: Initialize,
        flags: {
          billingAccessController: ID.contract(this.stepIds.BILLING_ACCESS_CONTROLLER),
          requesterAccessController: ID.contract(this.stepIds.REQUEST_ACCESS_CONTROLLER),
          link: ID.contract(this.stepIds.TOKEN),
          decimals: 8,
          minAnswer: 0,
          maxAnswer: 10000,
        },
        id: this.stepIds.OCR_2,
      },
      {
        name: 'Set Config',
        command: SetConfigDeployer,
        flags: {
          state: ID.contract(this.stepIds.OCR_2),
          keys: operators,
        },
      },
      {
        name: 'Set Payees',
        command: SetPayees,
        flags: {
          state: ID.contract(this.stepIds.OCR_2),
          keys: operators,
          link: ID.contract(this.stepIds.TOKEN),
        },
      },
    ]
  }
}

const ID = {
  contract: (id: number, index = 0): string => `ID.${id}.txs.${index}.contract`,
  tx: (id: number, index = 0): string => `ID.${id}.txs.${index}.tx`,
}
