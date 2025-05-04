package polycast

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"math/rand/v2"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
)

func init() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
}

var (
	testData = [][]byte{
		[]byte("hello"),
		[]byte("world"),
	}
	testQs = []uint64{8404993, 9338881}
)

func TestSimple(t *testing.T) {
	// create players:
	numPlayers := 7
	for leaderNum := range 7 {
		noIssueTest(t, numPlayers, leaderNum)
	}
}

func noIssueTest(t *testing.T, numPlayers, leaderIndex int) {
	timeout, cncl := context.WithTimeout(context.Background(), 5*time.Second)
	defer cncl()

	data := testData

	ps, playersOutChan := makePlayers(t, 7)

	err := ps[0].Send(dataToBigInts(data), ID{
		Unique: []byte("test1"),
	})

	if err != nil {
		t.Fatalf("send error: %v", err)
	}

	all := make([]int, len(ps))
	for i := range ps {
		all[i] = i
	}

	numDone := 0
	for {
		var msg *Wireable
		select {
		case msg = <-playersOutChan:
		case <-timeout.Done():
			t.Fatal("test timed out.")
		}

		dests := make([]int, 0)
		if msg.Meta.IsBroadcast {
			dests = all
		} else {
			dests = append(dests, int(msg.Meta.Destination))
		}

		for _, index := range dests {
			inc, err := FromWire(msg)
			if err != nil {
				panic("couldn't change wire message to incoming:" + err.Error())
			}

			val, err := ps[index].Receive(inc)
			if err != nil {
				// handle error
				fmt.Println("Error receiving message:", err)
			}
			if val == nil {
				continue
			}

			if string(val[0].Bytes()) != "hello" {
				t.Fatalf("expected hello, got %s", val[0].Bytes())
			}
			if string(val[1].Bytes()) != "world" {
				t.Fatalf("expected world, got %s", val[1].Bytes())
			}

			for i, v := range val {
				if i > 1 {
					if v.Cmp(big.NewInt(0)) != 0 {
						t.Fatalf("expected 0, got %s", v.String())
					}
				}
			}
			numDone++
			if numDone == len(ps)-1 {
				return
			}
		}
	}
}

func TestOmitReceiver(t *testing.T) {
	// create players:
	ps, playersOutChan := makePlayers(t, 4)

	data := testData

	err := ps[0].Send(dataToBigInts(data), ID{
		Unique: []byte("test1"),
	})

	if err != nil {
		t.Fatalf("send error: %v", err)
	}

	all := make([]int, len(ps))
	for i := range ps {
		all[i] = i
	}

	for msg := range playersOutChan {
		msg := msg

		dests := make([]int, 0)
		if msg.Meta.IsBroadcast {
			dests = all
		} else {
			dests = append(dests, int(msg.Meta.Destination))
		}

		for _, index := range dests {
			inc, err := FromWire(msg)
			if err != nil {
				panic("couldn't change wire message to incoming:" + err.Error())
			}

			if inc.GetContent().ProtoReflect().Type().Descriptor().Name() == "Round1Message" && index == 1 {
				continue
			}

			val, err := ps[index].Receive(inc)
			if err != nil {
				// handle error
				fmt.Println("Error receiving message:", err)
			}

			if val == nil {
				continue
			}

			if index == 1 {
				fmt.Println("received value:")
				for _, v := range val {
					fmt.Println("value: ", string(v.Bytes()))
				}

				return // Process complete. message received.
			}
		}
	}
}

// malicious checks:
type Receiver interface {
	Receive(inc IncomingMessage) ([]*big.Int, error)
}

type Malicious struct {
	wireChan         chan *Wireable
	identity         Identity
	peers            []Identity
	possibleMessages []proto.Message
	numRns           int
	rnd              *rand.Rand
}

func (m *Malicious) coin(perc float64) bool {
	coin := m.rnd.Float64()
	return coin < perc
}

func (m *Malicious) makeRandomRns() *RNSPoint {
	if m.coin(0.05) {
		return nil // with small probability,malformed messages too.
	}

	rns := &RNSPoint{V: make([]uint64, m.numRns)}
	for i := range rns.V {
		rns.V[i] = uint64(m.rnd.Uint32() % (1 << 16))
	}

	return rns
}

func (m *Malicious) Receive(inc IncomingMessage) ([]*big.Int, error) {
	numRandomMessages := 1 + m.rnd.IntN(5)
	for range numRandomMessages {
		randMsg := m.genRandomMessage()
		if randMsg == nil {
			continue
		}

		meta := &WireMeta{
			Sender: int32(m.identity.CommunicationIndex),
		}

		if m.coin(0.5) { // fair coin.
			meta.IsBroadcast = true
		} else {
			meta.IsBroadcast = false
			meta.Destination = int32(m.peers[m.rnd.IntN(len(m.peers))].CommunicationIndex)
		}

		m.wireChan <- ToWire(randMsg, bytesToID(inc.GetMessageID()), meta)
	}

	return nil, nil // no errors.
}

func (m *Malicious) genRandomMessage() proto.Message {
	chooseRandom := m.rnd.IntN(len(m.possibleMessages))
	randMsg := proto.Clone(m.possibleMessages[chooseRandom])

	switch msg := randMsg.(type) {
	case *Round1Message:
		m.makeRound1Message(msg)
	case *Round2Message:
		m.makeRound2Message(msg)
	case *Round3Message1:
		m.MakeRound3Message(msg)
	case *Round3DelayedMessage:
		msg.MyValue = m.makeRandomRns()
	case nil:
		return nil
	}

	return randMsg
}

