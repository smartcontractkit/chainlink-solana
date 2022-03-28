module.exports = {
  rootDir: '.',
  projects: [
    {
      displayName: 'gauntlet-solana',
      preset: 'ts-jest',
      testEnvironment: 'node',
      testMatch: ['<rootDir>/gaunlet/packages/gauntlet-solana/**/*.test.ts'],
      globals: {
        'ts-jest': {
          tsconfig: '<rootDir>/gaunlet/packages/gauntlet-solana/tsconfig.json',
        },
      },
    },
    {
      displayName: 'gauntlet-solana-contracts',
      preset: 'ts-jest',
      testEnvironment: 'node',
      testMatch: ['<rootDir>/gaunlet/packages/gauntlet-solana-contracts/**/*.test.ts'],
      globals: {
        'ts-jest': {
          tsconfig: '<rootDir>/gaunlet/packages/gauntlet-solana-contracts/tsconfig.json',
        },
      },
    },,
    {
      displayName: 'solana-sdk',
      preset: 'ts-jest',
      testEnvironment: 'node',
      testMatch: ['<rootDir>/ts/**/*.test.ts'],
      globals: {
        'ts-jest': {
          tsconfig: '<rootDir>/ts/tsconfig.json',
        },
      },
    },
  ],
}
