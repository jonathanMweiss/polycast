package polycast

import (
	"errors"
	"math/rand/v2"

	"log/slog"
)

func (p *Player) receiveRound1Message(m *Round1Message, wm *WireMeta, ID ID) (*relbroadState, error) {
	slog.Debug("received Round1Message", slog.Int("player", int(p.Self.CommunicationIndex)))

	if err := p.round1MsgValidation(wm, ID, m); err != nil {
		return nil, err
	}

	// attempt create state:
	state, err := p.loadOrCreateBroadcastState(ID)
	if err != nil && err != ErrIDExists {
		return nil, err
	}

	// update state:
	state.AddPolynomialsIfNotSeen(p.toFieldPoly(m.RnsPolynomials))
	state.Lock()
	shouldSendChallenges := !state.inspectionStage.sentChallenges
	state.inspectionStage.sentChallenges = true
	state.Unlock()

	if !shouldSendChallenges {
		return state, nil
	}

	// dataFromLeader is `writeonce`` object, so we can safely read it here.
	// without a lock.
	polys := state.dataFromLeader.polynomials

	challenges := make([]*Wireable, len(p.Peers))
	// now prepare challenges to your friends.
	for _, peerID := range p.Peers {
		msg := &Round2Message{
			Challenge: &Challenge{
				RandomX:                  &RNSPoint{V: make([]uint64, len(p.Codes))},
				PolynomialValueOfRandomX: &RNSPoint{V: make([]uint64, len(p.Codes))},
			},
		}

		// create challenge
		for rnsLevel, code := range p.Codes {
			fld := code.PrimeField()

			x := rand.Uint64() % fld.Prime()
			y := polys[rnsLevel].Eval(x)

			msg.Challenge.RandomX.V[rnsLevel] = x
			msg.Challenge.PolynomialValueOfRandomX.V[rnsLevel] = y.Value()
		}

		challenges[peerID.CommunicationIndex] = ToWire(msg, ID, &WireMeta{
			IsBroadcast: false,
			Sender:      int32(p.Self.CommunicationIndex),
			Destination: int32(peerID.CommunicationIndex),
		})
	}

	return state, p.output(challenges...)
}

var (
	ErrBadPolynomialRep = errors.New("polynomials have bad RNS representation")
)

func (p *Player) round1MsgValidation(wm *WireMeta, ID ID, m *Round1Message) error {
	if wm.Sender != int32(ID.leader) {
		return errors.New("round1Message leader ID doesn't match sender id")
	}

	if len(m.RnsPolynomials) != len(p.Codes) {
		return ErrBadPolynomialRep
	}

	for _, rnsPoly := range m.RnsPolynomials {
		if len(rnsPoly.Coeffs) != p.Parameters.F+1 {
			return errors.New("number of coefficients does not match number of codes")
		}
	}

	return nil
}
