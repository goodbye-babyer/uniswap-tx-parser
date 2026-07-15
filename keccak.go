package uniswaptxparser

import "github.com/ethereum/go-ethereum/crypto"

func keccak(b []byte) []byte { return crypto.Keccak256(b) }
