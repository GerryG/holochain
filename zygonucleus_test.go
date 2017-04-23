package holochain

import (
	"fmt"
	zygo "github.com/glycerine/zygomys/repl"
	peer "github.com/libp2p/go-libp2p-peer"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestNewZygoNucleus(t *testing.T) {
	Convey("new should create a nucleus", t, func() {
		v, err := NewZygoNucleus(nil, `(+ 1 1)`)
		z := v.(*ZygoNucleus)
		So(err, ShouldBeNil)
		So(z.lastResult.(*zygo.SexpInt).Val, ShouldEqual, 2)
	})
	Convey("new fail to create nucleus when code is bad", t, func() {
		v, err := NewZygoNucleus(nil, "(should make a zygo syntax error")
		So(v, ShouldBeNil)
		So(err.Error(), ShouldEqual, "Zygomys load error: Error on line 1: parser needs more input\n")
	})

	Convey("it should have an App structure:", t, func() {
		d, _, h := prepareTestChain("test")
		defer cleanupTestDir(d)

		v, err := NewZygoNucleus(h, "")
		So(err, ShouldBeNil)
		z := v.(*ZygoNucleus)

		_, err = z.Run("App_Name")
		So(err, ShouldBeNil)
		s := z.lastResult.(*zygo.SexpStr).S
		So(s, ShouldEqual, h.Name)
		_, err = z.Run("App_DNA_Hash")
		So(err, ShouldBeNil)
		s = z.lastResult.(*zygo.SexpStr).S
		So(s, ShouldEqual, h.dnaHash.String())
		_, err = z.Run("App_Agent_Hash")
		So(err, ShouldBeNil)
		s = z.lastResult.(*zygo.SexpStr).S
		So(s, ShouldEqual, h.agentHash.String())

		_, err = z.Run("App_Agent_String")
		So(err, ShouldBeNil)
		s = z.lastResult.(*zygo.SexpStr).S
		So(s, ShouldEqual, h.Agent().Name())

		_, err = z.Run("App_Key_Hash")
		So(err, ShouldBeNil)
		s = z.lastResult.(*zygo.SexpStr).S
		So(s, ShouldEqual, peer.IDB58Encode(h.id))
	})

	Convey("it should have an App structure:", t, func() {
		d, _, h := prepareTestChain("test")
		defer cleanupTestDir(d)

		v, err := NewZygoNucleus(h, "")
		So(err, ShouldBeNil)
		z := v.(*ZygoNucleus)

		_, err = z.Run("HC_Version")
		So(err, ShouldBeNil)
		s := z.lastResult.(*zygo.SexpStr).S
		So(s, ShouldEqual, VersionStr)
	})

	Convey("should have the built in functions:", t, func() {
		d, _, h := prepareTestChain("test")
		defer cleanupTestDir(d)

		v, err := NewZygoNucleus(h, "")
		So(err, ShouldBeNil)
		z := v.(*ZygoNucleus)

		Convey("atoi", func() {
			_, err = z.Run(`(atoi "3141")`)
			So(err, ShouldBeNil)
			So(z.lastResult.(*zygo.SexpInt).Val, ShouldEqual, 3141)
			_, err = z.Run(`(atoi 1)`)
			So(err.Error(), ShouldEqual, "Zygomys exec error: Error calling 'atoi': argument to atoi should be string")
		})
		Convey("isprime", func() {
			_, err = z.Run(`(isprime 100)`)
			So(err, ShouldBeNil)
			So(z.lastResult.(*zygo.SexpBool).Val, ShouldEqual, false)
			_, err = z.Run(`(isprime 7)`)
			So(err, ShouldBeNil)
			So(z.lastResult.(*zygo.SexpBool).Val, ShouldEqual, true)
			_, err = z.Run(`(isprime "fish")`)
			So(err.Error(), ShouldEqual, "Zygomys exec error: Error calling 'isprime': argument to isprime should be int")
		})
		Convey("property", func() {
			_, err = z.Run(`(property "description")`)
			So(err, ShouldBeNil)
			So(z.lastResult.(*zygo.SexpStr).S, ShouldEqual, "a bogus test holochain")

			ShouldLog(&infoLog, "Warning: Getting special properties via property() is deprecated as of 3. Returning nil values.  Use App* instead\n", func() {
				_, err = z.Run(`(property "` + ID_PROPERTY + `")`)
				So(err, ShouldBeNil)
			})

		})
	})
}

func TestZygoGenesis(t *testing.T) {
	Convey("it should fail if the genesis function returns false", t, func() {
		z, _ := NewZygoNucleus(nil, `(defn genesis [] false)`)
		err := z.ChainGenesis()
		So(err.Error(), ShouldEqual, "genesis failed")
	})
	Convey("it should work if the genesis function returns true", t, func() {
		z, _ := NewZygoNucleus(nil, `(defn genesis [] true)`)
		err := z.ChainGenesis()
		So(err, ShouldBeNil)
	})
}

func TestZygoValidateCommit(t *testing.T) {
	a, _ := NewAgent(IPFS, "Joe")
	h := NewHolochain(a, "some/path", "yaml")
	h.Zomes = []Zome{}
	h.config.Loggers.App.New(nil)
	hdr := mkTestHeader("evenNumbers")

	Convey("it should be passing in the correct values", t, func() {
		v, err := NewZygoNucleus(&h, `(defn validateCommit [name entry header sources] (debug name) (debug entry) (debug header) (debug sources) true)`)
		So(err, ShouldBeNil)
		d := EntryDef{Name: "evenNumbers", DataFormat: DataFormatString}
		ShouldLog(&h.config.Loggers.App, `evenNumbers
foo
{"Atype":"hash", "EntryLink":"QmNiCwBNA8MWDADTFVq1BonUEJbS2SvjAoNkZZrhEwcuU2", "Type":"evenNumbers", "Time":"1970-01-01T00:00:01Z", "zKeyOrder":["EntryLink", "Type", "Time"]}
["fakehashvalue"]
`, func() {
			err = v.ValidateCommit(&d, &GobEntry{C: "foo"}, &hdr, []string{"fakehashvalue"})
			So(err, ShouldBeNil)
		})
	})
	Convey("should run an entry value against the defined validator for string data", t, func() {
		v, err := NewZygoNucleus(nil, `(defn validateCommit [name entry header sources] (cond (== entry "fish") true false))`)
		d := EntryDef{Name: "evenNumbers", DataFormat: DataFormatString}
		err = v.ValidateCommit(&d, &GobEntry{C: "cow"}, nil, nil)
		So(err.Error(), ShouldEqual, "Invalid entry: cow")
		err = v.ValidateCommit(&d, &GobEntry{C: "fish"}, nil, nil)
		So(err, ShouldBeNil)
	})
	Convey("should run an entry value against the defined validator for zygo data", t, func() {
		v, err := NewZygoNucleus(nil, `(defn validateCommit [name entry header sources] (cond (== entry "fish") true false))`)
		d := EntryDef{Name: "evenNumbers", DataFormat: DataFormatRawZygo}
		err = v.ValidateCommit(&d, &GobEntry{C: "\"cow\""}, nil, nil)
		So(err.Error(), ShouldEqual, "Invalid entry: \"cow\"")
		err = v.ValidateCommit(&d, &GobEntry{C: "\"fish\""}, nil, nil)
		So(err, ShouldBeNil)
	})
	Convey("should run an entry value against the defined validator for json data", t, func() {
		v, err := NewZygoNucleus(nil, `(defn validateCommit [name entry header sources] (cond (== (hget entry data:) "fish") true false))`)
		d := EntryDef{Name: "evenNumbers", DataFormat: DataFormatJSON}
		err = v.ValidateCommit(&d, &GobEntry{C: `{"data":"cow"}`}, nil, nil)
		So(err.Error(), ShouldEqual, `Invalid entry: {"data":"cow"}`)
		err = v.ValidateCommit(&d, &GobEntry{C: `{"data":"fish"}`}, nil, nil)
		So(err, ShouldBeNil)
	})
}

func TestZygoValidateLink(t *testing.T) {
	a, _ := NewAgent(IPFS, "Joe")
	h := NewHolochain(a, "some/path", "yaml")
	h.Zomes = []Zome{}
	h.config.Loggers.App.New(nil)

	Convey("it should be passing in the correct values", t, func() {
		v, err := NewZygoNucleus(&h, `(defn validateLink [linkingEntryType baseHash linkHash tag sources]  (debug linkingEntryType) (debug baseHash) (debug linkHash) (debug tag) (debug sources) true)`)
		So(err, ShouldBeNil)
		ShouldLog(&h.config.Loggers.App, `rating
fakeBasehash
fakeLinkHash
some tag value
["fakeSrcHashvalue"]
`, func() {
			err = v.ValidateLink("rating", "fakeBasehash", "fakeLinkHash", "some tag value", []string{"fakeSrcHashvalue"})
			So(err, ShouldBeNil)
		})
	})
}

func TestZygoExposeCall(t *testing.T) {
	d, _, h := prepareTestChain("test")
	defer cleanupTestDir(d)

	zome, _ := h.GetZome("zySampleZome")
	v, err := h.makeNucleus(zome)
	if err != nil {
		panic(err)
	}
	z := v.(*ZygoNucleus)

	Convey("should allow calling exposed STRING based functions", t, func() {
		cater, _ := h.GetFunctionDef(zome, "testStrFn1")
		result, err := z.Call(cater, "fish \"zippy\"")
		So(err, ShouldBeNil)
		So(result.(string), ShouldEqual, "result: fish \"zippy\"")

		adder, _ := h.GetFunctionDef(zome, "testStrFn2")
		result, err = z.Call(adder, "10")
		So(err, ShouldBeNil)
		So(result.(string), ShouldEqual, "12")
	})
	Convey("should allow calling exposed JSON based functions", t, func() {
		times2, _ := h.GetFunctionDef(zome, "testJsonFn1")
		result, err := z.Call(times2, `{"input": 2}`)
		So(err, ShouldBeNil)
		So(string(result.([]byte)), ShouldEqual, `{"Atype":"hash", "input":2, "output":4, "zKeyOrder":["input", "output"]}`)
	})
	Convey("should allow a function declared with JSON parameter to be called with no parameter", t, func() {
		emptyParametersJson, _ := h.GetFunctionDef(zome, "testJsonFn2")
		result, err := z.Call(emptyParametersJson, "")
		So(err, ShouldBeNil)
		So(string(result.([]byte)), ShouldEqual, "[{\"Atype\":\"hash\", \"a\":\"b\", \"zKeyOrder\":[\"a\"]}]")
	})
}

func TestZygoDHT(t *testing.T) {
	d, _, h := prepareTestChain("test")
	defer cleanupTestDir(d)

	hash, _ := NewHash("QmY8Mzg9F69e5P9AoQPYat6x5HEhc1TVGs11tmfNSzkqh2")
	Convey("get should return hash not found if it doesn't exist", t, func() {
		v, err := NewZygoNucleus(h, fmt.Sprintf(`(get "%s")`, hash.String()))
		So(err, ShouldBeNil)
		z := v.(*ZygoNucleus)
		r, err := z.lastResult.(*zygo.SexpHash).HashGet(z.env, z.env.MakeSymbol("error"))
		So(err, ShouldBeNil)
		So(r.(*zygo.SexpStr).S, ShouldEqual, "hash not found")
	})
	// add an entry onto the chain
	hash, err := h.Commit("evenNumbers", "2")
	if err != nil {
		panic(err)
	}
	if err := h.dht.simHandleChangeReqs(); err != nil {
		panic(err)
	}

	Convey("get should return entry", t, func() {
		v, err := NewZygoNucleus(h, fmt.Sprintf(`(get "%s")`, hash.String()))
		So(err, ShouldBeNil)
		z := v.(*ZygoNucleus)
		r, err := z.lastResult.(*zygo.SexpHash).HashGet(z.env, z.env.MakeSymbol("result"))
		So(err, ShouldBeNil)
		So(r.(*zygo.SexpStr).S, ShouldEqual, `"2"`)
	})

	profileHash, err := h.Commit("profile", `{"firstName":"Zippy","lastName":"Pinhead"}`)
	if err != nil {
		panic(err)
	}

	_, err = h.Commit("rating", fmt.Sprintf(`{"Links":[{"Base":"%s","Link":"%s","Tag":"4stars"}]}`, hash.String(), profileHash.String()))
	if err != nil {
		panic(err)
	}

	if err := h.dht.simHandleChangeReqs(); err != nil {
		panic(err)
	}

	Convey("getlink function should return the Links", t, func() {
		v, err := NewZygoNucleus(h, fmt.Sprintf(`(getlink "%s" "4stars")`, hash.String()))
		So(err, ShouldBeNil)
		z := v.(*ZygoNucleus)
		sh := z.lastResult.(*zygo.SexpHash)

		r, err := sh.HashGet(z.env, z.env.MakeSymbol("result"))
		So(err, ShouldBeNil)
		So(r.(*zygo.SexpStr).S, ShouldEqual, `[{"H":"QmYeinX5vhuA91D3v24YbgyLofw9QAxY6PoATrBHnRwbtt","E":""}]`)
	})
	Convey("getlink function with load option should return the Links and entries", t, func() {
		v, err := NewZygoNucleus(h, fmt.Sprintf(`(getlink "%s" "4stars" (hash Load:true))`, hash.String()))
		So(err, ShouldBeNil)
		z := v.(*ZygoNucleus)
		sh := z.lastResult.(*zygo.SexpHash)

		r, err := sh.HashGet(z.env, z.env.MakeSymbol("result"))
		So(err, ShouldBeNil)
		So(r.(*zygo.SexpStr).S, ShouldEqual, `[{"H":"QmYeinX5vhuA91D3v24YbgyLofw9QAxY6PoATrBHnRwbtt","E":"{\"firstName\":\"Zippy\",\"lastName\":\"Pinhead\"}"}]`)
	})
}
