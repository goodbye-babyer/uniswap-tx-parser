package uniswaptxparser

import (
	"fmt"
)

type universalMethod struct {
	name     string
	deadline bool
}

var universalMethods = map[[4]byte]universalMethod{selectorFor("execute(bytes,bytes[])"): {"execute", false}, selectorFor("execute(bytes,bytes[],uint256)"): {"execute", true}}

const (
	cmdV3ExactIn            byte = 0x00
	cmdV3ExactOut           byte = 0x01
	cmdPermit2Transfer      byte = 0x02
	cmdPermit2BatchPermit   byte = 0x03
	cmdSweep                byte = 0x04
	cmdTransfer             byte = 0x05
	cmdPayPortion           byte = 0x06
	cmdV2ExactIn            byte = 0x08
	cmdV2ExactOut           byte = 0x09
	cmdPermit2Permit        byte = 0x0a
	cmdWrapETH              byte = 0x0b
	cmdUnwrapWETH           byte = 0x0c
	cmdPermit2BatchTransfer byte = 0x0d
	cmdBalanceCheck         byte = 0x0e
	cmdV4Swap               byte = 0x10
	cmdSubPlan              byte = 0x21
)

var commandNames = map[byte]string{cmdV3ExactIn: "V3_SWAP_EXACT_IN", cmdV3ExactOut: "V3_SWAP_EXACT_OUT", cmdPermit2Transfer: "PERMIT2_TRANSFER_FROM", cmdPermit2BatchPermit: "PERMIT2_PERMIT_BATCH", cmdSweep: "SWEEP", cmdTransfer: "TRANSFER", cmdPayPortion: "PAY_PORTION", cmdV2ExactIn: "V2_SWAP_EXACT_IN", cmdV2ExactOut: "V2_SWAP_EXACT_OUT", cmdPermit2Permit: "PERMIT2_PERMIT", cmdWrapETH: "WRAP_ETH", cmdUnwrapWETH: "UNWRAP_WETH", cmdPermit2BatchTransfer: "PERMIT2_TRANSFER_FROM_BATCH", cmdBalanceCheck: "BALANCE_CHECK_ERC20", cmdV4Swap: "V4_SWAP", cmdSubPlan: "EXECUTE_SUB_PLAN"}

func (p *Parser) parseUniversal(r *ParsedTransaction, data []byte, depth int) error {
	if depth > p.maxDepth {
		return ErrRecursionLimit
	}
	sel, e := selectorOf(data)
	if e != nil {
		return e
	}
	m, ok := universalMethods[sel]
	if !ok {
		return ErrUnsupportedMethod
	}
	d := decoder{data[4:]}
	commands, e := d.bytesAt(0, 0)
	if e != nil {
		return e
	}
	inputs, e := d.bytesArray(32, 0)
	if e != nil {
		return e
	}
	if m.deadline {
		r.Deadline, e = d.uint(64)
		if e != nil {
			return e
		}
	}
	if len(commands) != len(inputs) {
		return fmt.Errorf("%w: %d commands for %d inputs", ErrMalformedCalldata, len(commands), len(inputs))
	}
	r.Method = m.name
	r.Operations, r.Warnings = p.parseCommands(commands, inputs, depth)
	if len(r.Warnings) > 0 {
		r.Complete = false
	}
	return nil
}

