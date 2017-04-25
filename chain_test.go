package holochain

import (
	"bytes"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"reflect"
	"testing"
)

func TestNewChain(t *testing.T) {
	holo := Holochain{HashType: HASH_SHA, WireType: WIRE_GOB}
	Convey("it should make an empty chain", t, func() {
		holo.NewChain()
		So(len(holo.chain.Headers), ShouldEqual, 0)
		So(len(holo.chain.Entries), ShouldEqual, 0)
	})

}

func TestNewChainFromFile(t *testing.T) {
	holo, cleanup, key, now := setupTestChainDir()
	defer cleanup()

	var err error
	Convey("it should make an empty chain with encoder", t, func() {
		err = holo.NewChainFromFile()
		So(err, ShouldBeNil)
		So(holo.chain.s, ShouldNotBeNil)
		So(fileExists(holo.DBPath()+"/chain.db"), ShouldBeTrue)
	})

	e := EntryObj{C: "some data1"}
	holo.AddEntry(now, "entryTypeFoo1", &e, key)
	e = EntryObj{C: "some other data2"}
	holo.AddEntry(now, "entryTypeFoo2", &e, key)
	dump := holo.chain.String()
	dir := holo.rootPath
	holo.chain.s.Close()
	err = holo.NewChainFromFile()
	holo.rootPath = dir
	Convey("it should load chain data if available", t, func() {
		So(err, ShouldBeNil)
		So(holo.chain.String(), ShouldEqual, dump)
	})

	e = EntryObj{C: "yet other data"}
	holo.AddEntry(now, "yourData", &e, key)
	dump = holo.chain.String()
	holo.chain.s.Close()

	err = holo.NewChainFromFile()
	Convey("should continue to append data after reload", t, func() {
		So(err, ShouldBeNil)
		So(holo.chain.String(), ShouldEqual, dump)
	})
}

func TestTop(t *testing.T) {
	holo, cleanup, key, now := setupTestChainDir()
	defer cleanup()
	var hash *Hash
	var hd *Header

	holo_new := Holochain{HashType: HASH_SHA, WireType: WIRE_GOB}
	holo_new.NewChain()
	Convey("it should return an nil for an empty chain", t, func() {
		hd = holo_new.chain.Top()
		So(hd, ShouldBeNil)
		hash, hd = holo_new.chain.TopType("entryTypeFoo")
		So(hd, ShouldBeNil)
		So(hash, ShouldBeNil)
	})

	e := EntryObj{C: "some data"}
	holo.AddEntry(now, "entryTypeFoo", &e, key)
	Convey("Top it should return the top header", t, func() {
		hd = holo.chain.Top()
		So(hd, ShouldEqual, holo.chain.Headers[0])
	})

	Convey("TopType should return nil for non existent type", t, func() {
		hash, hd = holo.chain.TopType("otherData")
		So(hd, ShouldBeNil)
		So(hash, ShouldEqual, nil)
	})
	Convey("TopType should return header for correct type", t, func() {
		hash, hd = holo.chain.TopType("entryTypeFoo")
		So(hd, ShouldEqual, holo.chain.Headers[0])
	})

	holo.AddEntry(now, "otherData", &e, key)
	Convey("TopType should return headers for both types", t, func() {
		hash, hd = holo.chain.TopType("entryTypeFoo")
		So(hd, ShouldEqual, holo.chain.Headers[0])
		hash, hd = holo.chain.TopType("otherData")
		So(hd, ShouldEqual, holo.chain.Headers[1])
	})

	Convey("Nth should return the nth header", t, func() {
		hd = holo.chain.Nth(1)
		So(hd, ShouldEqual, holo.chain.Headers[0])
	})
}

func TestTopType(t *testing.T) {
	holo := Holochain{HashType: HASH_SHA, WireType: WIRE_GOB}
	holo.NewChain()
	//holo, cleanup, _, _ := setupTestChainDir()
	//defer cleanup()
	var hash *Hash
	var hd *Header
	Convey("it should return nil for an empty chain", t, func() {
		hash, hd = holo.chain.TopType("entryTypeFoo")
		So(hd, ShouldBeNil)
		So(hash, ShouldEqual, nil)
	})
	Convey("it should return nil for an chain with no entries of the type", t, func() {
	})
}

