package holochain

import (
	"bytes"
	gob "encoding/gob"
	"fmt"
	toml "github.com/BurntSushi/toml"
	"github.com/google/uuid"
	ic "github.com/libp2p/go-libp2p-crypto"
	. "github.com/smartystreets/goconvey/convey"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	Crash = true
	Initialize(nil)
	os.Exit(m.Run())
}

func TestNewHolochain(t *testing.T) {
	a, _ := NewAgent(IPFS, "Joe")

	Convey("New should fill Holochain struct with provided values and new UUID", t, func() {

		h := NewHolochain(a, "some/path", "json")
		nID := string(uuid.NodeID())
		So(nID, ShouldEqual, string(h.ID.NodeID()))
		So(h.agent.Name(), ShouldEqual, "Joe")
		So(h.agent.PrivKey(), ShouldEqual, a.PrivKey())
		So(h.encodingFormat, ShouldEqual, "json")
		So(h.rootPath, ShouldEqual, "some/path")
		So(h.UIPath(), ShouldEqual, "some/path/ui")
		So(h.DNAPath(), ShouldEqual, "some/path/dna")
		So(h.DBPath(), ShouldEqual, "some/path/db")
	})
	Convey("New with Zome should fill them", t, func() {
		z := Zome{Name: "zySampleZome",
			Description: "zome desc",
			Code:        "zome_zySampleZome.zy",
			Entries: map[string]EntryDef{
				"entryTypeFoo": {Name: "entryTypeFoo", DataFormat: DataFormatString},
				"entryTypeBar": {Name: "entryTypeBar", DataFormat: DataFormatRawZygo},
			},
		}

		h := NewHolochain(a, "some/path", "yaml")
		h.Zomes = map[string]Zome{z.Name: z}
		nz, _ := h.getZome("zySampleZome")
		So(nz.Description, ShouldEqual, "zome desc")
		So(nz.Code, ShouldEqual, "zome_zySampleZome.zy")
		So(fmt.Sprintf("%v", nz.Entries["entryTypeFoo"]), ShouldEqual, "{entryTypeFoo string    <nil>}")
		So(fmt.Sprintf("%v", nz.Entries["entryTypeBar"]), ShouldEqual, "{entryTypeBar zygo    <nil>}")
	})

}

func TestPrepare(t *testing.T) {
	Convey("it should fail if the requires version is incorrect", t, func() {
		h := Holochain{RequiresVersion: Version + 1}
		nextVersion := fmt.Sprintf("%d", Version+1)
		err := h.prepare()
		So(err.Error(), ShouldEqual, "Chain requires Holochain version "+nextVersion)

	})
	Convey("it should return no err if the requires version is correct", t, func() {
		cleanup, _, h := genTestChain("test")
		defer cleanup()
		h.RequiresVersion = Version
		err := h.prepare()
		So(err, ShouldBeNil)
	})
	//@todo build out test for other tests for prepare
}

func TestPrepareHashType(t *testing.T) {

	Convey("A bad hash type should return an error", t, func() {
		h := Holochain{HashType: "bogus"}
		err := h.prepareHashType()
		So(err.Error(), ShouldEqual, "Unknown hash type: bogus")
	})
	Convey("It should initialized fixed and variable sized hashes", t, func() {
		holo := &Holochain{HashType: "sha1"}
		err := holo.prepareHashType()
		So(err, ShouldBeNil)
		var hash Hash
		err = hash.Sum(holo, []byte("test data"))
		So(err, ShouldBeNil)
		So(hash.String(), ShouldEqual, "5duC28CW416wX42vses7TeTeRYwku9")

		holo.HashType = "blake2b-256"
		err = holo.prepareHashType()
		So(err, ShouldBeNil)
		err = hash.Sum(holo, []byte("test data"))
		So(err, ShouldBeNil)
		So(hash.String(), ShouldEqual, "2DrjgbL49zKmX4P7UgdopSCC7MhfVUySNbRHBQzdDuXgaJSNEg")
	})
}

