require("@nomicfoundation/hardhat-toolbox");
const fs = require('fs');
let PRIVATE_KEY = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80";
try {
  const envFile = fs.readFileSync('.env', 'utf8');
  const match = envFile.match(/PRIVATE_KEY=(0x[a-fA-F0-9]{64})/);
  if (match) PRIVATE_KEY = match[1];
} catch (e) {}

/** @type import('hardhat/config').HardhatUserConfig */
module.exports = {
  solidity: "0.8.20",
  networks: {
    botchain: {
      url: "https://rpc.botchain.ai",
      chainId: 677,
      accounts: [PRIVATE_KEY],
    },
    botchainTestnet: {
      url: "https://rpc.bohr.life",
      chainId: 968,
      accounts: [PRIVATE_KEY],
    }
  }
};
