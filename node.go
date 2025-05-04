package polycast

import (
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonathanmweiss/go-gao"
	"github.com/jonathanmweiss/go-gao/field"
	_ "google.golang.org/protobuf/types/known/anypb"
)

type Parameters struct {
	Qs  []uint64 // prime values that define the data.
	N   int      // number of expected participants
	F   int      // number of corrupted participants
	qis []*big.Int

	Self Identity

	// created with the private key, matches the  x509 cert inside Identity.
	TLSCert *tls.Certificate
}

type Identity struct {
	Hostname string
	Port     int

	PublicKey          ed25519.PublicKey // used to identify this Player
	Certificate        *x509.Certificate // used for TLS communication.
	CommunicationIndex int16             // cheap way to send the identity on the network.
}

// Player is the main object of this package. It represents a polyomial reliable-broadcast
// protocol participant. It can Send and Receive.
// Use NewPlayer to create a new Player.
type Player struct {
	Parameters
	// Galois fields for each prime value.
	Codes []*gao.Code // code for each field.
	Q     *big.Int    // Maximal value per incoming data coefficient.

	Peers []Identity     // list of all peers
	send  chan *Wireable // used by the Player to receive network access and send things to peers.
	// delivery chan Message  // used by the Player to indicate to the user about delvered messages.

	isclosed  atomic.Bool   // ensured that close() is only called once.
	closechan chan struct{} // used to close the player.

	broadstatesLock sync.Mutex
	broadstates     map[IDString]*relbroadState

	crt CRTApplier
}

// Inner types:
type broadcastStage int

const (
	initialStage   broadcastStage = iota
	challengeStage                // indicate received the first message from the leader.
	commitmentStage
	acceptanceStage
	finalStage
)

type relbroadState struct {
	sync.Mutex
	ID           ID
	CreationTime time.Time // using time.Time since it uses a monotonic clock.
	stage        broadcastStage
	// can be nil since it might've been missed.
	*dataFromLeader
	// cannot be nil, since it should receive from all honest players.
	inspectionStage inspectionStageData

	acceptanceStage acceptanceStageData

	reconstructionStage reconstructionStage
}

func (s *relbroadState) shouldDeliver() bool {
	s.Lock()
	defer s.Unlock()

	if s.reconstructionStage.delivered {
		return false
	}

	if s.reconstructionStage.done {
		// ensuring that delivery is done only once.
		s.reconstructionStage.delivered = true

		return true
	}

	return false
}

type reconstructionStage struct {
	done          bool
	delivered     bool
	reconstructed []*field.Polynomial // reconstructed polynomials.
}

type dataFromLeader struct {
	polynomials []*field.Polynomial // the polynomials received from the leader.
	// This should contain the polynomials from the leader.
}

// inspection phase context collects the number of polynomial challenges
// received AND accepted.
type inspectionStageData struct {
	sentChallenges bool
	// map of challenger ID and whether it was accepted or not.
	indexChallenge map[int]*Challenge
	challengeState map[int]bool // map of index to whether it was accepted or not.

	isAccepted bool // once changed to true, can proceed to the next stage.
}

type acceptanceStageData struct {
	myPoint     myPoint           // need to learn it.
	valueOfPeer map[int]*RNSPoint // value of peers. (each peer send this to me).
	// Once n-f peers have sent their value, we CAN start reconstructing the message.
	// Upon successful reconstruction, we can return it to our user.
}
type myPoint struct {
	sentMyPoint bool

	suggestedValues map[int]*RNSPoint // if f+1 sent the same value -> I can set myValue.
	myValue         *RNSPoint         // my value is nil if i couldn't send it.
}

func (p *Player) Close() {
	if p.isclosed.CompareAndSwap(false, true) {
		close(p.closechan)
	}
}

var ErrPeerMismatch = errors.New("number of peers does not match the number of players")

func NewPlayer(prms Parameters, peers []Identity, sendChan chan *Wireable) (*Player, error) {
	if prms.F*3+1 > prms.N {
		return nil, errTooManyMalicious
	}

	if len(peers) != prms.N {
		return nil, ErrPeerMismatch
	}

	if len(prms.Qs) == 0 {
		return nil, errNoUnderlyingField
	}

	Q := big.NewInt(1)

	codes := make([]*gao.Code, len(prms.Qs))
	qis := make([]*big.Int, len(prms.Qs))

	for i, qi := range prms.Qs {
		f, err := field.NewPrimeField(qi)
		if err != nil {
			return nil, err
		}

		coderParams, _ := gao.NewCodeParameters(gao.NewSlowEvaluator(f), prms.N, prms.F+1)
		codes[i] = gao.NewCodeGao(coderParams)

		qis[i] = new(big.Int).SetUint64(qi)
		Q.Mul(Q, qis[i])
	}

	prms.qis = qis

	p := &Player{
		Parameters: prms,
		Codes:      codes,
		Q:          Q,
		Peers:      peers,
		send:       sendChan,
		isclosed:   atomic.Bool{},
		closechan:  make(chan struct{}),

		broadstates: make(map[IDString]*relbroadState),

		crt: NewCRTApplier(prms.Qs),
	}

	return p, nil
}

