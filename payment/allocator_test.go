package payment

import (
	"testing"

	"github.com/hirotomasato/paygateme/core"
)

func TestAllocateReturnsFreeSlotInRange(t *testing.T) {
	a := DefaultAmountAllocator()
	taken := map[int64]bool{1001: true, 1002: true}
	offset, err := a.Allocate(1000, taken)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if offset < 1 || offset > 999 {
		t.Fatalf("offset %d outside [1,999]", offset)
	}
	if offset == 1 || offset == 2 {
		t.Fatalf("offset %d collides with a taken final amount", offset)
	}
}

func TestAllocateIsRandomized(t *testing.T) {
	a := DefaultAmountAllocator()
	seen := make(map[int64]bool)
	for i := 0; i < 25; i++ {
		offset, err := a.Allocate(1000, map[int64]bool{})
		if err != nil {
			t.Fatalf("Allocate: %v", err)
		}
		seen[offset] = true
	}
	// With a 999-wide window the chance of all 25 random starts landing on the
	// same value is effectively zero; this guards against sequential allocation.
	if len(seen) < 2 {
		t.Fatalf("allocations look deterministic/sequential: %v", seen)
	}
}

func TestAllocateNeverCollidesUnderLoad(t *testing.T) {
	a := DefaultAmountAllocator()
	taken := make(map[int64]bool)
	const n = 500
	for i := 0; i < n; i++ {
		offset, err := a.Allocate(10000, taken)
		if err != nil {
			t.Fatalf("Allocate #%d: %v", i, err)
		}
		amount := int64(10000) + offset
		if taken[amount] {
			t.Fatalf("allocator returned a taken amount %d", amount)
		}
		taken[amount] = true
	}
	if len(taken) != n {
		t.Fatalf("allocated %d distinct amounts, want %d", len(taken), n)
	}
}

func TestAllocateOnFinalAmount(t *testing.T) {
	// Two different base amounts landing on the same total must conflict:
	// 3500+1 and 3499+2 both settle at 3501.
	a := DefaultAmountAllocator()
	taken := map[int64]bool{3501: true}
	offset, err := a.Allocate(3500, taken)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if offset == 1 {
		t.Fatal("offset 1 collides with a taken final amount")
	}
}

func TestAllocatePoolExhausted(t *testing.T) {
	a, err := NewAmountAllocator(2)
	if err != nil {
		t.Fatalf("NewAmountAllocator: %v", err)
	}
	taken := map[int64]bool{11: true, 12: true}
	if _, err := a.Allocate(10, taken); err == nil {
		t.Fatal("expected pool exhaustion error")
	} else if !core.IsErrorCode(err, core.CodeAmountPoolExhausted) {
		t.Fatalf("error code = %v, want AMOUNT_POOL_EXHAUSTED", err)
	}
}

func TestAllocateRangeBounds(t *testing.T) {
	a, err := NewAmountAllocatorRange(100, 120)
	if err != nil {
		t.Fatalf("NewAmountAllocatorRange: %v", err)
	}
	taken := make(map[int64]bool)
	for i := 0; i < 21; i++ {
		offset, err := a.Allocate(5000, taken)
		if err != nil {
			t.Fatalf("Allocate #%d: %v", i, err)
		}
		if offset < 100 || offset > 120 {
			t.Fatalf("offset %d outside [100,120]", offset)
		}
		taken[5000+offset] = true
	}
	if _, err := a.Allocate(5000, taken); err == nil {
		t.Fatal("expected exhaustion after filling the range")
	}
}

func TestNewAmountAllocatorRejectsInvalidMax(t *testing.T) {
	if _, err := NewAmountAllocator(0); err == nil {
		t.Fatal("expected error for maxOffset 0")
	}
	if _, err := NewAmountAllocator(-1); err == nil {
		t.Fatal("expected error for negative maxOffset")
	}
	if _, err := NewAmountAllocatorRange(50, 10); err == nil {
		t.Fatal("expected error for min > max")
	}
}
