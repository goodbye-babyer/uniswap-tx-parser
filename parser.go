package uniswaptxparser

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type Option func(*Parser)
type Parser struct {
	routers      map[common.Address]RouterDescriptor
	selectorOnly bool
	maxDepth     int
}

func NewParser(opts ...Option) *Parser {
	p := &Parser{routers: make(map[common.Address]RouterDescriptor), maxDepth: 8}
	for _, o := range opts {
		o(p)
	}
	return p
}
func WithRouter(addr common.Address, d RouterDescriptor) Option {
	return func(p *Parser) { p.routers[addr] = d }
}
func WithSelectorOnly(v bool) Option { return func(p *Parser) { p.selectorOnly = v } }
func WithMaxDepth(n int) Option {
	return func(p *Parser) {
		if n > 0 {
			p.maxDepth = n
		}
	}
}
func ParseTransaction(tx *types.Transaction) (*ParsedTransaction, error) {
	return NewParser(WithSelectorOnly(true)).ParseTransaction(tx)
}

func (p *Parser) ParseTransaction(tx *types.Transaction) (*ParsedTransaction, error) {
	if tx == nil || tx.To() == nil {
		return nil, ErrNotRouterTransaction
	}
	d, ok := p.routers[*tx.To()]
	if !ok && !p.selectorOnly {
		return nil, ErrNotRouterTransaction
	}
	data := tx.Data()
	sel, e := selectorOf(data)
	if e != nil {
		return nil, e
	}
	if !ok {
		var found bool
		d, found = p.detect(sel)
		if !found {
			return nil, ErrUnsupportedMethod
		}
	}
	r := &ParsedTransaction{Hash: tx.Hash(), To: tx.To(), Value: new(big.Int).Set(tx.Value()), Nonce: tx.Nonce(), Router: d, Complete: true}
	var err error
	switch d.Kind {
	case RouterUniversal:
		err = p.parseUniversal(r, data, 0)
	case RouterSwap02:
		err = p.parseSwap02(r, data, 0)
	case RouterV2:
		err = p.parseV2(r, data)
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

func cloneBytes(b []byte) []byte { return append([]byte(nil), b...) }
func boolptr(v bool) *bool       { return &v }
func bigcopy(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}