func TestGenDev(t *testing.T) {
	cleanup, s := setupTestService()
	defer cleanup()
	name := "test"
	root := s.Path + "/" + name

	Convey("we detected unconfigured holochains", t, func() {
		f, err := s.IsConfigured(name)
		So(f, ShouldEqual, "")
		So(err.Error(), ShouldEqual, "No DNA file in "+root+"/"+ChainDNADir+"/")
		_, err = s.load("test", "json")
		So(err.Error(), ShouldEqual, "open "+root+"/"+ChainDNADir+"/"+DNAFileName+".json: no such file or directory")

	})

	Convey("when generating a dev holochain", t, func() {
		h, err := s.GenDev(root, "json")
		So(err, ShouldBeNil)

		f, err := s.IsConfigured(name)
		So(err, ShouldBeNil)
		So(f, ShouldEqual, "json")

		h, err = s.Load(name)
		So(err, ShouldBeNil)

		lh, err := s.load(name, "json")
		So(err, ShouldBeNil)
		So(lh.id, ShouldEqual, h.id)
		So(lh.config.Port, ShouldEqual, DefaultPort)
		So(h.config.PeerModeDHTNode, ShouldEqual, s.Settings.DefaultPeerModeDHTNode)
		So(h.config.PeerModeAuthor, ShouldEqual, s.Settings.DefaultPeerModeAuthor)
		So(h.config.BootstrapServer, ShouldEqual, s.Settings.DefaultBootstrapServer)

		So(fileExists(h.DNAPath()+"/zySampleZome/profile.json"), ShouldBeTrue)
		So(fileExists(h.UIPath()+"/index.html"), ShouldBeTrue)
		So(fileExists(h.UIPath()+"/hc.js"), ShouldBeTrue)
		So(fileExists(h.rootPath+"/"+ConfigFileName+".json"), ShouldBeTrue)

		Convey("we should not be able re generate it", func() {
			_, err = s.GenDev(root, "json")
			So(err.Error(), ShouldEqual, "holochain: "+root+" already exists")
		})
	})
}

func TestCloneNew(t *testing.T) {
	cleanup, s, h0 := genTestChain("test")
	defer cleanup()

	name := "test2"
	root := s.Path + "/" + name

	orig := s.Path + "/test"
	Convey("it should create a chain from the examples directory", t, func() {
		h, err := s.Clone(orig, root, true)
		So(err, ShouldBeNil)
		So(h.Name, ShouldEqual, "test2")
		So(h.ID, ShouldNotEqual, h0.ID)
		agent, err := LoadAgent(s.Path)
		So(err, ShouldBeNil)
		So(h.agent.Name(), ShouldEqual, agent.Name())
		So(ic.KeyEqual(h.agent.PrivKey(), agent.PrivKey()), ShouldBeTrue)
		src, _ := readFile(orig+"/dna/", "zySampleZome.zy")
		dst, _ := readFile(h.DNAPath(), "zySampleZome.zy")
		So(string(src), ShouldEqual, string(dst))
		So(h.rootPath, ShouldEqual, root)
		So(h.UIPath(), ShouldEqual, root+"/ui")
		So(h.DNAPath(), ShouldEqual, root+"/dna")
		So(h.DBPath(), ShouldEqual, root+"/db")

		So(fileExists(h.UIPath()+"/index.html"), ShouldBeTrue)
		So(fileExists(h.DNAPath()+"/zySampleZome/profile.json"), ShouldBeTrue)
		So(fileExists(h.DNAPath()+"/properties_schema.json"), ShouldBeTrue)
		So(fileExists(h.rootPath+"/"+ConfigFileName+".toml"), ShouldBeTrue)

		So(fileExists(h.rootPath+"/"+ChainTestDir+"/test_0.json"), ShouldBeTrue)

	})
}

func TestCloneJoin(t *testing.T) {
	cleanup, s, h0 := genTestChain("test")
	defer cleanup()

	name := "test2"
	root := s.Path + "/" + name

	orig := s.Path + "/test"
	Convey("it should create a chain from the examples directory", t, func() {
		h, err := s.Clone(orig, root, false)
		So(err, ShouldBeNil)
		So(h.Name, ShouldEqual, "test")
		So(h.ID, ShouldEqual, h0.ID)
		agent, err := LoadAgent(s.Path)
		So(err, ShouldBeNil)
		So(h.agent.Name(), ShouldEqual, agent.Name())
		So(ic.KeyEqual(h.agent.PrivKey(), agent.PrivKey()), ShouldBeTrue)
		src, _ := readFile(orig+"/dna/", "zySampleZome.zy")
		dst, _ := readFile(root, "zySampleZome.zy")
		So(string(src), ShouldEqual, string(dst))
		So(fileExists(h.UIPath()+"/index.html"), ShouldBeTrue)
		So(fileExists(h.DNAPath()+"/zySampleZome/profile.json"), ShouldBeTrue)
		So(fileExists(h.DNAPath()+"/properties_schema.json"), ShouldBeTrue)
		So(fileExists(h.rootPath+"/"+ConfigFileName+".toml"), ShouldBeTrue)
	})
}

