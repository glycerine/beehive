package bh

import (
	"fmt"
	"testing"
)

type joinedHandler struct {
	joined chan bool
}

func (h *joinedHandler) Rcv(msg Msg, ctx RcvContext) error {
	return nil
}

func (h *joinedHandler) Map(msg Msg, ctx MapContext) MappedCells {
	h.joined <- true
	return nil
}

func startHivesForReplicationTest(t *testing.T, addrs []string,
	preStart func(h Hive)) ([]Hive, []chan bool) {

	hiveJoinedCh := make(chan bool)
	hives := make([]Hive, len(addrs))
	chans := make([]chan bool, len(addrs))
	for i, a := range addrs {
		hives[i] = hiveWithAddressForRegistryTests(a, t)
		maybeSkipRegistryTest(hives[i].(*hive), t)
		chans[i] = make(chan bool)
		hives[i].NewApp("joined").Handle(HiveJoined{}, &joinedHandler{
			joined: hiveJoinedCh,
		})
		preStart(hives[i])
		go hives[i].Start(chans[i])
	}

	for _ = range addrs {
		for _ = range addrs {
			<-hiveJoinedCh
		}
	}

	return hives, chans
}

func stopHivesForReplicationTest(hives []Hive, joinChs []chan bool) {
	for i := range hives {
		hives[i].Stop()
		<-joinChs[i]
	}
}

func TestReplicationStrategy(t *testing.T) {
	hives, joinChs := startHivesForReplicationTest(t,
		[]string{"127.0.0.1:32771", "127.0.0.1:32772"}, func(h Hive) {})

	slaves := hives[1].ReplicationStrategy().SelectSlaveHives(MappedCells{}, 2)
	if len(slaves) != 1 {
		t.Errorf("Returned more slaves that asked: %+v", slaves)
	}

	if slaves[0] != hives[0].ID() {
		t.Errorf("Wrong slave selected %+v", hives[0].ID())
	}

	stopHivesForReplicationTest(hives, joinChs)
}

type replicatedTestAppMsg int

type replicatedTestApp struct {
	rcvCh chan bool
}

func (h *replicatedTestApp) Rcv(msg Msg, ctx RcvContext) error {
	h.rcvCh <- true
	return nil
}

func (h *replicatedTestApp) Map(msg Msg, ctx MapContext) MappedCells {
	return MappedCells{{"D", Key("K")}}
}

func TestReplicatedBee(t *testing.T) {
	rcvCh := make(chan bool)
	registerApp := func(h Hive) {
		app := h.NewApp("MyApp")
		app.Handle(replicatedTestAppMsg(0), &replicatedTestApp{rcvCh})
		app.SetReplicationFactor(3)
	}

	hives, joinChs := startHivesForReplicationTest(t,
		[]string{"127.0.0.1:32771", "127.0.0.1:32772", "127.0.0.1:32773"},
		registerApp)

	hives[0].Emit(replicatedTestAppMsg(0))
	<-rcvCh

	stopHivesForReplicationTest(hives, joinChs)
	for _, b := range hives[0].(*hive).apps["MyApp"].qee.idToBees {
		colony := b.colonyUnsafe()
		if len(colony.Slaves) != 2 {
			t.Errorf("Incorrect number of slaves for MyApp: %+v", colony)
		}
		fmt.Println(colony)
	}
}