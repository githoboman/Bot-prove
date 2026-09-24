const hre = require("hardhat");

async function main() {
  const BotChainProver = await hre.ethers.getContractFactory("BotChainProver");
  console.log("Deploying BotChainProver...");
  const prover = await BotChainProver.deploy();
  await prover.waitForDeployment();
  console.log("BotChainProver deployed to:", prover.target);
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
