module.exports = {
  rootDir: '.',
  projects: [
    {
      displayName: 'gauntlet-solana',
      preset: 'ts-jest',
      testEnvironment: 'node',
      testMatch: ['<rootDir>/packages/gauntlet-solana/**/*.test.ts'],
      globals: {
        'ts-jest': {
          tsconfig: '<rootDir>/packages/gauntlet-solana/tsconfig.json',
        },
      },
      moduleNameMapper: {
        // workaround for https://github.com/LedgerHQ/ledger-live/issues/763
        '@ledgerhq/devices/hid-framing': '<rootDir>/node_modules/@ledgerhq/devices/lib/hid-framing.js',
      },
    },
    {
      displayName: 'gauntlet-solana-contracts',
      preset: 'ts-jest',
      testEnvironment: 'node',
      testMatch: ['<rootDir>/packages/gauntlet-solana-contracts/**/*.test.ts'],
      globals: {
        'ts-jest': {
          tsconfig: '<rootDir>/packages/gauntlet-solana-contracts/tsconfig.json',
        },
      },
      moduleNameMapper: {
        // workaround for https://github.com/LedgerHQ/ledger-live/issues/763
        '@ledgerhq/devices/hid-framing': '<rootDir>/node_modules/@ledgerhq/devices/lib/hid-framing.js',
      },
    },
  ],
}
