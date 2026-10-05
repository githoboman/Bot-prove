import os
import json
from dotenv import load_dotenv
from web3 import Web3

load_dotenv()

PRIVATE_KEY = os.getenv("PRIVATE_KEY")
if not PRIVATE_KEY:
    print("PRIVATE_KEY not found in .env")
    exit(1)

RPC_URL = "https://rpc.botchain.ai"
w3 = Web3(Web3.HTTPProvider(RPC_URL))

print("Attempting to connect without checking is_connected()...")

account = w3.eth.account.from_key(PRIVATE_KEY)
print(f"Deployer Address: {account.address}")

with open("artifacts/contracts/BotChainProver.sol/BotChainProver.json") as f:
    artifact = json.load(f)

bytecode = artifact["bytecode"]
abi = artifact["abi"]

BotChainProver = w3.eth.contract(abi=abi, bytecode=bytecode)

nonce = w3.eth.get_transaction_count(account.address)
print(f"Nonce: {nonce}")

transaction = BotChainProver.constructor().build_transaction({
    'chainId': 677,
    'gas': 500000,
    'gasPrice': w3.eth.gas_price,
    'nonce': nonce,
})

signed_txn = w3.eth.account.sign_transaction(transaction, private_key=PRIVATE_KEY)
try:
    print("Sending transaction...")
    tx_hash = w3.eth.send_raw_transaction(signed_txn.raw_transaction)
    print(f"Transaction Hash: {w3.to_hex(tx_hash)}")
    
    print("Waiting for receipt...")
    tx_receipt = w3.eth.wait_for_transaction_receipt(tx_hash)
    print(f"Contract Deployed to: {tx_receipt.contractAddress}")
except Exception as e:
    print(f"Error deploying: {e}")
