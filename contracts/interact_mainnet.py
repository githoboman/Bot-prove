import os
import json
import time
from dotenv import load_dotenv
from web3 import Web3

load_dotenv()

PRIVATE_KEY = os.getenv("PRIVATE_KEY")
if not PRIVATE_KEY:
    print("Error: PRIVATE_KEY not found in .env")
    exit(1)

RPC_URL = "https://rpc.botchain.ai"
w3 = Web3(Web3.HTTPProvider(RPC_URL))
chain_id = 677

deployer = w3.eth.account.from_key(PRIVATE_KEY)
contract_address = w3.to_checksum_address("0x601794aFcE3443668f0280ACA1c2e522739478f7")

print(f"Deployer Address: {deployer.address}")
deployer_balance = w3.eth.get_balance(deployer.address)
print(f"Deployer Balance: {w3.from_wei(deployer_balance, 'ether')} BOT ({deployer_balance} wei)")

with open("artifacts/contracts/BotChainProver.sol/BotChainProver.json") as f:
    artifact = json.load(f)

prover_contract = w3.eth.contract(address=contract_address, abi=artifact["abi"])

# Define 7 distinct AI Agent interactions
agent_interactions = [
    {
        "agent_name": "ZK Sentiment Analysis Agent",
        "model": "zk-sentiment-analysis-v1",
        "tag": "inference_zk_sentiment_botchain_mainnet_1"
    },
    {
        "agent_name": "DeepSeek R1 Reasoning Verifier",
        "model": "deepseek-r1-math-verifier",
        "tag": "inference_deepseek_r1_reasoning_2"
    },
    {
        "agent_name": "DeFi Risk Assessment Bot",
        "model": "defi-risk-scoring-v2",
        "tag": "inference_defi_risk_scoring_3"
    },
    {
        "agent_name": "ZK Portfolio Rebalancer",
        "model": "zk-portfolio-optimizer",
        "tag": "inference_portfolio_rebalancer_4"
    },
    {
        "agent_name": "Autonomous Liquidity Manager",
        "model": "autonomous-liquidity-v1",
        "tag": "inference_liquidity_provision_5"
    },
    {
        "agent_name": "ZK Agent Reputation Scorer",
        "model": "agent-reputation-scorer-v1",
        "tag": "inference_agent_reputation_attestation_6"
    },
    {
        "agent_name": "ZK Fraud Detection Engine",
        "model": "zk-fraud-detection-v3",
        "tag": "inference_fraud_detection_anomaly_7"
    }
]

# Amount to fund each agent wallet for gas (0.0016 BOT = 1.6e15 wei)
# Gas cost for storeProof is ~50k gas * 20 Gwei = ~0.001 BOT
FUND_AMOUNT = w3.to_wei(0.0016, "ether")
gas_price = w3.eth.gas_price

print(f"\n--- STEP 1: Generating 7 Unique Wallets & Funding Each with {w3.from_wei(FUND_AMOUNT, 'ether')} BOT ---")
wallets = []
deployer_nonce = w3.eth.get_transaction_count(deployer.address)

for i, interaction in enumerate(agent_interactions):
    wallet = w3.eth.account.create(f"agent_entropy_bot_prove_{i}_{time.time()}")
    wallets.append(wallet)
    print(f"[{i+1}/7] Created Wallet: {wallet.address} for '{interaction['agent_name']}'")

    # Send funds from deployer to agent wallet
    fund_tx = {
        'to': wallet.address,
        'value': FUND_AMOUNT,
        'gas': 21000,
        'gasPrice': gas_price,
        'nonce': deployer_nonce,
        'chainId': chain_id
    }
    deployer_nonce += 1
    signed_fund_tx = w3.eth.account.sign_transaction(fund_tx, private_key=PRIVATE_KEY)
    tx_hash = w3.eth.send_raw_transaction(signed_fund_tx.raw_transaction)
    print(f"    Funding Tx Sent: {w3.to_hex(tx_hash)}")
    
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=60)
    print(f"    Fund Confirmed in Block {receipt.blockNumber} (Status: {receipt.status})")
    time.sleep(1)

print("\n--- STEP 2: Executing Smart Contract Interactions (storeProof) from each unique wallet ---")
results = []

for i, (wallet, interaction) in enumerate(zip(wallets, agent_interactions)):
    proof_hash = w3.keccak(text=interaction["tag"])
    model_name = interaction["model"]
    
    # Check wallet balance
    agent_bal = w3.eth.get_balance(wallet.address)
    print(f"\n[{i+1}/7] Interacting as {wallet.address} (Bal: {w3.from_wei(agent_bal, 'ether')} BOT)")
    print(f"    Model: {model_name}")
    print(f"    Proof Hash: {w3.to_hex(proof_hash)}")

    tx_data = prover_contract.functions.storeProof(proof_hash, model_name).build_transaction({
        'chainId': chain_id,
        'from': wallet.address,
        'gas': 80000,
        'gasPrice': gas_price,
        'nonce': 0
    })

    signed_interaction = w3.eth.account.sign_transaction(tx_data, private_key=wallet.key)
    interaction_tx_hash = w3.eth.send_raw_transaction(signed_interaction.raw_transaction)
    interaction_tx_hex = w3.to_hex(interaction_tx_hash)
    print(f"    Smart Contract Call Sent: {interaction_tx_hex}")

    receipt = w3.eth.wait_for_transaction_receipt(interaction_tx_hash, timeout=60)
    print(f"    Confirmed in Block {receipt.blockNumber} (Status: {'SUCCESS' if receipt.status == 1 else 'FAILED'}, Gas Used: {receipt.gasUsed})")

    results.append({
        "agent_name": interaction["agent_name"],
        "wallet_address": wallet.address,
        "contract_address": contract_address,
        "function": "storeProof(bytes32,string)",
        "model": model_name,
        "proof_hash": w3.to_hex(proof_hash),
        "tx_hash": interaction_tx_hex,
        "block_number": receipt.blockNumber,
        "status": "SUCCESS" if receipt.status == 1 else "FAILED"
    })
    time.sleep(1)

# Write summary to JSON
with open("onchain_interactions_mainnet.json", "w") as f:
    json.dump({
        "network": "Botchain Mainnet (Chain ID 677)",
        "contract_address": contract_address,
        "total_wallets": len(results),
        "total_transactions": len(results),
        "interactions": results
    }, f, indent=2)

print("\n--- ALL 7 ON-CHAIN SMART CONTRACT TRANSACTIONS COMPLETE ---")
print(f"Total Unique Wallets: {len(results)}")
print(f"Total Contract Transactions: {len(results)}")