func TestNewEntry(t *testing.T) {
	cleanup, s := setupTestService()
	defer cleanup()
	n := "test"
	path := s.Path + "/" + n
	h, err := s.GenDev(path, "toml")
	if err != nil {
		panic(err)
	}

	entryTypeFoo := `(message (from "art") (to "eric") (contents "test"))`

	now := time.Unix(1, 1) // pick a constant time so the test will always work

	e := EntryObj{C: entryTypeFoo}
	headerHash, header, err := h.NewEntry(now, "entryTypeFoo", &e)
	Convey("parameters passed in should be in the header", t, func() {
		So(err, ShouldBeNil)
		So(header.Time == now, ShouldBeTrue)
		So(header.Type, ShouldEqual, "entryTypeFoo")
		So(header.HeaderLink.IsNullHash(), ShouldBeTrue)
	})
	Convey("the entry hash is correct", t, func() {
		So(err, ShouldBeNil)
		So(header.EntryLink.String(), ShouldEqual, "QmdRXz53TVT9qBYfbXctHyy2GpTNa6YrpAy6ZcDGG8Xhc5")
	})

	// can't check against a fixed hash because signature created each time test runs is
	// different (though valid) so the header will hash to a different value
	Convey("the returned header hash is the SHA256 of the byte encoded header", t, func() {
		b, _ := header.Marshal(h)
		var hh Hash
		err = hh.Sum(h, b)
		So(err, ShouldBeNil)
		So(headerHash.String(), ShouldEqual, hh.String())
	})

	Convey("it should have signed the entry with my key", t, func() {
		sig := header.Sig
		hash := header.EntryLink.H
		valid, err := h.agent.PrivKey().GetPublic().Verify(hash, sig.S)
		So(err, ShouldBeNil)
		So(valid, ShouldBeTrue)
	})

	Convey("it should store the header and entry to the data store", t, func() {
		s1 := fmt.Sprintf("%v", *header)
		d1 := fmt.Sprintf("%v", entryTypeFoo)

		h2, err := h.chain.Get(headerHash)
		So(err, ShouldBeNil)
		s2 := fmt.Sprintf("%v", *h2)
		So(s2, ShouldEqual, s1)

		Convey("and the returned header should hash to the same value", func() {
			b, _ := (h2).Marshal(h)
			var hh Hash
			err = hh.Sum(h, b)
			So(err, ShouldBeNil)
			So(headerHash.String(), ShouldEqual, hh.String())
		})

		var d2 interface{}
		var d2t string
		d2, d2t, err = h.chain.GetEntry(h2.EntryLink)
		So(err, ShouldBeNil)
		So(d2t, ShouldEqual, "entryTypeFoo")

		So(d2, ShouldNotBeNil)
		So(d2.(Entry).Content(), ShouldEqual, d1)
	})

	Convey("Top should still work", t, func() {
		hash, err := h.Top()
		So(err, ShouldBeNil)
		So(hash.Equal(&headerHash), ShouldBeTrue)
	})

	e = EntryObj{C: "more data"}
	_, header2, err := h.NewEntry(now, "entryTypeFoo", &e)

	Convey("a second entry should have prev link correctly set", t, func() {
		So(err, ShouldBeNil)
		So(header2.HeaderLink.String(), ShouldEqual, headerHash.String())
	})
}

func TestHeader(t *testing.T) {
	var h1, h2 Header
	h1 = mkTestHeader("entryTypeFoo")

	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(&h1)
	Convey("it should encode", t, func() {
		So(err, ShouldBeNil)
	})

	dec := gob.NewDecoder(&buf)
	err = dec.Decode(&h2)

	Convey("it should decode", t, func() {
		s1 := fmt.Sprintf("%v", h1)
		s2 := fmt.Sprintf("%v", h2)
		So(err, ShouldBeNil)
		So(s1, ShouldEqual, s2)
	})
}

