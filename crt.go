package polycast

import (
	"math/big"

	"github.com/jonathanmweiss/go-gao/field"
)

type CRTApplier struct {
	mi []*big.Int
	m  *big.Int
}

func (c CRTApplier) CRT(a []*big.Int) *big.Int {
	// Compute M = m₁ * m₂ * ... * mₖ
	M := c.m

	result := big.NewInt(0)
	tmp := new(big.Int)
	Mi := new(big.Int)
	yi := new(big.Int)

	for i := 0; i < len(a); i++ {
		Mi.Div(M, c.mi[i])

		// yi = Mi^{-1} mod m[i]
		yi.ModInverse(Mi, c.mi[i])

		// result += a[i] * Mi * yi
		tmp.Mul(a[i], Mi)
		tmp.Mul(tmp, yi)
		result.Add(result, tmp)
	}

	// result = result mod M
	result.Mod(result, M)

	return result
}

func NewCRTApplier(mi []uint64) CRTApplier {
	m := big.NewInt(1)

	c := CRTApplier{
		mi: make([]*big.Int, len(mi)),
		m:  m,
	}

	for i := range mi {
		c.mi[i] = new(big.Int).SetUint64(mi[i])
		m.Mul(m, c.mi[i])
	}

	return c
}

func (c CRTApplier) CRTPoly(p []*field.Polynomial) []*big.Int {
	if len(p) != len(c.mi) {
		panic("length of polynomials must match length of moduli")
	}

	ps := make([][]uint64, len(p))
	for i := range p {
		ps[i] = p[i].ToSlice()
	}

	lenn := len(ps[0])
	for i := range ps {
		if len(ps[i]) != lenn {
			panic("length of polynomials must match length of moduli")
		}
	}

	result := make([]*big.Int, len(ps[0]))
	for i := range result {
		result[i] = new(big.Int)

		// put all polynomial values in a small RNS array of big ints.
		a := make([]*big.Int, len(ps))
		for j := range ps {
			a[j] = new(big.Int).SetUint64(ps[j][i])
		}

		result[i] = c.CRT(a)
	}

	return result
}
