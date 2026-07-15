# uniswap-tx-parser

Standalone Go library for decoding Uniswap router calls from a single
`*types.Transaction`. It does not connect to RPC or the Arbitrum sequencer feed and does
not depend on Nitro.

The parser does not use `reflect` or `go-ethereum/accounts/abi`. Selectors, ABI words,
dynamic offsets, tuples and arrays are decoded directly with bounds checks.

Supported entry points:

- Universal Router 2.0 and 2.1.1 (V2, V3 and V4 swap commands, nested sub-plans)
- SwapRouter02 (V3 swaps and nested multicalls)
- UniswapV2Router02 swaps, including fee-on-transfer variants

## Usage

```go
parsed, err := uniswaptxparser.ParseTransaction(tx)
if errors.Is(err, uniswaptxparser.ErrNotRouterTransaction) {
    // Ignore transactions sent to other contracts.
}
for _, operation := range parsed.Operations {
    if operation.Swap != nil {
        // Consume the normalized swap intent.
    }
}
```

The default registry contains the official Arbitrum One router addresses. Custom router
deployments can be registered explicitly:

```go
parser := uniswaptxparser.NewParser(
    uniswaptxparser.WithRouter(address, uniswaptxparser.RouterDescriptor{
        Kind: uniswaptxparser.RouterUniversal,
        Version: "custom",
    }),
)
```

Unknown Universal Router commands produce a partial result (`Complete == false`) with a
warning and preserved raw input. Malformed top-level calldata returns an error.

## Development

```sh
go test ./...
go test -run '^$' -bench . -benchmem ./...
go test -fuzz FuzzParseTransaction
go vet ./...
```

## Robinhood Chain fixtures

Capture all transactions sent to the configured Robinhood Chain routers in an inclusive
block range, then run the offline decoder test:

```sh
FROM_BLOCK=123 TO_BLOCK=456 go test -run TestCaptureRobinhoodRouterTransactions -v
go test -run TestDecodeRobinhoodRouterFixture -v
```

The WebSocket endpoint defaults to `ws://127.0.0.1:8748` and can be overridden
with `RPC_URL`. The generated fixture is
`testdata/robinhood_router_transactions.json`.

To fetch and decode one transaction directly by hash:

```sh
TX_HASH=0x... go test -run TestDecodeTransactionByHash -v
```

This test also uses `RPC_URL` when it is set and otherwise uses the default
endpoint above. It enables selector-based router detection so transactions sent to a
compatible custom router can be decoded as well.
