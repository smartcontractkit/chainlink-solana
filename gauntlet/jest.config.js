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
    },
  ],
}
