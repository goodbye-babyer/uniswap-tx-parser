package uniswaptxparser

type v2Method struct {
	name          string
	kind          SwapKind
	ethIn         bool
	exactArgs     bool
	feeOnTransfer bool
}

var v2Methods = map[[4]byte]v2Method{
	selectorFor("swapExactTokensForTokens(uint256,uint256,address[],address,uint256)"):                              {"swapExactTokensForTokens", ExactInput, false, true, false},
	selectorFor("swapTokensForExactTokens(uint256,uint256,address[],address,uint256)"):                              {"swapTokensForExactTokens", ExactOutput, false, true, false},
	selectorFor("swapExactETHForTokens(uint256,address[],address,uint256)"):                                         {"swapExactETHForTokens", ExactInput, true, false, false},
	selectorFor("swapTokensForExactETH(uint256,uint256,address[],address,uint256)"):                                 {"swapTokensForExactETH", ExactOutput, false, true, false},
	selectorFor("swapExactTokensForETH(uint256,uint256,address[],address,uint256)"):                                 {"swapExactTokensForETH", ExactInput, false, true, false},
	selectorFor("swapETHForExactTokens(uint256,address[],address,uint256)"):                                         {"swapETHForExactTokens", ExactOutput, true, false, false},
	selectorFor("swapExactTokensForTokensSupportingFeeOnTransferTokens(uint256,uint256,address[],address,uint256)"): {"swapExactTokensForTokensSupportingFeeOnTransferTokens", ExactInput, false, true, true},
	selectorFor("swapExactETHForTokensSupportingFeeOnTransferTokens(uint256,address[],address,uint256)"):            {"swapExactETHForTokensSupportingFeeOnTransferTokens", ExactInput, true, false, true},
	selectorFor("swapExactTokensForETHSupportingFeeOnTransferTokens(uint256,uint256,address[],address,uint256)"):    {"swapExactTokensForETHSupportingFeeOnTransferTokens", ExactInput, false, true, true},
}

func (p *Parser) parseV2(r *ParsedTransaction, data []byte) error {
	sel, e := selectorOf(data)
	if e != nil {
		return e
	}
	m, ok := v2Methods[sel]
	if !ok {
		return ErrUnsupportedMethod
	}
	d := decoder{data[4:]}
	s := &Swap{Protocol: ProtocolV2, Kind: m.kind, SupportsFeeOnTransfer: m.feeOnTransfer}
	var pathHead, recipientHead, deadlineHead int
	if m.ethIn {
		a, e := d.uint(0)
		if e != nil {
			return e
		}
		if m.kind == ExactInput {
			s.AmountIn = bigcopy(r.Value)
			s.AmountOutMinimum = a
		} else {
			s.AmountOut = a
			s.AmountInMaximum = bigcopy(r.Value)
		}
		pathHead = 32
		recipientHead = 64
		deadlineHead = 96
	} else {
		a, e := d.uint(0)
		if e != nil {
			return e
		}
		b, e := d.uint(32)
		if e != nil {
			return e
		}
		if m.kind == ExactInput {
			s.AmountIn = a
			s.AmountOutMinimum = b
		} else {
			s.AmountOut = a
			s.AmountInMaximum = b
		}
		pathHead = 64
		recipientHead = 96
		deadlineHead = 128
	}
	s.TokenPath, e = d.addressArray(pathHead, 0)
	if e != nil {
		return e
	}
	s.Recipient, e = d.address(recipientHead)
	if e != nil {
		return e
	}
	r.Deadline, e = d.uint(deadlineHead)
	if e != nil {
		return e
	}
	r.Method = m.name
	r.Operations = []Operation{{Index: 0, Name: m.name, Swap: s, RawInput: cloneBytes(data[4:])}}
	return nil
}

func (p *Parser) detect(sel [4]byte) (RouterDescriptor, bool) {
	if _, ok := universalMethods[sel]; ok {
		return RouterDescriptor{RouterUniversal, "unknown"}, true
	}
	if _, ok := swap02Methods[sel]; ok {
		return RouterDescriptor{RouterSwap02, "02"}, true
	}
	if _, ok := v2Methods[sel]; ok {
		return RouterDescriptor{RouterV2, "02"}, true
	}
	return RouterDescriptor{}, false
}