var (
	ErrDataLength   = errors.New("data length does not match code size")
	ErrClosedPlayer = errors.New("player is closed")
)

// // if tracking ID is nil, will generate one according to the hash
// // of the DATA.
func (p *Player) Send(data []*big.Int, ID ID) error {
	if len(data) != p.Parameters.F+1 {

		// increase with zeroes.
		for i := len(data); i < p.Parameters.F+1; i++ {
			data = append(data, big.NewInt(0))
		}
	}

	ID.leader = p.Self.CommunicationIndex

	if err := ID.validID(); err != nil {
		return err
	}

	if p.isclosed.Load() {
		return ErrClosedPlayer
	}

	state, err := p.loadOrCreateBroadcastState(ID)
	if err != nil {
		return err
	}

	rnsData, err := p.toRNSPolynomials(data)
	if err != nil {
		return err
	}

	state.AddPolynomialsIfNotSeen(p.toFieldPoly(rnsData))

	msg := &Round1Message{
		RnsPolynomials: rnsData,
	}

	return p.output(ToWire(msg, ID, &WireMeta{
		IsBroadcast: true,
		Sender:      int32(p.Self.CommunicationIndex),
		Destination: -1, // broadcast
	}))
}

var ErrCoeffsTooBig = errors.New("value coefficients are too big")

// switches from big ints to residual number system representation.
func (p *Player) toRNSPolynomials(data []*big.Int) ([]*Polynomial, error) {
	rnsData := make([]*Polynomial, len(p.Codes))
	for i := range p.Codes {
		rnsData[i] = &Polynomial{
			Coeffs: make([]uint64, len(data)),
		}
	}

	tmp := new(big.Int)
	for i, b := range data {
		if b.Cmp(p.Q) >= 0 {
			return nil, ErrCoeffsTooBig
		}

		for rns, qi := range p.Parameters.qis {
			tmp.Mod(b, qi)
			rnsData[rns].Coeffs[i] = tmp.Uint64()
		}
	}

	return rnsData, nil
}

func (r *relbroadState) AddPolynomialsIfNotSeen(data []*field.Polynomial) {
	r.Lock()
	defer r.Unlock()

	if r.dataFromLeader != nil {
		return
	}

	r.dataFromLeader = &dataFromLeader{
		polynomials: data,
	}
}

var (
	errTooManyMalicious  = errors.New("value of F and N should be set such that 3*F +1 < N")
	errNoUnderlyingField = errors.New("parameters must include a Q value")
)

var ErrIDExists = errors.New("ID already exists")

func (p *Player) loadOrCreateBroadcastState(ID ID) (*relbroadState, error) {
	id := ID.string()
	if len(id) == 0 {
		return nil, ErrEmptyMessageID
	}

	p.broadstatesLock.Lock()
	defer p.broadstatesLock.Unlock()

	if v, ok := p.broadstates[id]; ok {
		return v, ErrIDExists
	}

	state := &relbroadState{
		stage:          initialStage,
		ID:             ID,
		dataFromLeader: nil,
		inspectionStage: inspectionStageData{
			indexChallenge: map[int]*Challenge{},
			challengeState: map[int]bool{},
			isAccepted:     false,
		},
		acceptanceStage: acceptanceStageData{
			myPoint: myPoint{
				sentMyPoint:     false,
				suggestedValues: map[int]*RNSPoint{},
				myValue:         nil,
			},
			valueOfPeer: map[int]*RNSPoint{},
		},
		reconstructionStage: reconstructionStage{},
		Mutex:               sync.Mutex{},
		CreationTime:        time.Now(),
	}

	p.broadstates[id] = state

	return state, nil
}

// if the return value is not nil, it means that the message was ACCEPTED, i.e., it
// can be delivered to the user of the polycast protocol.
func (p *Player) Receive(incoming IncomingMessage) ([]*big.Int, error) {
	if err := incoming.BasicValidation(); err != nil {
		return nil, err
	}

	id := bytesToID(incoming.GetMessageID())
	if err := id.validID(); err != nil {
		return nil, err
	}

	var state *relbroadState
	var err error
	// for each message, do something.
	switch m := incoming.GetContent().(type) {
	case *Round1Message:
		state, err = p.receiveRound1Message(m, incoming.GetWireMeta(), id)
	case *Round2Message:
		state, err = p.receiveRound2Message(m, incoming.GetWireMeta(), id)
	case *Round3Message1:
		state, err = p.receiveRound3Message1(m, incoming.GetWireMeta(), id)
	case *Round3DelayedMessage: // This is part of round3 also.
		state, err = p.receiveRound3DelayedMessage(m, incoming.GetWireMeta(), id)
	default:
		err = errors.New("unknown message type")
	}
	if err != nil {
		return nil, err
	}

	// Done receiving challenges, can commit to to the next stage.
	p.checkReceivedChallenges(state)

	if state.shouldCommit() {
		p.makeCommitment(state)

		slog.Debug("sent commitment stage messages",
			slog.Int("player", int(p.Self.CommunicationIndex)),
		)
	}

	p.attemptChooseMyValue(state)

	p.attemptReconstruction(state)

	if state.shouldDeliver() {
		return p.crt.CRTPoly(state.reconstructionStage.reconstructed), nil
	}

	return nil, nil
}
