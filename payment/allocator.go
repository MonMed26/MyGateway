// Package payment implements amount allocation, settlement matching, and the
// reconciliation orchestrator shared by every provider adapter.
package payment

import (
	"crypto/rand"
	"math/big"

	"github.com/hirotomasato/paygateme/core"
)

// AmountAllocator allocates a unique whole-rupiah offset so concurrent orders
// can be told apart purely by the amount that lands in the merchant account.
//
// Uniqueness is enforced on the resulting amount (baseAmount + offset), not on
// the offset alone. Scoping per base amount is not enough: `3500+1` and
// `3499+2` both settle at `3501`.
type AmountAllocator struct {
	minOffset int64
	maxOffset int64
}

// NewAmountAllocator creates an allocator with the given offset window
// [1, maxOffset]. maxOffset must be positive.
func NewAmountAllocator(maxOffset int64) (*AmountAllocator, error) {
	return NewAmountAllocatorRange(1, maxOffset)
}

// NewAmountAllocatorRange creates an allocator with the offset window
// [minOffset, maxOffset]. Both bounds are inclusive; minOffset may be zero.
func NewAmountAllocatorRange(minOffset, maxOffset int64) (*AmountAllocator, error) {
	if minOffset < 0 {
		minOffset = 0
	}
	if maxOffset < minOffset {
		return nil, core.NewConfigError("maxOffset must be greater than or equal to minOffset", nil)
	}
	if maxOffset < 1 {
		return nil, core.NewConfigError("maxOffset must be a positive integer", nil)
	}
	return &AmountAllocator{minOffset: minOffset, maxOffset: maxOffset}, nil
}

// Min returns the smallest offset this allocator will hand out.
func (a *AmountAllocator) Min() int64 { return a.minOffset }

// DefaultAmountAllocator returns an allocator with the default offset window.
func DefaultAmountAllocator() *AmountAllocator {
	a, _ := NewAmountAllocator(core.DefaultMaxUniqueOffset)
	return a
}

// Max returns the largest offset this allocator will hand out.
func (a *AmountAllocator) Max() int64 { return a.maxOffset }

// Allocate chooses a free offset in [minOffset, maxOffset] whose resulting
// amount (baseAmount + offset) is not claimed by an active payment.
//
// The starting offset is drawn from crypto/rand and the scan then probes
// forward with wraparound. Randomizing the start prevents sequential, guessable
// unique codes (a buyer could otherwise infer another order's transfer amount),
// while the probe still guarantees no collision within the window. It returns a
// CodeAmountPoolExhausted error when every slot in range is claimed.
func (a *AmountAllocator) Allocate(baseAmount int64, taken map[int64]bool) (int64, error) {
	window := a.maxOffset - a.minOffset + 1
	if window <= 0 {
		return 0, core.NewBaseErrorf(core.CodeAmountPoolExhausted,
			"No free unique amount slot available for base amount %d (offset window %d..%d is invalid)",
			baseAmount, a.minOffset, a.maxOffset)
	}

	start := a.minOffset
	if window > 1 {
		if n, err := rand.Int(rand.Reader, big.NewInt(window)); err == nil {
			start = a.minOffset + n.Int64()
		}
	}

	for i := int64(0); i < window; i++ {
		offset := a.minOffset + (start-a.minOffset+i)%window
		if !taken[baseAmount+offset] {
			return offset, nil
		}
	}
	return 0, core.NewBaseErrorf(core.CodeAmountPoolExhausted,
		"No free unique amount slot available for base amount %d (offset window %d..%d is fully claimed)",
		baseAmount, a.minOffset, a.maxOffset)
}
