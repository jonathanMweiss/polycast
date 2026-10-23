package polycast

import (
	"log/slog"

	"github.com/jonathanmweiss/go-gao"
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

	// grab each value, and declare any missing value as an erasure.
	// then use gao's reconstruction for each polynomial.
	// on failure: return nil, and keep on waiting.
	vals := state.acceptanceStage.valueOfPeer
	polys := make([]*field.Polynomial, len(p.Codes))

	var missing []int

	for i := 0; i < p.Parameters.N; i++ {
		if _, ok := vals[i]; !ok {
			missing = append(missing, i)
		}
	}

	for rnsLevel, code := range p.Codes {
		erasures, err := code.Erasures(missing...)
		if err != nil {
			return
		}

		codeword := make(gao.Codeword, p.Parameters.N)

		for i := range codeword {
			if v, ok := vals[i]; ok {
				codeword[i] = v.V[rnsLevel]
			}
		}

		data, err := code.Decode(codeword, erasures)
		if err != nil {
			return
		}

		polys[rnsLevel] = p.Rings[rnsLevel].NewPolynomial(data, false)
	}

	slog.Debug("reconstructed polynomials!",
		slog.Int("player", int(p.Self.CommunicationIndex)),
	)
	// We managed to reconstruct the polynomials without issues!

	state.reconstructionStage.reconstructed = polys
	state.reconstructionStage.done = true
}
