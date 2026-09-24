require("@nomicfoundation/hardhat-toolbox");

const PRIVATE_KEY = "0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80";

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