func TestAddEntry(t *testing.T) {
	holo, cleanup, key, now := setupTestChainDir()
	defer cleanup()
	//holo, _, key, now := chainTestSetup("")

	Convey("it should add nil to the chain", t, func() {
		e := EntryObj{C: "some data"}
		hash, err := holo.AddEntry(now, "entryTypeFoo", &e, key)
		So(err, ShouldBeNil)
		So(len(holo.chain.Headers), ShouldEqual, 1)
		So(len(holo.chain.Entries), ShouldEqual, 1)
		So(holo.chain.TypeTops["entryTypeFoo"], ShouldEqual, 0)
		So(hash.Equal(&holo.chain.Hashes[0]), ShouldBeTrue)
	})
}

func TestGet(t *testing.T) {
	holo, cleanup, key, now := setupTestChainDir()
	defer cleanup()

	e1 := EntryObj{C: "some data"}
	h1, _ := holo.AddEntry(now, "entryTypeFoo", &e1, key)
	hd1, err1 := holo.chain.Get(h1)

	e2 := EntryObj{C: "some other data"}
	h2, _ := holo.AddEntry(now, "entryTypeFoo", &e2, key)
	hd2, err2 := holo.chain.Get(h2)

	Convey("it should get header by hash or by Entry hash", t, func() {
		So(hd1, ShouldEqual, holo.chain.Headers[0])
		So(err1, ShouldBeNil)

		ehd, err := holo.chain.GetEntryHeader(hd1.EntryLink)
		So(ehd, ShouldEqual, holo.chain.Headers[0])
		So(err, ShouldBeNil)

		So(hd2, ShouldEqual, holo.chain.Headers[1])
		So(err2, ShouldBeNil)

		ehd, err = holo.chain.GetEntryHeader(hd2.EntryLink)
		So(ehd, ShouldEqual, holo.chain.Headers[1])
		So(err, ShouldBeNil)
	})

	Convey("it should get entry by hash", t, func() {
		ed, et, err := holo.chain.GetEntry(hd1.EntryLink)
		So(err, ShouldBeNil)
		So(et, ShouldEqual, "entryTypeFoo")
		So(fmt.Sprintf("%v", &e1), ShouldEqual, fmt.Sprintf("%v", ed))
		ed, et, err = holo.chain.GetEntry(hd2.EntryLink)
		So(err, ShouldBeNil)
		So(et, ShouldEqual, "entryTypeFoo")
		So(fmt.Sprintf("%v", &e2), ShouldEqual, fmt.Sprintf("%v", ed))
	})

	Convey("it should return nil for non existent hash", t, func() {
		hash, _ := NewHash("QmNiCwBNA8MWDADTFVq1BonUEJbS2SvjAoNkZZrhEwcuUi")
		hd, err := holo.chain.Get(hash)
		So(hd, ShouldBeNil)
		So(err, ShouldEqual, ErrHashNotFound)
	})
}