func TestGenChain(t *testing.T) {
	cleanup, _, holo := genTestChain("test")
	defer cleanup()

	var err error
	Convey("Generating DNA Hashes should re-save the DNA file", t, func() {
		err = holo.GenDNAHashes()
		So(err, ShouldBeNil)
		var h2 Holochain
		_, err = toml.DecodeFile(holo.DNAPath()+"/"+DNAFileName+".toml", &h2)
		So(err, ShouldBeNil)
		z2, _ := h2.getZome("zySampleZome")
		z1, _ := holo.getZome("zySampleZome")
		Debugf("Zome before: %v", z2)
		Debugf("Zome before: %v", z1)
		So(z2.CodeHash.String(), ShouldEqual, z1.CodeHash.String())
		execCmd("ls", "-ltR", holo.DNAPath())
		b, _ := readFile(holo.DNAPath()+"/zySampleZome", "profile.json")
		Debugf("pf.json %s", string(b))
		var sh Hash
		sh.Sum(holo, b)

		Debugf("D:%v", z2.Entries)
		So(z1.Entries["entryTypeFoo"].SchemaHash.String(), ShouldEqual, sh.String())
		So(z2.Entries["entryTypeFoo"].SchemaHash.String(), ShouldEqual, sh.String())
	})

	/*
		Convey("before GenChain call DNAHash call should fail", t, func() {
			h := holo.DNAHash()
			So(h.String(), ShouldEqual, "")
		})
		var headerHash Hash
		Convey("GenChain call works", t, func() {
			headerHash, err = holo.GenChain()
			So(err, ShouldBeNil)
		})

		var header Header
		Convey("top link should be Key entry", t, func() {
			hdr, err := holo.chain.Get(headerHash)
			So(err, ShouldBeNil)
			entry, _, err := holo.chain.GetEntry(hdr.EntryLink)
			So(err, ShouldBeNil)
			header = *hdr
			var a = entry.Content().(AgentEntry)
			So(a.Name, ShouldEqual, holo.agent.Name())
			//So(k.Key,ShouldEqual,"something?") // test that key got correctly retrieved
		})

		var dnaHash Hash
		Convey("next link should be the dna entry", t, func() {
			hdr, err := holo.chain.Get(header.HeaderLink)
			So(err, ShouldBeNil)
			entry, et, err := holo.chain.GetEntry(hdr.EntryLink)
			So(err, ShouldBeNil)
			So(et, ShouldEqual, DNAEntryType)

			var buf bytes.Buffer
			err = holo.EncodeDNA(&buf)
			So(err, ShouldBeNil)
			So(string(entry.Content().([]byte)), ShouldEqual, buf.String())
			dnaHash = hdr.EntryLink
		})

		Convey("holochain id and top should have now been set", t, func() {
			id := holo.DNAHash()
			So(err, ShouldBeNil)
			So(id.String(), ShouldEqual, dnaHash.String())
			top, err := holo.Top()
			So(err, ShouldBeNil)
			So(top.String(), ShouldEqual, headerHash.String())
		})
	*/
}

func TestWalk(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()

	// add an extra link onto the chain
	entryTypeFoo := `(message (from "art") (to "eric") (contents "test"))`
	now := time.Unix(1, 1) // pick a constant time so the test will always work
	e := EntryObj{C: entryTypeFoo}
	_, _, err := h.NewEntry(now, "entryTypeFoo", &e)
	if err != nil {
		panic(err)
	}

	Convey("walk should call a function on all the elements of a chain", t, func() {

		c := make(map[int]string, 0)
		//	c := make([]string,0)
		idx := 0
		err := h.Walk(func(key *Hash, header *Header, entry Entry) (err error) {
			c[idx] = header.EntryLink.String()
			idx++
			//	c = append(c, header.HeaderLink.String())
			return nil
		}, false)
		So(err, ShouldBeNil)
		id := h.DNAHash()
		So(c[2], ShouldEqual, id.String())
		//	So(c,ShouldEqual,"fish")
	})
}

func TestValidate(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()

	// add an extra link onto the chain
	entryTypeFoo := `(message (from "art") (to "eric") (contents "test"))`
	now := time.Unix(1, 1) // pick a constant time so the test will always work
	e := EntryObj{C: entryTypeFoo}
	_, _, err := h.NewEntry(now, "entryTypeFoo", &e)
	if err != nil {
		panic(err)
	}
	Convey("validate should check the hashes of the headers, and optionally of the entries", t, func() {
		//	Convey("This isn't yet fully implemented", nil)
		valid, err := h.Validate(false)
		So(err, ShouldBeNil)
		So(valid, ShouldEqual, true)
	})
}

