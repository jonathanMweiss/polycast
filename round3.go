package polycast

import (
	"crypto/sha512"
	"log/slog"

	"github.com/jonathanmweiss/go-gao/field"
	"google.golang.org/protobuf/proto"
)

func (p *Player) receiveRound3Message1(m *Round3Message1, meta *WireMeta, id ID) (*relbroadState, error) {
	if err := p.round3MsgValidation(meta, id, m); err != nil {
		return nil, err
	}

	state, err := p.loadOrCreateBroadcastState(id)
	if err != nil && err != ErrIDExists {
		return nil, err
	}

	state.Lock()
	defer state.Unlock()

	slog.Debug("received Round3Message1",
		slog.Int("player", int(p.Self.CommunicationIndex)),
		slog.Int("peer", int(meta.Sender)),
	)

	if _, ok := state.acceptanceStage.valueOfPeer[int(meta.Sender)]; ok {
		return state, nil // ignoring attempts to send different values.
	}

	state.acceptanceStage.valueOfPeer[int(meta.Sender)] = m.MyValue

	if state.acceptanceStage.myPoint.sentMyPoint {
		return state, nil // already have my point.
	}

	state.acceptanceStage.myPoint.suggestedValues[int(meta.Sender)] = m.SuggestedValue

	return state, nil
}

func (p *Player) receiveRound3DelayedMessage(m *Round3DelayedMessage, meta *WireMeta, id ID) (*relbroadState, error) {
	if err := p.round3DelayedMsgValidation(meta, id, m); err != nil {
		return nil, err
	}

	state, err := p.loadOrCreateBroadcastState(id)
	if err != nil && err != ErrIDExists {
		return nil, err
	}

	state.Lock()
	defer state.Unlock()

	slog.Debug("received Round3DelayedMessage",
		slog.Int("player", int(p.Self.CommunicationIndex)),
		slog.Int("peer", int(meta.Sender)),
	)

	if _, ok := state.acceptanceStage.valueOfPeer[int(meta.Sender)]; ok {
		return state, nil // ignoring attempts to send different values.
	}

	state.acceptanceStage.valueOfPeer[int(meta.Sender)] = m.MyValue

	return state, nil

}

func (p *Player) round3DelayedMsgValidation(meta *WireMeta, id ID, m *Round3DelayedMessage) error {
	if m == nil {
		return ErrNilMessage
	}

	return p.validateRNSValue(m.MyValue)
}

func (p *Player) round3MsgValidation(meta *WireMeta, id ID, m *Round3Message1) error {
	if m == nil {
		return ErrNilMessage
	}

	if err := p.validateRNSValue(m.MyValue); err != nil {
		return err
	}

	return p.validateRNSValue(m.SuggestedValue)
}

// makeCommitment is called by the player to create a commitment to its value, and
// to send to all other players the values this player has committed to.
func (p *Player) makeCommitment(state *relbroadState) error {
	state.Lock()

	stage := state.stage
	id := state.ID
	polys := state.dataFromLeader.polynomials

	if stage == finalStage {
		state.Unlock()

		return nil
	}
	stage = finalStage // moving to the final stage: acceptance.

	state.Unlock()

	msgs, err := p.makeEvaluations(polys)
	if err != nil {
		return err
	}

	// every msg contains this player's point.
	p.storeMyPoint(state, proto.CloneOf(msgs[0].MyValue))

	slog.Debug("sending commitment stage messages",
		slog.Int("player", int(p.Self.CommunicationIndex)),
	)
	wireables := p.wrapRound3Message1(msgs, id)
	if err := p.output(wireables...); err != nil {
		return err
	}

	return nil
}

func (p *Player) storeMyPoint(state *relbroadState, myRnsPoint *RNSPoint) {
	state.Lock()
	defer state.Unlock()

	state.acceptanceStage.myPoint.sentMyPoint = true
	state.acceptanceStage.myPoint.myValue = proto.CloneOf(myRnsPoint)

	state.acceptanceStage.valueOfPeer[int(p.Self.CommunicationIndex)] = proto.CloneOf(myRnsPoint)
}

func (p *Player) wrapRound3Message1(msgs []*Round3Message1, id ID) []*Wireable {
	wireables := make([]*Wireable, len(msgs))
	i := -1
	for _, peerID := range p.Peers {
		if peerID.CommunicationIndex == p.Self.CommunicationIndex {
			continue
		}
		i++

		msg := msgs[i]

		wireables[i] = ToWire(msg, id, &WireMeta{
			IsBroadcast: false,
			Sender:      int32(p.Self.CommunicationIndex),
			Destination: int32(peerID.CommunicationIndex),
		})
	}

	return wireables
}