func (m *Malicious) MakeRound3Message(msg *Round3Message1) {
	msg.SuggestedValue = m.makeRandomRns()
	msg.MyValue = m.makeRandomRns()
}

func (m *Malicious) makeRound2Message(msg *Round2Message) {
	if m.coin(0.05) {
		msg.Challenge = nil

		return
	}

	msg.Challenge = &Challenge{
		RandomX:                  m.makeRandomRns(),
		PolynomialValueOfRandomX: m.makeRandomRns(),
	}
}

func (m *Malicious) makeRound1Message(msg *Round1Message) {
	if m.coin(0.05) {
		msg.RnsPolynomials = nil

		return
	}
	msg.RnsPolynomials = []*Polynomial{}
	for range m.numRns {
		msg.RnsPolynomials = append(msg.RnsPolynomials, &Polynomial{
			Coeffs: make([]uint64, len(m.peers)+1),
		})

		for i := range msg.RnsPolynomials[len(msg.RnsPolynomials)-1].Coeffs {
			msg.RnsPolynomials[len(msg.RnsPolynomials)-1].Coeffs[i] = uint64(m.rnd.Uint32() % (1 << 16))
		}
	}
}

func TestMaliciousPlayer(t *testing.T) {
	maxRushMessages := 10
	for i := range 512 {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			id := ID{
				Unique: []byte("testMal"),
			}

			players, playersOutChan := makePlayers(t, 4)

			data := testData

			ps := make([]Receiver, len(players))
			for i, p := range players {
				ps[i] = p
			}

			b := byte(i)
			seed := [32]byte{b, b % 3, b % 5, b % 7, b % 11, b % 13, b % 17, b % 19, b % 23, b % 29, b % 31}
			maliciousIndex := 1
			ps[maliciousIndex] = makeMaliciousPlayer(players, playersOutChan, maliciousIndex, seed)

			for range ps[maliciousIndex].(*Malicious).rnd.IntN(maxRushMessages) {
				// forces the malicious player to send random messages.
				ps[maliciousIndex].Receive(&incomingMessage{ID: id.Bytes()})
			}

			err := players[0].Send(dataToBigInts(data), id)

			if err != nil {
				t.Fatalf("send error: %v", err)
			}

			all := make([]int, len(ps))
			for i := range ps {
				all[i] = i
			}

			for msg := range playersOutChan {
				msg := msg

				dests := make([]int, 0)
				if msg.Meta.IsBroadcast {
					dests = all
				} else {
					dests = append(dests, int(msg.Meta.Destination))
				}

				for _, index := range dests {
					inc, err := FromWire(msg)
					if err != nil {
						panic("couldn't change wire message to incoming:" + err.Error())
					}

					if inc.GetContent().ProtoReflect().Type().Descriptor().Name() == "Round1Message" && index == 1 {
						continue
					}

					val, err := ps[index].Receive(inc)
					if err != nil {
						// handle error
						fmt.Println("Error receiving message:", err)
					}

					if val == nil {
						continue
					}

					if index == maliciousIndex+1 {
						fmt.Println("received value:")
						for _, v := range val {
							fmt.Println("value: ", string(v.Bytes()))
						}

						return // Process complete. message received.
					}
				}
			}
		})
	}
}

func makeMaliciousPlayer(players []*Player, playersOutChan chan *Wireable, index int, seed [32]byte) *Malicious {
	peersWithoutMal := make([]Identity, len(players)-1)
	i := 0
	for _, p := range players {
		if i == index {
			continue
		}

		peersWithoutMal[i] = p.Self
		i++
	}

	return &Malicious{
		wireChan:         playersOutChan,
		identity:         players[index].Self,
		peers:            peersWithoutMal,
		possibleMessages: []proto.Message{&Round1Message{}, &Round2Message{}, &Round3Message1{}, &Round3DelayedMessage{}, nil},
		rnd:              rand.New(rand.NewChaCha8(seed)), // using specific seed for reproducibility
		numRns:           len(players[index].Codes),
	}
}

func makePlayers(t *testing.T, n int) ([]*Player, chan *Wireable) {
	prms := make([]Parameters, n)
	identities := make([]Identity, n)
	for i := range n {
		p := Parameters{
			Qs:      testQs,
			N:       n,
			F:       n / 3,
			TLSCert: nil,
			Self: Identity{
				Hostname:           "localhost",
				Port:               i,
				CommunicationIndex: int16(i),
			},
			qis: []*big.Int{},
		}

		identities[i] = p.Self
		prms[i] = p
	}

	sendchan := make(chan *Wireable, n*n*10000)
	players := make([]*Player, n)
	for i, p := range prms {
		player, err := NewPlayer(p, identities, sendchan)
		if err != nil {
			t.Fatalf("failed to create player: %v", err)
		}

		players[i] = player
	}

	return players, sendchan
}

func dataToBigInts(data [][]byte) []*big.Int {
	bigInts := make([]*big.Int, len(data))
	for i, d := range data {
		bigInts[i] = new(big.Int).SetBytes(d)
	}
	return bigInts
}
