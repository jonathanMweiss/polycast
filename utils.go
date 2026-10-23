package polycast

import (
	"errors"

	"github.com/jonathanmweiss/go-gao/field"
)

func (p *Player) toFieldPoly(polynomial []*Polynomial) []*field.Polynomial {
	fieldPolys := make([]*field.Polynomial, len(p.Codes))
	for i, ring := range p.Rings {
		// NewPolynomial reduces its input in place, so don't hand it the message's slice.
		coeffs := append([]uint64(nil), polynomial[i].Coeffs...)
		fieldPolys[i] = ring.NewPolynomial(coeffs, false)
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
