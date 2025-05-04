package polycast

import (
	"log/slog"

	"github.com/jonathanmweiss/go-gao/field"
)

// The final stage of the protocol.
func (p *Player) attemptReconstruction(state *relbroadState) {
	state.Lock()
	defer state.Unlock()

	if len(state.acceptanceStage.valueOfPeer) < p.Parameters.N-p.Parameters.F {
		return
	}

	if state.reconstructionStage.done {
		return
	}

	slog.Debug(
		"attempting to reconstruct polynomials",
		slog.Int("player", int(p.Self.CommunicationIndex)),
	)

	// grab each value and put in a map, any missing value: put 0.
	// then use go's reconstruction for each polynomial.
	// on failure: return nil, and keep on waiting.
	vals := state.acceptanceStage.valueOfPeer
	polys := make([]*field.Polynomial, len(p.Codes))

	for rnsLevel, code := range p.Codes {
		indexToValue := make(map[uint64]uint64)

		xs := code.EvaluationPoints(code.N())

		for i := 0; i < p.Parameters.N; i++ {
			rnsVal := uint64(0)
			if v, ok := vals[i]; ok {
				rnsVal = v.V[rnsLevel]
			}

			indexToValue[uint64(xs[i])] = rnsVal
		}

		data, err := code.Decode(indexToValue)
		if err != nil {
			return
		}

		polys[rnsLevel] = field.NewPolynomial(code.PrimeField().ElemSlice(data), false)
	}

	slog.Debug("reconstructed polynomials!",
		slog.Int("player", int(p.Self.CommunicationIndex)),
	)
	// We managed to reconstruct the polynomials without issues!

	state.reconstructionStage.reconstructed = polys
	state.reconstructionStage.done = true
}
