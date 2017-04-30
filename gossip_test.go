package holochain

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

/*
@TODO add setup for gossip that adds entry and meta entry so we have something
to gossip about.  Currently test is DHTReceiver test

func TestGossipReceiver(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()
	h.dht.SetupDHT()

}*/

func TestGetFindGossiper(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()
	dht := h.dht
	Convey("FindGossiper should start empty", t, func() {
		_, err := dht.FindGossiper()
		So(err, ShouldEqual, ErrDHTErrNoGossipersAvailable)

	})

	fooAddr, _ := makePeer("peer_foo")

	Convey("UpdateGossiperIdx should add a gossiper", t, func() {
		err := dht.UpdateGossiperIdx(fooAddr, 92)
		So(err, ShouldBeNil)
	})

	Convey("GetGossiperIdx should return the gossiper idx", t, func() {
		idx, err := dht.GetGossiperIdx(fooAddr)
		So(err, ShouldBeNil)
		So(idx, ShouldEqual, 92)
	})

	Convey("FindGossiper should return the gossiper", t, func() {
		g, err := dht.FindGossiper()
		So(err, ShouldBeNil)
		So(g.Idx, ShouldEqual, 92)
		So(g.Id, ShouldEqual, fooAddr)
	})

	Convey("GetIdx for self should be 3 to start with", t, func() {
		idx, err := dht.GetIdx()
		So(err, ShouldBeNil)
		So(idx, ShouldEqual, 3)
	})

	barAddr, _ := makePeer("peer_bar")

	Convey("GetGossiperIdx should return 0 for unknown gossiper", t, func() {
		idx, err := dht.GetGossiperIdx(barAddr)
		So(err, ShouldBeNil)
		So(idx, ShouldEqual, 0)
	})

}

func TestGossipData(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()
	dht := h.dht
	Convey("Idx should be 3 at start (first puts are DNA, Agent & Key)", t, func() {
		var idx int
		idx, err := dht.GetIdx()
		So(err, ShouldBeNil)
		So(idx, ShouldEqual, 3)
	})

	// simulate a handled put request
	now := time.Unix(1, 1) // pick a constant time so the test will always work
	e := EntryObj{C: "124"}
	_, hd, _ := h.NewEntry(now, "evenNumbers", &e)
	hash := hd.EntryLink
	m1 := h.node.NewMessage(PUT_REQUEST, PutReq{H: hash})

	Convey("fingerprints for messages should not exist", t, func() {
		f, _ := m1.Fingerprint()
		r, _ := dht.HaveFingerprint(f)
		So(r, ShouldBeFalse)
	})
	DHTReceiver(h, m1)
	dht.simHandleChangeReqs()

	someData := `{"firstName":"Zippy","lastName":"Pinhead"}`
	e = EntryObj{C: someData}
	_, hd, _ = h.NewEntry(now, "profile", &e)
	profileHash := hd.EntryLink

	ee := EntryObj{C: fmt.Sprintf(`{"Links":[{"Base":"%s"},{"Link":"%s"},{"Tag":"4stars"}]}`, hash.String(), profileHash.String())}
	_, le, _ := h.NewEntry(time.Now(), "rating", &ee)
	lr := LinkReq{Base: hash, Links: le.EntryLink}

	m2 := h.node.NewMessage(LINK_REQUEST, lr)
	DHTReceiver(h, m2)
	h.dht.simHandleChangeReqs()

	Convey("fingerprints for messages should exist", t, func() {
		f, _ := m1.Fingerprint()
		r, _ := dht.HaveFingerprint(f)
		So(r, ShouldBeTrue)
		f, _ = m1.Fingerprint()
		r, _ = dht.HaveFingerprint(f)
		So(r, ShouldBeTrue)
	})

	Convey("Idx should be 5 after puts", t, func() {
		var idx int
		idx, err := dht.GetIdx()
		So(err, ShouldBeNil)
		So(idx, ShouldEqual, 5)
	})

	Convey("GetPuts should return a list of the puts since an index value", t, func() {
		puts, err := dht.GetPuts(0)
		So(err, ShouldBeNil)
		So(len(puts), ShouldEqual, 5)
		So(fmt.Sprintf("%v", puts[3].M), ShouldEqual, fmt.Sprintf("%v", *m1))
		So(fmt.Sprintf("%v", puts[4].M), ShouldEqual, fmt.Sprintf("%v", *m2))

		puts, err = dht.GetPuts(5)
		So(err, ShouldBeNil)
		So(len(puts), ShouldEqual, 1)
		So(fmt.Sprintf("%v", puts[0].M), ShouldEqual, fmt.Sprintf("%v", *m2))
	})
}

func TestGossip(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()
	dht := h.dht

	idx, _ := dht.GetIdx()
	dht.UpdateGossiperIdx(h.node.HashAddr, idx)

	Convey("gossip should send a request", t, func() {
		var err error
		err = dht.gossip()
		So(err, ShouldBeNil)
	})
}
