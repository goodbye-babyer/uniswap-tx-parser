package uniswaptxparser

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type RouterKind string

const (
	RouterUniversal RouterKind = "universal-router"
	RouterSwap02    RouterKind = "swap-router-02"
	RouterV2        RouterKind = "v2-router-02"
)

type Protocol string

const (
	ProtocolV2 Protocol = "v2"
	ProtocolV3 Protocol = "v3"
	ProtocolV4 Protocol = "v4"
)

type SwapKind string

const (
	ExactInput  SwapKind = "exact-input"
	ExactOutput SwapKind = "exact-output"
)

type RouterDescriptor struct {
	Kind    RouterKind
	Version string
}
type V3Hop struct {
	TokenIn, TokenOut common.Address
	Fee               uint32
}
type V4Action struct {
	Code     byte
	Name     string
	RawInput []byte
}
type V4PoolKey struct {
	Currency0, Currency1 common.Address
	Fee                  uint32
	TickSpacing          int32
	Hooks                common.Address
}
type V4Hop struct {
	IntermediateCurrency common.Address
	Fee                  uint32
	TickSpacing          int32
	Hooks                common.Address
	HookData             []byte
}

type Swap struct {
	Protocol                                               Protocol
	Kind                                                   SwapKind
	Recipient                                              common.Address
	PayerIsUser                                            *bool
	TokenPath                                              []common.Address
	V3Hops                                                 []V3Hop
	V4Actions                                              []V4Action
	V4PoolKey                                              *V4PoolKey
	V4Path                                                 []V4Hop
	ZeroForOne                                             *bool
	HookData                                               []byte
	AmountIn, AmountInMaximum, AmountOut, AmountOutMinimum *big.Int
	SupportsFeeOnTransfer                                  bool
	AllowRevert                                            bool
}

type Operation struct {
	Index       int
	Name        string
	Command     byte
	AllowRevert bool
	Swap        *Swap
	Children    []Operation
	RawInput    []byte
}

type ParseWarning struct{ Path, Message string }
type ParsedTransaction struct {
	Hash       common.Hash
	To         *common.Address
	Value      *big.Int
	Nonce      uint64
	Router     RouterDescriptor
	Method     string
	Deadline   *big.Int
	Operations []Operation
	Complete   bool
	Warnings   []ParseWarning
}
