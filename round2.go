package polycast

import (
	"errors"
	"fmt"
	"log/slog"

	"github.com/jonathanmweiss/go-gao/field"
)

func (p *Player) receiveRound2Message(m *Round2Message, meta *WireMeta, id ID) (*relbroadState, error) {
	if err := p.round2MsgValidation(meta, id, m); err != nil {
		return nil, err
	}

	state, err := p.loadOrCreateBroadcastState(id)
	if err != nil && err != ErrIDExists {
		return nil, err
	}

	state.Lock()
	defer state.Unlock()

	if p.Self.CommunicationIndex == 2 {
		fmt.Print()
	}
	// doesn't allow to receive the same challenge twice.
	if _, ok := state.inspectionStage.indexChallenge[int(meta.Sender)]; ok {
		return nil, errors.New("already received challenge from this peer")
	}

	slog.Debug("received challenge", slog.Int("player", int(p.Self.CommunicationIndex)), slog.Int("peer", int(meta.Sender)))

	state.inspectionStage.indexChallenge[int(meta.Sender)] = m.Challenge

	return state, nil
}

func (s *relbroadState) shouldCommit() bool {
	s.Lock()
	defer s.Unlock()

	stage := s.stage
	isAccepted := s.inspectionStage.isAccepted

	if isAccepted && commitmentStage == stage {
		s.stage = acceptanceStage

		return true
	}

	return false
}

func (p *Player) checkReceivedChallenges(state *relbroadState) {
	phase := &state.inspectionStage
	if phase.isAccepted {
		return
	}

	if state.dataFromLeader == nil {
		return
	}

	polys := state.dataFromLeader.polynomials
	numAccepted := 0

	for i := range phase.indexChallenge {
		if phase.inspectChallenge(polys, i) {
			numAccepted++
		}
	}

	if numAccepted < (p.Parameters.N - p.Parameters.F) {
		return
	}

	slog.Debug("moved to commitment stage.", slog.Int("player", int(p.Self.CommunicationIndex)))
	state.stage = commitmentStage
	state.inspectionStage.isAccepted = true
}

func (i *inspectionStageData) inspectChallenge(polys []*field.Polynomial, playerIndex int) bool {
	challengeResult, ok := i.challengeState[playerIndex]
	if ok {
		return challengeResult // already accepted.
	}

	ch, ok := i.indexChallenge[playerIndex]
	if !ok {
		return false
	}

	x := ch.RandomX
	challengeY := ch.PolynomialValueOfRandomX

	for j, p := range polys {
		y := p.Eval(x.V[j])

		if y.Value() != challengeY.V[j] {
			// not accepted. won't be checked again.
			i.challengeState[playerIndex] = false

			return false
		}
	}

	i.challengeState[playerIndex] = true

	return true
}

func (p *Player) round2MsgValidation(meta *WireMeta, id ID, m *Round2Message) error {
	if m == nil {
		return ErrNilMessage
	}

	if m.Challenge == nil {
		return errors.New("challenge is nil")
	}

	if err := p.validateRNSValue(m.Challenge.RandomX); err != nil {
		return err
	}

	return p.validateRNSValue(m.Challenge.PolynomialValueOfRandomX)
}
