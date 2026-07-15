package uniswaptxparser

import (
	"fmt"
)

type swap02Method struct {
	name string
	mode byte
}

const (
	modeExactInSingle byte = iota
	modeExactIn
	modeExactOutSingle
	modeExactOut
	modeMulticall
	modeMulticallDeadline
	modeAux
)

var swap02Methods = map[[4]byte]swap02Method{
	selectorFor("exactInputSingle((address,address,uint24,address,uint256,uint256,uint160))"):  {"exactInputSingle", modeExactInSingle},
	selectorFor("exactInput((bytes,address,uint256,uint256))"):                                 {"exactInput", modeExactIn},
	selectorFor("exactOutputSingle((address,address,uint24,address,uint256,uint256,uint160))"): {"exactOutputSingle", modeExactOutSingle},
	selectorFor("exactOutput((bytes,address,uint256,uint256))"):                                {"exactOutput", modeExactOut},
	selectorFor("multicall(bytes[])"):                                                          {"multicall", modeMulticall}, selectorFor("multicall(uint256,bytes[])"): {"multicall", modeMulticallDeadline}, selectorFor("multicall(bytes32,bytes[])"): {"multicall", modeMulticall},
	selectorFor("unwrapWETH9(uint256,address)"): {"unwrapWETH9", modeAux}, selectorFor("unwrapWETH9WithFee(uint256,address,uint256,address)"): {"unwrapWETH9WithFee", modeAux}, selectorFor("sweepToken(address,uint256,address)"): {"sweepToken", modeAux}, selectorFor("sweepTokenWithFee(address,uint256,address,uint256,address)"): {"sweepTokenWithFee", modeAux}, selectorFor("refundETH()"): {"refundETH", modeAux}, selectorFor("selfPermit(address,uint256,uint256,uint8,bytes32,bytes32)"): {"selfPermit", modeAux}, selectorFor("selfPermitIfNecessary(address,uint256,uint256,uint8,bytes32,bytes32)"): {"selfPermitIfNecessary", modeAux},
}

func (p *Parser) parseSwap02(r *ParsedTransaction, data []byte, depth int) error {
	if depth > p.maxDepth {
		return ErrRecursionLimit
	}
	sel, e := selectorOf(data)
	if e != nil {
		return e
	}
	m, ok := swap02Methods[sel]
	if !ok {
		return ErrUnsupportedMethod
	}
	r.Method = m.name
	d := decoder{data[4:]}
	if m.mode == modeMulticall || m.mode == modeMulticallDeadline {
		head := 0
		if m.mode == modeMulticallDeadline {
			r.Deadline, e = d.uint(0)
			if e != nil {
				return e
			}
			head = 32
		}
		calls, e := d.bytesArray(head, 0)
		if e != nil {
			return e
		}
		op := Operation{Name: "multicall", RawInput: cloneBytes(data[4:])}
		for i, c := range calls {
			tmp := &ParsedTransaction{Complete: true}
			if e := p.parseSwap02(tmp, c, depth+1); e != nil {
				r.Complete = false
				r.Warnings = append(r.Warnings, ParseWarning{fmt.Sprintf("multicall[%d]", i), e.Error()})
				op.Children = append(op.Children, Operation{Index: i, Name: "unknown", RawInput: cloneBytes(c)})
				continue
			}
			for _, child := range tmp.Operations {
				child.Index = i
				op.Children = append(op.Children, child)
			}
		}
		r.Operations = []Operation{op}
		return nil
	}
	op := Operation{Index: 0, Name: m.name, RawInput: cloneBytes(data[4:])}
	if m.mode == modeAux {
		r.Operations = []Operation{op}
		return nil
	}
	s := &Swap{Protocol: ProtocolV3}
	var base int
	if m.mode == modeExactIn || m.mode == modeExactOut {
		base, e = d.offset(0)
		if e != nil {
			return e
		}
		raw, e := d.bytesAt(base, base)
		if e != nil {
			return e
		}
		s.Recipient, e = d.address(base + 32)
		if e != nil {
			return e
		}
		exactOut := m.mode == modeExactOut
		s.V3Hops, e = DecodeV3Path(raw, exactOut)
		if e != nil {
			return fmt.Errorf("%w: %v", ErrMalformedCalldata, e)
		}
		a, e := d.uint(base + 64)
		if e != nil {
			return e
		}
		b, e := d.uint(base + 96)
		if e != nil {
			return e
		}
		if exactOut {
			s.Kind = ExactOutput
			s.AmountOut = a
			s.AmountInMaximum = b
		} else {
			s.Kind = ExactInput
			s.AmountIn = a
			s.AmountOutMinimum = b
		}
	} else {
		tin, e := d.address(0)
		if e != nil {
			return e
		}
		tout, e := d.address(32)
		if e != nil {
			return e
		}
		fee, e := d.uint64(64)
		if e != nil || fee > 0xffffff {
			return ErrMalformedCalldata
		}
		s.Recipient, e = d.address(96)
		if e != nil {
			return e
		}
		s.V3Hops = []V3Hop{{tin, tout, uint32(fee)}}
		a, e := d.uint(128)
		if e != nil {
			return e
		}
		b, e := d.uint(160)
		if e != nil {
			return e
		}
		if m.mode == modeExactOutSingle {
			s.Kind = ExactOutput
			s.AmountOut = a
			s.AmountInMaximum = b
		} else {
			s.Kind = ExactInput
			s.AmountIn = a
			s.AmountOutMinimum = b
		}
	}
	op.Swap = s
	r.Operations = []Operation{op}
	return nil
}
