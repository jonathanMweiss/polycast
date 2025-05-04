package polycast

import (
	"math/big"
	"testing"
)

func TestCRT(t *testing.T) {
	// Example moduli
	testNums := []uint64{27, 23, 12, 13}

	moduli := []uint64{3, 5, 7}
	crtApplier := NewCRTApplier(moduli)

	for _, v := range testNums {
		crtNum := rnsBreak(v, moduli)

		result := crtApplier.CRT(crtNum).Uint64()
		if result != v {
			t.Errorf("Expected %d, got %d", v, result)
		}
	}
}

func rnsBreak(v uint64, moduli []uint64) []*big.Int {
	crtNum := make([]*big.Int, len(moduli))
	for i, m := range moduli {
		crtNum[i] = new(big.Int).SetUint64(v % m)
	}
	return crtNum
}
