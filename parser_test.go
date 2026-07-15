package uniswaptxparser

import (
	"bytes"
	"encoding/binary"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var testA = common.HexToAddress("0x0000000000000000000000000000000000000001")
var testB = common.HexToAddress("0x0000000000000000000000000000000000000002")
var testRecipient = common.HexToAddress("0x0000000000000000000000000000000000000003")
var testUniversalRouter = common.HexToAddress("0x0000000000000000000000000000000000000010")
var testSwapRouter02 = common.HexToAddress("0x0000000000000000000000000000000000000020")
var testV2Router02 = common.HexToAddress("0x0000000000000000000000000000000000000030")

func wnum(n uint64) []byte          { w := make([]byte, 32); binary.BigEndian.PutUint64(w[24:], n); return w }
func wbig(n *big.Int) []byte        { w := make([]byte, 32); n.FillBytes(w); return w }
func waddr(a common.Address) []byte { w := make([]byte, 32); copy(w[12:], a[:]); return w }
func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}
func bodyBytes(b []byte) []byte {
	out := cat(wnum(uint64(len(b))), b)
	return append(out, make([]byte, (32-len(b)%32)%32)...)
}
func bodyAddresses(a []common.Address) []byte {
	out := wnum(uint64(len(a)))
	for _, x := range a {
		out = append(out, waddr(x)...)
	}
	return out
}
func bodyBytesArray(items [][]byte) []byte {
	head := wnum(uint64(len(items)))
	off := 32 * len(items)
	var tails []byte
	for _, x := range items {
		head = append(head, wnum(uint64(off))...)
		b := bodyBytes(x)
		tails = append(tails, b...)
		off += len(b)
	}
	return append(head, tails...)
}
func dynamicPair(aBody, bBody []byte) []byte {
	return cat(wnum(64), wnum(uint64(64+len(aBody))), aBody, bBody)
}
func addSelector(sig string, args []byte) []byte { s := selectorFor(sig); return cat(s[:], args) }
func txTo(to common.Address, data []byte, value *big.Int) *types.Transaction {
	return types.NewTx(&types.LegacyTx{To: &to, Data: data, Value: value, Gas: 500000, GasPrice: big.NewInt(1)})
}

func universalCall(commands []byte, inputs [][]byte) []byte {
	return addSelector("execute(bytes,bytes[])", dynamicPair(bodyBytes(commands), bodyBytesArray(inputs)))
}
func universalV3Input(recipient common.Address, a, b *big.Int, path []byte, payer bool) []byte {
	pv := uint64(0)
	if payer {
		pv = 1
	}
	return cat(waddr(recipient), wbig(a), wbig(b), wnum(160), wnum(pv), bodyBytes(path))
}

func TestParseV2ExactInput(t *testing.T) {
	args := cat(wbig(big.NewInt(100)), wbig(big.NewInt(90)), wnum(160), waddr(testRecipient), wnum(123), bodyAddresses([]common.Address{testA, testB}))
	r, e := ParseTransaction(txTo(testV2Router02, addSelector("swapExactTokensForTokens(uint256,uint256,address[],address,uint256)", args), big.NewInt(0)))
	if e != nil {
		t.Fatal(e)
	}
	if r.Method != "swapExactTokensForTokens" || r.Operations[0].Swap.AmountIn.Uint64() != 100 {
		t.Fatalf("unexpected: %+v", r)
	}
}

func TestNewParserUsesConfiguredRouterAddress(t *testing.T) {
	args := cat(wbig(big.NewInt(100)), wbig(big.NewInt(90)), wnum(160), waddr(testRecipient), wnum(123), bodyAddresses([]common.Address{testA, testB}))
	data := addSelector("swapExactTokensForTokens(uint256,uint256,address[],address,uint256)", args)
	parser := NewParser(WithRouter(testV2Router02, RouterDescriptor{Kind: RouterV2, Version: "02"}))

	if _, err := parser.ParseTransaction(txTo(testV2Router02, data, big.NewInt(0))); err != nil {
		t.Fatalf("parse configured router: %v", err)
	}
	if _, err := parser.ParseTransaction(txTo(testSwapRouter02, data, big.NewInt(0))); err != ErrNotRouterTransaction {
		t.Fatalf("unconfigured router error = %v, want %v", err, ErrNotRouterTransaction)
	}
}

