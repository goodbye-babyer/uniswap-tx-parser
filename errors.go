package uniswaptxparser

import "errors"

var (
	ErrNotRouterTransaction = errors.New("not a configured router transaction")
	ErrUnsupportedMethod    = errors.New("unsupported router method")
	ErrMalformedCalldata    = errors.New("malformed calldata")
	ErrRecursionLimit       = errors.New("maximum recursion depth exceeded")
)
