// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract BotChainProver {
    event ProofStored(bytes32 indexed proofHash, string model, uint256 timestamp);

    function storeProof(bytes32 proofHash, string memory model) external {
        emit ProofStored(proofHash, model, block.timestamp);
    }
}