func TestValidatePrepare(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()

	Convey("it should fail if a validator doesn't exist for the entry type", t, func() {
		hdr := mkTestHeader("bogusType")
		d, _, n, err := h.validatePrepare(hdr.Type, &EntryObj{C: "foo"}, []peer.ID{h.id})
		So(err.Error(), ShouldEqual, "no definition for entry type: bogusType")
		So(d, ShouldBeNil)
		So(n, ShouldBeNil)
	})
	Convey("a nil entry is invalid", t, func() {
		hdr := mkTestHeader("evenNumbers")
		_, _, _, err := h.validatePrepare(hdr.Type, nil, []peer.ID{h.id})
		So(err.Error(), ShouldEqual, "nil entry invalid")
	})

	profile := `{"firstName":"Eric","lastName":"H-B"}`
	hdr := mkTestHeader("profile")
	h.prepare()
	Convey("successful prepare should convert sources and return a nucleus", t, func() {
		_, srcs, _, err := h.validatePrepare(hdr.Type, &EntryObj{C: profile}, []peer.ID{h.id})
		So(err, ShouldBeNil)
		So(fmt.Sprintf("%v", srcs), ShouldEqual, "["+peer.IDB58Encode(h.id)+"]")
	})

	Convey("validate on a schema based entry should check entry against the schema", t, func() {
		profile = `{"firstName":"Eric"}` // missing required lastName
		_, _, _, err := h.validatePrepare(hdr.Type, &EntryObj{C: profile}, []peer.ID{h.id})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "validator profile.json failed: object property 'lastName' is required")
	})

	hdr = mkTestHeader("rating")
	Convey("validate on a links entry should fail if not formatted correctly", t, func() {
		_, _, _, err := h.validatePrepare(hdr.Type, &EntryObj{C: "badjson"}, []peer.ID{h.id})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "invalid links entry, invalid json: invalid character 'b' looking for beginning of value")

		_, _, _, err = h.validatePrepare(hdr.Type, &EntryObj{C: `{}`}, []peer.ID{h.id})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "invalid links entry: you must specify at least one link")

		_, _, _, err = h.validatePrepare(hdr.Type, &EntryObj{C: `{"Links":[{}]}`}, []peer.ID{h.id})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "invalid links entry: missing Base")

		_, _, _, err = h.validatePrepare(hdr.Type, &EntryObj{C: `{"Links":[{"Base":"x","Link":"x","Tag":"sometag"}]}`}, []peer.ID{h.id})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "invalid links entry: Base multihash too short. must be > 3 bytes")
		_, _, _, err = h.validatePrepare(hdr.Type, &EntryObj{C: `{"Links":[{"Base":"QmdRXz53TVT9qBYfbXctHyy2GpTNa6YrpAy6ZcDGG8Xhc5","Link":"x","Tag":"sometag"}]}`}, []peer.ID{h.id})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "invalid links entry: Link multihash too short. must be > 3 bytes")

		_, _, _, err = h.validatePrepare(hdr.Type, &EntryObj{C: `{"Links":[{"Base":"QmdRXz53TVT9qBYfbXctHyy2GpTNa6YrpAy6ZcDGG8Xhc5","Link":"QmdRXz53TVT9qBYfbXctHyy2GpTNa6YrpAy6ZcDGG8Xhc5"}]}`}, []peer.ID{h.id})
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldEqual, "invalid links entry: missing Tag")
	})
}

func TestValidateCommit(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()
	var err error

	Convey("it should fail if a validator doesn't exist for the entry type", t, func() {
		hdr := mkTestHeader("bogusType")
		_, err = h.ValidateCommit(hdr.Type, &EntryObj{C: "foo"}, &hdr, []peer.ID{h.id})
		So(err.Error(), ShouldEqual, "no definition for entry type: bogusType")
	})

	Convey("a valid entry validates", t, func() {
		hdr := mkTestHeader("evenNumbers")
		var d *EntryDef
		d, err = h.ValidateCommit(hdr.Type, &EntryObj{C: "2"}, &hdr, []peer.ID{h.id})
		So(err, ShouldBeNil)
		So(fmt.Sprintf("%v", d), ShouldEqual, "&{evenNumbers zygo   public <nil>}")
	})
	Convey("an invalid entry doesn't validate", t, func() {
		hdr := mkTestHeader("evenNumbers")
		_, err = h.ValidateCommit(hdr.Type, &EntryObj{C: "1"}, &hdr, []peer.ID{h.id})
		So(err.Error(), ShouldEqual, "Invalid entry: 1")
	})
}