func TestMarshal(t *testing.T) {
	holo, cleanup, key, now := setupTestChainDir()
	defer cleanup()

	e := EntryObj{C: "some data"}
	holo.AddEntry(now, "entryTypeFoo1", &e, key)

	e = EntryObj{C: "some other data"}
	holo.AddEntry(now, "entryTypeFoo2", &e, key)

	e = EntryObj{C: "and more data"}
	holo.AddEntry(now, "entryTypeFoo3", &e, key)

	Convey("it should be able to marshal and unmarshal", t, func() {
		var before, after bytes.Buffer

		err := holo.MarshalChain(&before)
		chain_before := holo.chain.String()
		tt_before := holo.chain.TypeTops
		hmap_before := holo.chain.Hmap
		emap_before := holo.chain.Emap
		hashes_before := []string{}
		for i := 0; i < len(holo.chain.Headers); i++ {
			hashes_before = append(hashes_before, holo.chain.Hashes[i].String())
		}
		So(err, ShouldBeNil)
		err = holo.UnmarshalChain(&after)
		So(err, ShouldBeNil)
		So(holo.chain.String(), ShouldEqual, chain_before)

		// confirm that internal structures are properly set up
		for i := 0; i < len(holo.chain.Headers); i++ {
			So(holo.chain.Hashes[i].String(), ShouldEqual, hashes_before[i])
		}
		So(reflect.DeepEqual(holo.chain.TypeTops, tt_before), ShouldBeTrue)
		So(reflect.DeepEqual(holo.chain.Hmap, hmap_before), ShouldBeTrue)
		So(reflect.DeepEqual(holo.chain.Emap, emap_before), ShouldBeTrue)
	})
}

func TestWalkChain(t *testing.T) {
	holo, cleanup, key, now := setupTestChainDir()
	defer cleanup()

	e := EntryObj{C: "some data"}
	holo.AddEntry(now, "entryTypeFoo1", &e, key)

	e = EntryObj{C: "some other data"}
	holo.AddEntry(now, "entryTypeFoo2", &e, key)

	e = EntryObj{C: "and more data"}
	holo.AddEntry(now, "entryTypeFoo3", &e, key)

	Convey("it should walk back from the top through all entries", t, func() {
		var x string
		var i int
		err := holo.chain.Walk(func(key *Hash, h *Header, entry Entry) error {
			i++
			x += fmt.Sprintf("%d:%v ", i, entry.(*EntryObj).C)
			return nil
		})
		So(err, ShouldBeNil)
		So(x, ShouldEqual, "1:and more data 2:some other data 3:some data ")
	})
}

func TestValidateChain(t *testing.T) {
	holo, cleanup, key, now := setupTestChainDir()
	defer cleanup()

	e := EntryObj{C: "some data"}
	holo.AddEntry(now, "entryTypeFoo1", &e, key)

	e = EntryObj{C: "some other data"}
	holo.AddEntry(now, "entryTypeFoo1", &e, key)

	e = EntryObj{C: "and more data"}
	holo.AddEntry(now, "entryTypeFoo1", &e, key)

	Convey("it should validate", t, func() {
		So(holo.ValidateChain(), ShouldBeNil)
	})

	Convey("it should fail to validate if we diddle some bits", t, func() {
		holo.chain.Entries[0].(*EntryObj).C = "fish"
		So(holo.ValidateChain().Error(), ShouldEqual, "entry hash mismatch at link 0")
		holo.chain.Entries[0].(*EntryObj).C = "some data"
		holo.chain.Headers[1].TypeLink = NullHash()
		So(holo.ValidateChain().Error(), ShouldEqual, "header hash mismatch at link 1")
	})
}

/*
func TestPersistingChain(t *testing.T) {
	holo := Holochain{HashType: HASH_SHA, WireType: WIRE_GOB}
	var b bytes.Buffer
	holo.chain.encoder = gob.NewEncoder(&b)

	holo, h, key, now := chainTestSetup("")
	e := EntryObj{C: "some data"}
	holo.AddEntry(now, "entryTypeFoo1", &e, key)

	e = EntryObj{C: "some other data"}
	holo.AddEntry(now, "entryTypeFoo1", &e, key)

	e = EntryObj{C: "and more data"}
	holo.AddEntry(now, "entryTypeFoo1", &e, key)

	dec := gob.NewDecoder(&b)

	var header *Header
	var entry Entry
	header, entry, err := readPair(dec)

	Convey("it should have added items to the writer", t, func() {
		So(err, ShouldBeNil)
		So(fmt.Sprintf("%v", header), ShouldEqual, fmt.Sprintf("%v", holo.chain.Headers[0]))
		So(fmt.Sprintf("%v", entry), ShouldEqual, fmt.Sprintf("%v", holo.chain.Entries[0]))
	})
}
*/
