const https = require('https');
const fs = require('fs');

const artifact = JSON.parse(fs.readFileSync('./artifacts/contracts/BotChainProver.sol/BotChainProver.json', 'utf8'));
const bytecode = artifact.bytecode;
const deployer = "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266";

function rpcCall(method, params) {
  return new Promise((resolve, reject) => {
    const data = JSON.stringify({ jsonrpc: "2.0", method, params, id: 1 });
    const req = https.request({
      hostname: 'rpc.botchain.ai', port: 443, path: '/', method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Content-Length': data.length }
    }, res => {
      let resData = '';
      res.on('data', d => { resData += d; });
      res.on('end', () => {
        try { resolve(JSON.parse(resData).result); }
        catch (e) { reject(e); }
      });
    });
    req.on('error', reject);
    req.write(data);
    req.end();
  });
}

async function main() {
  try {
    const gasLimitHex = await rpcCall("eth_estimateGas", [{ from: deployer, data: bytecode }]);
    const gasPriceHex = await rpcCall("eth_gasPrice", []);
    
    const gasLimit = parseInt(gasLimitHex, 16);
    const gasPrice = parseInt(gasPriceHex, 16);
    
    console.log(`Estimated Gas Limit: ${gasLimit}`);
    console.log(`Current Gas Price: ${gasPrice} wei`);
    
    const totalCostWei = BigInt(gasLimit) * BigInt(gasPrice);
    const totalCostBot = Number(totalCostWei) / 1e18;
    console.log(`\nTotal estimated deployment cost: ${totalCostBot} BOT`);
  } catch(e) {
    console.error("Error:", e);
  }
}

main();