// creates messages for all peers. where message i is for peer[i].
func (p *Player) makeEvaluations(polys []*field.Polynomial) ([]*Round3Message1, error) {
	msgs := make([]*Round3Message1, len(p.Peers)-1) // removing this player.
	for i := range msgs {
		msgs[i] = &Round3Message1{
			SuggestedValue: &RNSPoint{
				V: make([]uint64, len(p.Codes)),
			},
		}
	}

	// xs represents the x values for which the polynomial is evaluated.
	// each peer receive one specific x value (according to its peer index).
	xs := make([][]uint64, len(p.Codes))

	evaluatedPolynomials := make([]map[uint64]uint64, len(p.Codes))
	// for each poly, create an evaluation for all peers.
	for rnsLevel, code := range p.Codes {
		evalOverSpecificRNSValue, err := code.Encode(polys[rnsLevel].ToSlice())
		if err != nil {
			return nil, err
		}

		evaluatedPolynomials[rnsLevel] = evalOverSpecificRNSValue

		xs[rnsLevel] = code.EvaluationPoints(code.N())
	}

	m := 0

	for _, peer := range p.Peers {
		peerPos := peer.CommunicationIndex

		if peerPos == p.Self.CommunicationIndex {
			continue
		}

		for rnsLevel := range p.Codes {
			rnsX := xs[rnsLevel][peerPos]
			msgs[m].SuggestedValue.V[rnsLevel] = evaluatedPolynomials[rnsLevel][rnsX]
		}

		m++
	}

	myValue := &RNSPoint{V: make([]uint64, len(p.Codes))}
	// now add my value to the message.
	for rnsLevel := range p.Codes {
		x := xs[rnsLevel][p.Self.CommunicationIndex]
		myValue.V[rnsLevel] = evaluatedPolynomials[rnsLevel][x]
	}

	// now add my value to the messages.
	for i := range msgs {
		msgs[i].MyValue = proto.CloneOf(myValue)
	}

	return msgs, nil
}

func (p *Player) attemptChooseMyValue(state *relbroadState) {
	state.Lock()
	defer state.Unlock()

	//  attempt to choose myValue ONLY if I don't have a value from the leader.
	if state.dataFromLeader != nil {
		return
	}

	// if I already have a value, don't need to choose it again.
	if state.acceptanceStage.myPoint.myValue != nil {
		return
	}

	// need at least f+1 values to choose myvalue
	if len(state.acceptanceStage.myPoint.suggestedValues) < p.Parameters.F {
		return
	}

	counter := map[[32]byte]int{}

	for key, v := range state.acceptanceStage.myPoint.suggestedValues {
		bts, err := proto.Marshal(v)
		if err != nil {
			slog.Error("failed to marshal suggested value",
				slog.Int("player", int(p.Self.CommunicationIndex)),
				slog.String("err:", err.Error()),
			)

			// go allows to delete from a map while iterating over it.
			delete(state.acceptanceStage.myPoint.suggestedValues, key)
		}

		hash := sha512.Sum512_256(bts)
		counter[hash]++

		if counter[hash] > p.Parameters.F {
			slog.Debug("chose my value",
				slog.Int("player", int(p.Self.CommunicationIndex)),
			)

			state.acceptanceStage.myPoint.myValue = proto.CloneOf(v)

			break
		}
	}

	if state.acceptanceStage.myPoint.myValue == nil {
		return
	}

	if state.acceptanceStage.myPoint.sentMyPoint {
		panic("bug: sent my point, but reached this code")
	}

	// send my value to all other peers.
	state.acceptanceStage.myPoint.sentMyPoint = true
	tosend := &Round3DelayedMessage{
		MyValue: proto.CloneOf(state.acceptanceStage.myPoint.myValue),
	}

	slog.Debug("sending my delayed value",
		slog.Int("player", int(p.Self.CommunicationIndex)),
	)

	err := p.output(ToWire(tosend, state.ID, &WireMeta{
		IsBroadcast: true,
		Sender:      int32(p.Self.CommunicationIndex),
	}))

	if err != nil {
		slog.Error("failed to send my value",
			slog.Int("player", int(p.Self.CommunicationIndex)),
			slog.String("err:", err.Error()),
			slog.String("type", string(tosend.ProtoReflect().Descriptor().Name())),
		)
	}
}