func (p *Parser) parseCommands(commands []byte, inputs [][]byte, depth int) ([]Operation, []ParseWarning) {
	ops := make([]Operation, 0, len(commands))
	var warns []ParseWarning
	for i, raw := range commands {
		typ := raw & 0x7f
		name, known := commandNames[typ]
		if !known {
			name = "UNKNOWN"
		}
		op := Operation{Index: i, Name: name, Command: typ, AllowRevert: raw&0x80 != 0, RawInput: cloneBytes(inputs[i])}
		var e error
		switch typ {
		case cmdV2ExactIn, cmdV2ExactOut:
			e = parseUniversalV2(&op, typ, inputs[i])
		case cmdV3ExactIn, cmdV3ExactOut:
			e = parseUniversalV3(&op, typ, inputs[i])
		case cmdV4Swap:
			e = parseV4(&op, inputs[i])
		case cmdSubPlan:
			if depth >= p.maxDepth {
				e = ErrRecursionLimit
				break
			}
			d := decoder{inputs[i]}
			sub, e1 := d.bytesAt(0, 0)
			if e1 != nil {
				e = e1
				break
			}
			subInputs, e1 := d.bytesArray(32, 0)
			if e1 != nil {
				e = e1
				break
			}
			if len(sub) != len(subInputs) {
				e = ErrMalformedCalldata
				break
			}
			op.Children, warns = p.parseCommands(sub, subInputs, depth+1)
		default:
			if !known {
				e = fmt.Errorf("unknown command 0x%02x", typ)
			}
		}
		if e != nil {
			warns = append(warns, ParseWarning{fmt.Sprintf("commands[%d]", i), e.Error()})
		}
		ops = append(ops, op)
	}
	return ops, warns
}

func parseUniversalV2(op *Operation, typ byte, input []byte) error {
	d := decoder{input}
	recipient, e := d.address(0)
	if e != nil {
		return e
	}
	a, e := d.uint(32)
	if e != nil {
		return e
	}
	b, e := d.uint(64)
	if e != nil {
		return e
	}
	path, e := d.addressArray(96, 0)
	if e != nil {
		return e
	}
	payer, e := d.boolean(128)
	if e != nil {
		return e
	}
	s := &Swap{Protocol: ProtocolV2, Recipient: recipient, TokenPath: path, PayerIsUser: boolptr(payer), AllowRevert: op.AllowRevert}
	if typ == cmdV2ExactIn {
		s.Kind = ExactInput
		s.AmountIn = a
		s.AmountOutMinimum = b
	} else {
		s.Kind = ExactOutput
		s.AmountOut = a
		s.AmountInMaximum = b
	}
	op.Swap = s
	return nil
}
func parseUniversalV3(op *Operation, typ byte, input []byte) error {
	d := decoder{input}
	recipient, e := d.address(0)
	if e != nil {
		return e
	}
	a, e := d.uint(32)
	if e != nil {
		return e
	}
	b, e := d.uint(64)
	if e != nil {
		return e
	}
	path, e := d.bytesAt(96, 0)
	if e != nil {
		return e
	}
	payer, e := d.boolean(128)
	if e != nil {
		return e
	}
	exactOut := typ == cmdV3ExactOut
	h, e := DecodeV3Path(path, exactOut)
	if e != nil {
		return e
	}
	s := &Swap{Protocol: ProtocolV3, Recipient: recipient, V3Hops: h, PayerIsUser: boolptr(payer), AllowRevert: op.AllowRevert}
	if exactOut {
		s.Kind = ExactOutput
		s.AmountOut = a
		s.AmountInMaximum = b
	} else {
		s.Kind = ExactInput
		s.AmountIn = a
		s.AmountOutMinimum = b
	}
	op.Swap = s
	return nil
}

var v4ActionNames = map[byte]string{0x06: "SWAP_EXACT_IN_SINGLE", 0x07: "SWAP_EXACT_IN", 0x08: "SWAP_EXACT_OUT_SINGLE", 0x09: "SWAP_EXACT_OUT", 0x0b: "SETTLE", 0x0c: "SETTLE_ALL", 0x0d: "SETTLE_PAIR", 0x0e: "TAKE", 0x0f: "TAKE_ALL", 0x10: "TAKE_PORTION", 0x11: "TAKE_PAIR", 0x12: "CLOSE_CURRENCY", 0x13: "CLEAR_OR_TAKE", 0x14: "SWEEP"}