func TestGetZome(t *testing.T) {
	cleanup, _, h := genTestChain("test")
	defer cleanup()
	Convey("it should fail if the zome isn't defined in the DNA", t, func() {
		_, err := h.getZome("bogusZome")
		So(err.Error(), ShouldEqual, "unknown zome: bogusZome")
	})
	Convey("it should return the Zome structure of a defined zome", t, func() {
		z, err := h.getZome("zySampleZome")
		So(err, ShouldBeNil)
		So(z.Name, ShouldEqual, "zySampleZome")
	})
}

func TestGetFunctionDef(t *testing.T) {
	cleanup, _, h := genTestChain("test")
	defer cleanup()
	z, _ := h.getZome("zySampleZome")

	Convey("it should fail if the fn isn't defined in the DNA", t, func() {
		_, err := h.GetFunctionDef(z, "foo")
		So(err.Error(), ShouldEqual, "unknown exposed function: foo")
	})
	Convey("it should return the Fn structure of a defined fn", t, func() {
		fn, err := h.GetFunctionDef(z, "getDNA")
		So(err, ShouldBeNil)
		So(fn.Name, ShouldEqual, "getDNA")
	})
}

func TestMakeNucleus(t *testing.T) {
	cleanup, _, h := genTestChain("test")
	defer cleanup()
	Convey("it should fail if the zome isn't defined in the DNA", t, func() {
		_, _, err := h.MakeNucleus("bogusZome")
		So(err.Error(), ShouldEqual, "unknown zome: bogusZome")
	})
	Convey("it should make a nucleus based on the type and return the zome def", t, func() {
		v, zome, err := h.MakeNucleus("zySampleZome")
		So(err, ShouldBeNil)
		So(zome.Name, ShouldEqual, "zySampleZome")
		z := v.(*ZygoNucleus)
		_, err = z.env.Run()
		So(err, ShouldBeNil)
	})
}

func TestCall(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()
	Convey("it should call the exposed function", t, func() {
		result, err := h.Call("zySampleZome", "testStrFn1", "arg1 arg2")
		So(err, ShouldBeNil)
		So(result.(string), ShouldEqual, "result: arg1 arg2")

		result, err = h.Call("zySampleZome", "addEven", "42")
		So(err, ShouldBeNil)

		ph := h.chain.Top().EntryLink
		So(result.(string), ShouldEqual, ph.String())

		//_, err = h.Call("zySampleZome", "addEven", "41")
		//So(err.Error(), ShouldEqual, "Error calling 'commit': Invalid entry: 41")
	})
}

func TestLoadTestFiles(t *testing.T) {
	cleanup, _, h := genTestChain("test")
	defer cleanup()

	Convey("it should fail if there's no test data", t, func() {
		d := h.DBPath()
		tests, err := LoadTestFiles(d)
		So(tests, ShouldBeNil)
		So(err.Error(), ShouldEqual, "no test files found in: "+d)
	})

	Convey("it should load test files", t, func() {
		path := h.rootPath + "/" + ChainTestDir
		tests, err := LoadTestFiles(path)
		So(err, ShouldBeNil)
		So(len(tests), ShouldEqual, 8)
	})

}

func TestCommit(t *testing.T) {
	cleanup, _, h := prepareTestChain("test")
	defer cleanup()

	// add an entry onto the chain
	hash := commit(h, "oddNumbers", "7")

	if err := h.dht.simHandleChangeReqs(); err != nil {
		panic(err)
	}

	Convey("publicly shared entries should generate a put", t, func() {
		err := h.dht.exists(hash)
		So(err, ShouldBeNil)
	})

	profileHash := commit(h, "profile", `{"firstName":"Zippy","lastName":"Pinhead"}`)

	Convey("it should attach links after commit of Links entry", t, func() {
		commit(h, "rating", fmt.Sprintf(`{"Links":[{"Base":"%s","Link":"%s","Tag":"4stars"}]}`, hash.String(), profileHash.String()))

		if err := h.dht.simHandleChangeReqs(); err != nil {
			panic(err)
		}
		results, err := h.dht.getLink(hash, "4stars")
		So(err, ShouldBeNil)
		So(fmt.Sprintf("%v", results), ShouldEqual, "[{QmYeinX5vhuA91D3v24YbgyLofw9QAxY6PoATrBHnRwbtt }]")
	})
}

func commit(h *Holochain, entryType, entryStr string) (entryHash Hash) {
	entry := GobEntry{C: entryStr}

	r, err := NewCommitAction(entryType, &entry).Do(h)
	if err != nil {
		return
	}
	if r != nil {
		entryHash = r.(Hash)
	}
	if err != nil {
		panic(err)
	}
	return
}
