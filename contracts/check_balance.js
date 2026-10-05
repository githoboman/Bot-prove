const https = require('https');

const data = JSON.stringify({
  jsonrpc: "2.0",
  method: "eth_getBalance",
  params: ["0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266", "latest"],
  id: 1
});

const options = {
  hostname: 'rpc.botchain.ai',
  port: 443,
  path: '/',
  method: 'POST',
  headers: {
    'Content-Type': 'application/json',
    'Content-Length': data.length
  }
};

const req = https.request(options, res => {
  let resData = '';
  res.on('data', d => { resData += d; });
  res.on('end', () => {
    try {
      const parsed = JSON.parse(resData);
      const balance = parseInt(parsed.result, 16);
      console.log(`Balance: ${balance} wei (${balance / 1e18} BOT)`);
    } catch (e) {
      console.log("Error parsing response:", resData);
    }
  });
});

req.on('error', error => {
  console.error('Request error:', error);
});

req.write(data);
req.end();
