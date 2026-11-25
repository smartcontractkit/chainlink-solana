const { PublicKey } = require("@solana/web3.js");
const { createHash } = require("crypto");
const fs = require("fs");
const path = require("path");

const PROGRAM_ID = new PublicKey(
  "9qCsUGTdKCanybkroJuPzAhuRU8P2SZN4NCexevSZAwu"
);
const STORE_ID = 424242;
const WRONG_STATE_STORE_ID = 424243;
const DATA_ID = Buffer.from("01020304050607080000000000000000", "hex");
const LINK_DATA_ID = Buffer.from("11020304050607080000000000000000", "hex");
const DF1_FEED = PublicKey.findProgramAddressSync(
  [Buffer.from("wrong-version-df1")],
  PROGRAM_ID
)[0];

const discriminator = (name) =>
  createHash("sha256").update(`account:${name}`).digest().subarray(0, 8);
const seed32 = (id) => Buffer.concat([id, Buffer.alloc(16)]);
const storeIdLe = (id) => {
  const out = Buffer.alloc(8);
  out.writeBigUInt64LE(BigInt(id));
  return out;
};
const statePda = (storeId) =>
  PublicKey.findProgramAddressSync(
    [Buffer.from("state"), storeIdLe(storeId)],
    PROGRAM_ID
  )[0];
const feedPda = (state, id) =>
  PublicKey.findProgramAddressSync(
    [Buffer.from("feed"), state.toBuffer(), seed32(id)],
    PROGRAM_ID
  )[0];
const linkPda = (state, df1Feed) =>
  PublicKey.findProgramAddressSync(
    [Buffer.from("df1_link"), state.toBuffer(), df1Feed.toBuffer()],
    PROGRAM_ID
  )[0];

const stateAccount = (version) => {
  const data = Buffer.alloc(8 + 3232);
  discriminator("StoreState").copy(data, 0);
  data[8] = version;
  data.writeUInt32LE(3600, 8 + 4);
  data.writeBigUInt64LE(4n, 8 + 8);
  DF1_FEED.toBuffer().copy(data, 8 + 80);
  return data;
};

const feedAccount = (state, id, version, df1Feed) => {
  const data = Buffer.alloc(8 + 1344 + 48 * 4);
  discriminator("Feed").copy(data, 0);
  data[8] = version;
  data[8 + 2] = 18;
  seed32(id).copy(data, 8 + 32);
  state.toBuffer().copy(data, 8 + 64);
  data.writeBigUInt64LE(4n, 8 + 112);
  if (df1Feed) df1Feed.toBuffer().copy(data, 8 + 208);
  return data;
};

const linkAccount = (id, version) => {
  const data = Buffer.alloc(8 + 40);
  discriminator("Df1Link").copy(data, 0);
  data[8] = version;
  seed32(id).copy(data, 16);
  return data;
};

const json = (address, data) => ({
  pubkey: address.toBase58(),
  account: {
    lamports: 1_000_000_000,
    data: [data.toString("base64"), "base64"],
    owner: PROGRAM_ID.toBase58(),
    executable: false,
    rentEpoch: 0,
  },
});

function writeFixtures(directory) {
  fs.mkdirSync(directory, { recursive: true });
  const validState = statePda(STORE_ID);
  const fixtures = [
    {
      address: statePda(WRONG_STATE_STORE_ID),
      name: "wrong-state.json",
      data: stateAccount(0),
    },
    {
      address: feedPda(validState, DATA_ID),
      name: "wrong-feed.json",
      data: feedAccount(validState, DATA_ID, 2),
    },
    {
      address: feedPda(validState, LINK_DATA_ID),
      name: "linked-feed.json",
      data: feedAccount(validState, LINK_DATA_ID, 1, DF1_FEED),
    },
    {
      address: linkPda(validState, DF1_FEED),
      name: "wrong-link.json",
      data: linkAccount(LINK_DATA_ID, 0),
    },
  ];
  for (const fixture of fixtures) {
    const file = path.join(directory, fixture.name);
    fs.writeFileSync(file, JSON.stringify(json(fixture.address, fixture.data)));
    console.log(`${fixture.address.toBase58()} ${file}`);
  }
}

if (require.main === module) {
  const directory = process.argv[2];
  if (!directory)
    throw new Error(
      "usage: node data-feeds-store-wrong-versions.cjs DIRECTORY"
    );
  writeFixtures(directory);
}

module.exports = {
  PROGRAM_ID,
  STORE_ID,
  WRONG_STATE_STORE_ID,
  DATA_ID,
  LINK_DATA_ID,
  DF1_FEED,
  statePda,
  feedPda,
  linkPda,
  writeFixtures,
};
