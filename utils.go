package polycast

import (
	"errors"

	"github.com/jonathanmweiss/go-gao/field"
)

func (p *Player) toFieldPoly(polynomial []*Polynomial) []*field.Polynomial {
	fieldPolys := make([]*field.Polynomial, len(p.Codes))
	for i, code := range p.Codes {
		fld := code.PrimeField()
		elems := fld.ElemSlice(polynomial[i].Coeffs)
		fieldPolys[i] = field.NewPolynomial(elems, false)
	}

	return fieldPolys
}

// sendMany iterates over many wireables and sends each one.
func (p *Player) output(msgs ...*Wireable) error {
	for _, m := range msgs {
		select {
		case p.send <- m:
		case <-p.closechan:
			return ErrClosedPlayer
		}
	}

	return nil
}

var ErrBadRnsValue = errors.New("RNS value is not valid")

func (p *Player) validateRNSValue(rnsValue *RNSPoint) error {
	if rnsValue == nil {
		return ErrNilContent
	}

	if len(rnsValue.V) != len(p.Codes) {
		return ErrBadRnsValue
	}

	return nil
}
