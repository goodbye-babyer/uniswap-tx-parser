package uniswaptxparser

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

const defaultRPCURL = "ws://127.0.0.1:8748"
const routerFixturePath = "testdata/robinhood_router_transactions.json"

var robinhoodRouterAddresses = map[common.Address]struct{}{
	common.HexToAddress("0x8876789976decbfcbbbe364623c63652db8c0904"): {},
	common.HexToAddress("0xcaf681a66d020601342297493863e78c959e5cb2"): {},
	common.HexToAddress("0x89e5db8b5aa49aa85ac63f691524311aeb649eba"): {},
}

type routerFixture struct {
	RPCURL    string              `json:"rpcUrl"`
	FromBlock uint64              `json:"fromBlock"`
	ToBlock   uint64              `json:"toBlock"`
	CreatedAt time.Time           `json:"createdAt"`
	Txs       []routerFixtureItem `json:"transactions"`
}

type routerFixtureItem struct {
	BlockNumber uint64         `json:"blockNumber"`
	BlockHash   common.Hash    `json:"blockHash"`
	Index       uint           `json:"transactionIndex"`
	Hash        common.Hash    `json:"hash"`
	To          common.Address `json:"to"`
	Raw         string         `json:"raw"`
}

// TestCaptureRobinhoodRouterTransactions is an explicit integration test. Set
// FROM_BLOCK and TO_BLOCK to capture every transaction sent to a configured router.
func TestCaptureRobinhoodRouterTransactions(t *testing.T) {
	fromText, toText := os.Getenv("FROM_BLOCK"), os.Getenv("TO_BLOCK")
	if fromText == "" || toText == "" {
		t.Skip("set FROM_BLOCK and TO_BLOCK to run the RPC fixture capture")
	}
	from, err := strconv.ParseUint(fromText, 0, 64)
	if err != nil {
		t.Fatalf("invalid FROM_BLOCK: %v", err)
	}
	to, err := strconv.ParseUint(toText, 0, 64)
	if err != nil {
		t.Fatalf("invalid TO_BLOCK: %v", err)
	}
	if from > to {
		t.Fatalf("FROM_BLOCK %d exceeds TO_BLOCK %d", from, to)
	}

	rpcURL := os.Getenv("RPC_URL")
	if rpcURL == "" {
		rpcURL = defaultRPCURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial Robinhood RPC: %v", err)
	}
	defer client.Close()

	fixture := routerFixture{RPCURL: rpcURL, FromBlock: from, ToBlock: to, CreatedAt: time.Now().UTC()}
	for number := from; ; number++ {
		block, err := client.BlockByNumber(ctx, new(big.Int).SetUint64(number))
		if err != nil {
			t.Fatalf("fetch block %d: %v", number, err)
		}
		for index, tx := range block.Transactions() {
			if tx.To() == nil {
				continue
			}
			if _, wanted := robinhoodRouterAddresses[*tx.To()]; !wanted {
				continue
			}
			raw, err := tx.MarshalBinary()
			if err != nil {
				t.Fatalf("marshal tx %s: %v", tx.Hash(), err)
			}
			fixture.Txs = append(fixture.Txs, routerFixtureItem{BlockNumber: number, BlockHash: block.Hash(), Index: uint(index), Hash: tx.Hash(), To: *tx.To(), Raw: "0x" + hex.EncodeToString(raw)})
		}
		if number == to {
			break
		}
	}

	if err := os.MkdirAll(filepath.Dir(routerFixturePath), 0o755); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tmp := routerFixturePath + ".tmp"
	if err := os.WriteFile(tmp, append(encoded, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, routerFixturePath); err != nil {
		t.Fatal(err)
	}
	t.Logf("saved %d router transactions from blocks %d..%d to %s", len(fixture.Txs), from, to, routerFixturePath)
}

func TestDecodeRobinhoodRouterFixture(t *testing.T) {
	encoded, err := os.ReadFile(routerFixturePath)
	if os.IsNotExist(err) {
		t.Skipf("fixture is absent; run TestCaptureRobinhoodRouterTransactions with FROM_BLOCK and TO_BLOCK first")
	}
	if err != nil {
		t.Fatal(err)
	}
	var fixture routerFixture
	if err := json.Unmarshal(encoded, &fixture); err != nil {
		t.Fatalf("decode fixture JSON: %v", err)
	}
	if len(fixture.Txs) == 0 {
		t.Fatal("fixture contains no matching router transactions")
	}
	parser := NewParser(WithSelectorOnly(true))
	for _, item := range fixture.Txs {
		item := item
		t.Run(fmt.Sprintf("block_%d_tx_%d_%s", item.BlockNumber, item.Index, item.Hash.Hex()), func(t *testing.T) {
			raw, err := hex.DecodeString(strings.TrimPrefix(item.Raw, "0x"))
			if err != nil {
				t.Fatalf("decode raw transaction: %v", err)
			}
			var tx types.Transaction
			if err := tx.UnmarshalBinary(raw); err != nil {
				t.Fatalf("unmarshal raw transaction: %v", err)
			}
			if tx.Hash() != item.Hash {
				t.Fatalf("hash mismatch: got %s want %s", tx.Hash(), item.Hash)
			}
			if tx.To() == nil || *tx.To() != item.To {
				t.Fatalf("recipient mismatch: got %v want %s", tx.To(), item.To)
			}
			parsed, err := parser.ParseTransaction(&tx)
			if err != nil {
				t.Fatalf("parse router transaction: %v (selector 0x%x)", err, tx.Data()[:min(4, len(tx.Data()))])
			}
			if len(parsed.Operations) == 0 {
				t.Fatal("decoder returned no operations")
			}
		})
	}
}

// TestDecodeTransactionByHash fetches one transaction from RPC and runs it
// through the decoder. It is intentionally opt-in because it requires a live
// RPC endpoint.
func TestDecodeTransactionByHash(t *testing.T) {
	txHashText := os.Getenv("TX_HASH")
	if txHashText == "" {
		t.Skip("set TX_HASH to run the RPC decoder test")
	}
	hashBytes, err := hex.DecodeString(strings.TrimPrefix(txHashText, "0x"))
	if err != nil || len(hashBytes) != common.HashLength {
		t.Fatalf("invalid TX_HASH %q: expected a 32-byte hex transaction hash", txHashText)
	}
	wantHash := common.BytesToHash(hashBytes)

	rpcURL := os.Getenv("RPC_URL")
	if rpcURL == "" {
		rpcURL = defaultRPCURL
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial RPC: %v", err)
	}
	defer client.Close()

	tx, pending, err := client.TransactionByHash(ctx, wantHash)
	if err != nil {
		t.Fatalf("fetch transaction %s: %v", wantHash, err)
	}
	if tx.Hash() != wantHash {
		t.Fatalf("hash mismatch: got %s want %s", tx.Hash(), wantHash)
	}
	if tx.To() == nil {
		t.Fatal("transaction creates a contract and cannot be decoded as a router call")
	}

	parsed, err := NewParser(WithSelectorOnly(true)).ParseTransaction(tx)
	if err != nil {
		t.Fatalf("decode transaction %s (to %s, selector 0x%x): %v", tx.Hash(), tx.To(), tx.Data()[:min(4, len(tx.Data()))], err)
	}
	if len(parsed.Operations) == 0 {
		t.Fatal("decoder returned no operations")
	}
	encoded, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		t.Fatalf("format decoded transaction: %v", err)
	}
	t.Logf("pending=%t\ndecoded transaction:\n%s", pending, encoded)
}