func parseV4(op *Operation, input []byte) error {
	d := decoder{input}
	actions, e := d.bytesAt(0, 0)
	if e != nil {
		return e
	}
	params, e := d.bytesArray(32, 0)
	if e != nil {
		return e
	}
	if len(actions) != len(params) {
		return fmt.Errorf("%w: %d actions for %d params", ErrMalformedCalldata, len(actions), len(params))
	}
	s := &Swap{Protocol: ProtocolV4, AllowRevert: op.AllowRevert}
	for i, a := range actions {
		name := v4ActionNames[a]
		if name == "" {
			name = "UNKNOWN"
		}
		s.V4Actions = append(s.V4Actions, V4Action{a, name, cloneBytes(params[i])})
		switch a {
		case 0x06:
			s.Kind = ExactInput
			e = decodeV4Single(s, params[i], false)
		case 0x08:
			s.Kind = ExactOutput
			e = decodeV4Single(s, params[i], true)
		case 0x07:
			s.Kind = ExactInput
			e = decodeV4Multi(s, params[i], false)
		case 0x09:
			s.Kind = ExactOutput
			e = decodeV4Multi(s, params[i], true)
		}
		if e != nil {
			return e
		}
	}
	op.Swap = s
	return nil
}

func decodeV4Single(s *Swap, input []byte, exactOut bool) error {
	d := decoder{input}
	base, e := d.offset(0)
	if e != nil {
		return e
	}
	c0, e := d.address(base)
	if e != nil {
		return e
	}
	c1, e := d.address(base + 32)
	if e != nil {
		return e
	}
	fee, e := d.uint64(base + 64)
	if e != nil || fee > 0xffffff {
		return ErrMalformedCalldata
	}
	tick, e := d.signed24(base + 96)
	if e != nil {
		return e
	}
	hooks, e := d.address(base + 128)
	if e != nil {
		return e
	}
	zero, e := d.boolean(base + 160)
	if e != nil {
		return e
	}
	a, e := d.uint(base + 192)
	if e != nil {
		return e
	}
	b, e := d.uint(base + 224)
	if e != nil {
		return e
	}
	hook, e := d.bytesAt(base+256, base)
	if e != nil {
		return e
	}
	s.V4PoolKey = &V4PoolKey{c0, c1, uint32(fee), tick, hooks}
	s.ZeroForOne = boolptr(zero)
	s.HookData = hook
	if exactOut {
		s.AmountOut = a
		s.AmountInMaximum = b
	} else {
		s.AmountIn = a
		s.AmountOutMinimum = b
	}
	return nil
}
func decodeV4Multi(s *Swap, input []byte, exactOut bool) error {
	d := decoder{input}
	base, e := d.offset(0)
	if e != nil {
		return e
	}
	pathRel, e := d.offset(base + 32)
	if e != nil {
		return e
	}
	arr := base + pathRel
	n, e := d.offset(arr)
	if e != nil {
		return e
	}
	itemsBase := arr + 32
	if n > (len(input)-itemsBase)/32 {
		return ErrMalformedCalldata
	}
	s.V4Path = make([]V4Hop, n)
	for i := 0; i < n; i++ {
		rel, e := d.offset(itemsBase + i*32)
		if e != nil {
			return e
		}
		x := itemsBase + rel
		currency, e := d.address(x)
		if e != nil {
			return e
		}
		fee, e := d.uint64(x + 32)
		if e != nil || fee > 0xffffff {
			return ErrMalformedCalldata
		}
		tick, e := d.signed24(x + 64)
		if e != nil {
			return e
		}
		hooks, e := d.address(x + 96)
		if e != nil {
			return e
		}
		hook, e := d.bytesAt(x+128, x)
		if e != nil {
			return e
		}
		s.V4Path[i] = V4Hop{currency, uint32(fee), tick, hooks, hook}
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
		s.AmountOut = a
		s.AmountInMaximum = b
	} else {
		s.AmountIn = a
		s.AmountOutMinimum = b
	}
	return nil
}
