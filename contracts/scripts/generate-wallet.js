const { ethers } = require("ethers");
const fs = require("fs");

function main() {
    const wallet = ethers.Wallet.createRandom();
    console.log("Generated Deployer Address:", wallet.address);
    console.log("Private Key:", wallet.privateKey);
    
    // Save to .env
    const envContent = `PRIVATE_KEY=${wallet.privateKey}\nDEPLOYER_ADDRESS=${wallet.address}\n`;
    fs.writeFileSync('.env', envContent, { flag: 'a' });
    console.log("Saved to .env. Please do not share this file!");
}

main();