func TestParseUniversalV3ExactOutput(t *testing.T) {
	path := cat(testB.Bytes(), []byte{0, 11, 184}, testA.Bytes())
	data := universalCall([]byte{cmdV3ExactOut}, [][]byte{universalV3Input(testRecipient, big.NewInt(50), big.NewInt(80), path, true)})
	r, e := ParseTransaction(txTo(testUniversalRouter, data, big.NewInt(0)))
	if e != nil {
		t.Fatal(e)
	}
	s := r.Operations[0].Swap
	if s.Kind != ExactOutput || s.V3Hops[0].TokenIn != testA || s.V3Hops[0].TokenOut != testB {
		t.Fatalf("bad direction: %+v", s)
	}
}
func TestParseSwapRouter02ExactInputSingle(t *testing.T) {
	args := cat(waddr(testA), waddr(testB), wnum(3000), waddr(testRecipient), wnum(10), wnum(9), wnum(0))
	r, e := ParseTransaction(txTo(testSwapRouter02, addSelector("exactInputSingle((address,address,uint24,address,uint256,uint256,uint160))", args), big.NewInt(0)))
	if e != nil {
		t.Fatal(e)
	}
	if r.Operations[0].Swap.V3Hops[0].Fee != 3000 {
		t.Fatalf("bad result: %+v", r)
	}
}
func TestUnknownUniversalCommandIsPartial(t *testing.T) {
	r, e := ParseTransaction(txTo(testUniversalRouter, universalCall([]byte{0x7e}, [][]byte{{1}}), big.NewInt(0)))
	if e != nil {
		t.Fatal(e)
	}
	if r.Complete || len(r.Warnings) != 1 {
		t.Fatalf("expected partial: %+v", r)
	}
}
func TestParseUniversalV4ExactInputSingle(t *testing.T) {
	hook := []byte{1, 2}
	tuple := cat(waddr(testA), waddr(testB), wnum(3000), wnum(60), waddr(common.Address{}), wnum(1), wnum(100), wnum(90), wnum(288), bodyBytes(hook))
	actionParam := cat(wnum(32), tuple)
	v4 := dynamicPair(bodyBytes([]byte{0x06}), bodyBytesArray([][]byte{actionParam}))
	r, e := ParseTransaction(txTo(testUniversalRouter, universalCall([]byte{cmdV4Swap}, [][]byte{v4}), big.NewInt(0)))
	if e != nil {
		t.Fatal(e)
	}
	s := r.Operations[0].Swap
	if s.V4PoolKey == nil || s.V4PoolKey.Fee != 3000 || s.AmountIn.Uint64() != 100 || s.ZeroForOne == nil || !*s.ZeroForOne {
		t.Fatalf("bad v4: %+v", s)
	}
}
func TestDecodeV3PathRejectsBadLength(t *testing.T) {
	if _, e := DecodeV3Path(make([]byte, 42), false); e == nil {
		t.Fatal("expected error")
	}
}
func TestV3PathDoesNotMutateInput(t *testing.T) {
	p := cat(testB.Bytes(), []byte{0, 1, 244}, testA.Bytes())
	orig := cloneBytes(p)
	_, _ = DecodeV3Path(p, true)
	if !bytes.Equal(p, orig) {
		t.Fatal("input mutated")
	}
}
func FuzzParseTransaction(f *testing.F) {
	f.Add([]byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if x := recover(); x != nil {
				t.Fatalf("panic: %v", x)
			}
		}()
		_, _ = NewParser(WithSelectorOnly(true)).ParseTransaction(txTo(testUniversalRouter, data, big.NewInt(0)))
	})
}
func BenchmarkUniversalV3ExactIn(b *testing.B) {
	path := cat(testA.Bytes(), []byte{0, 11, 184}, testB.Bytes())
	tx := txTo(testUniversalRouter, universalCall([]byte{cmdV3ExactIn}, [][]byte{universalV3Input(testRecipient, big.NewInt(100), big.NewInt(90), path, true)}), big.NewInt(0))
	p := NewParser(WithRouter(testUniversalRouter, RouterDescriptor{Kind: RouterUniversal, Version: "test"}))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, e := p.ParseTransaction(tx)
		if e != nil || r == nil {
			b.Fatal(e)
		}
	}
}
