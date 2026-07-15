package uniswaptxparser

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
)

func DecodeV3Path(path []byte, exactOutput bool) ([]V3Hop, error) {
	if len(path) < 43 || (len(path)-20)%23 != 0 {
		return nil, fmt.Errorf("invalid v3 path length %d", len(path))
	}
	addresses := make([]common.Address, 0, 1+(len(path)-20)/23)
	fees := make([]uint32, 0, (len(path)-20)/23)
	addresses = append(addresses, common.BytesToAddress(path[:20]))
	off := 20
	for off < len(path) {
		fees = append(fees, uint32(path[off])<<16|uint32(path[off+1])<<8|uint32(path[off+2]))
		off += 3
		addresses = append(addresses, common.BytesToAddress(path[off:off+20]))
		off += 20
	}
	if exactOutput {
		for i, j := 0, len(addresses)-1; i < j; i, j = i+1, j-1 {
			addresses[i], addresses[j] = addresses[j], addresses[i]
		}
		for i, j := 0, len(fees)-1; i < j; i, j = i+1, j-1 {
			fees[i], fees[j] = fees[j], fees[i]
		}
	}
	h := make([]V3Hop, len(fees))
	for i := range fees {
		h[i] = V3Hop{addresses[i], addresses[i+1], fees[i]}
	}
	return h, nil
}
