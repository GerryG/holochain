// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//---------------------------------------------------------------------------------------

// Data integrity engine for distributed applications -- a validating monotonic
// DHT "backed" by authoritative hashchains for data provenance.

package holochain

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	ic "github.com/libp2p/go-libp2p-crypto"
	peer "github.com/libp2p/go-libp2p-peer"
	protocol "github.com/libp2p/go-libp2p-protocol"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version is the numeric version number of the holochain library
const Version int = 7

// VersionStr is the textual version number of the holochain library
const VersionStr string = "7"
const HASH_SHA = "sha2-256"

// Config holds the non-DNA configuration for a holo-chain
// Move to dna or dht connected file?
type Config struct {
	Port            int
	PeerModeAuthor  bool
	PeerModeDHTNode bool
	BootstrapServer string
	Loggers         Loggers
}

// Holochain struct holds the full "DNA" of the holochain
type Holochain struct {
	Version          int
	ID               uuid.UUID
	Name             string
	Properties       map[string]string
	PropertiesSchema string
	HashType         string
	BasedOn          Hash // holochain hash for base schemas and code
	Zomes            []Zome
	RequiresVersion  int
	//---- private values not serialized; initialized on Load
	nuclei         map[string]*Nucleus     // Cache instantiated nuclei by zome name
	entryDefs      map[string]*EntryDef    // Cache of by entry type all zomes
	fnDefs         map[string]*FunctionDef // Cache of by function name, all zomes
	id             peer.ID                 // this is hash of the id, also used in the node
	dnaHash        Hash
	agentHash      Hash
	rootPath       string
	agent          Agent
	encodingFormat string
	HashSpec       HashType
	config         Config
	// Stores and Networks
	dht   *DHT
	node  *Node
	chain *Chain
}

// Initialize function that must be called once at startup by any client app
func Initialize(initProtocols func()) {
	initLoggers()

	gob.Register(Header{})
	gob.Register(AgentEntry{})
	gob.Register(Hash{})
	gob.Register(PutReq{})
	gob.Register(GetReq{})
	gob.Register(DelReq{})
	gob.Register(LinkReq{})
	gob.Register(LinkQuery{})
	gob.Register(GossipReq{})
	gob.Register(Gossip{})
	gob.Register(ValidateQuery{})
	gob.Register(ValidateResponse{})
	gob.Register(ValidateLinkResponse{})
	gob.Register(ValidateDelResponse{})
	gob.Register(Put{})
	gob.Register(EntryObj{})
	gob.Register(LinkQueryResp{})
	gob.Register(TaggedHash{})

	RegisterBultinNucleii()

	rand.Seed(time.Now().Unix()) // initialize global pseudo random generator

	if initProtocols != nil {
		initProtocols()
	} else {
		InitializeProtocols()
	}
}

func InitializeProtocols() {
	DHTProtocol = Protocol{protocol.ID("/hc-dht/0.0.0"), DHTReceiver}
	ValidateProtocol = Protocol{protocol.ID("/hc-validate/0.0.0"), ValidateReceiver}
	GossipProtocol = Protocol{protocol.ID("/hc-gossip/0.0.0"), GossipReceiver}
}

func findDNA(path string) (f string, err error) {
	p := path + "/" + DNAFileName

	Debugf("findDNA %v %v", path, p)
	matches, err := filepath.Glob(p + ".*")
	if err != nil {
		return
	}
	for _, fn := range matches {
		s := strings.Split(fn, ".")
		f = s[len(s)-1]
		if f == "json" || f == "yaml" || f == "toml" {
			break
		}
		f = ""
	}

	if f == "" {
		err = fmt.Errorf("No DNA file in %s/", path)
		return
	}
	return
}

// IsConfigured checks a directory for correctly set up holochain configuration files
func (s *Service) IsConfigured(name string) (f string, err error) {
	root := s.Path + "/" + name

	f, err = findDNA(root + "/" + ChainDNADir)
	if err != nil {
		return
	}
	//@todo check other things?

	return
}

// Load instantiates a Holochain instance from disk
func (s *Service) Load(name string) (h *Holochain, err error) {
	f, err := s.IsConfigured(name)
	if err != nil {
		return
	}
	h, err = s.load(name, f)
	return
}

func (h *Holochain) mkChainDirs() (err error) {
	//Debugf("Making chain dirs: %v, %v (%v)", h.DBPath(), h.DNAPath(), os.ModePerm)
	if err = os.MkdirAll(h.DBPath(), os.ModePerm); err != nil {
		return err
	}
	if err = os.MkdirAll(h.DNAPath(), os.ModePerm); err != nil {
		return
	}
	if err = os.MkdirAll(h.UIPath(), os.ModePerm); err != nil {
		return
	}
	return
}

// NewHolochain creates a new holochain structure with a randomly generated ID and default values
func NewHolochain(agent Agent, root string, format string) Holochain {
	Debugf("NewHolochain root %v, format %v", root, format)
	u, err := uuid.NewUUID()
	if err != nil {
		panic(err)
	}
	h := Holochain{
		Id:              u,
		HashType:        HASH_SHA,
		RequiresVersion: Version,
		agent:           agent,
		rootPath:        root,
		encodingFormat:  format,
	}

	// once the agent is set up we can calculate the id
	Debug("Gen keys")
	h.id, err = peer.IDFromPrivateKey(agent.PrivKey())
	if err != nil {
		panic(err)
	}

	h.prepareHashType()
	return h
}

// decodeDNA decodes a Holochain structure from an io.Reader
func decodeDNA(reader io.Reader, format string) (hP *Holochain, err error) {
	var h Holochain
	err = Decode(reader, format, &h)
	if err != nil {
		return
	}
	hP = &h
	hP.encodingFormat = format

	return
}

// load unmarshals a holochain structure for the named chain and format
func (s *Service) load(name string, format string) (hP *Holochain, err error) {

	root := s.Path + "/" + name
	Debugf("Service load: %v, %v p:%v", name, format, root)
	var f *os.File
	f, err = os.Open(root + "/" + ChainDNADir + "/" + DNAFileName + "." + format)
	if err != nil {
		return
	}
	defer f.Close()
	h, err := decodeDNA(f, format)
	if err != nil {
		return
	}
	h.encodingFormat = format
	h.rootPath = root

	// load the config
	f, err = os.Open(root + "/" + ConfigFileName + "." + format)
	if err != nil {
		return
	}
	defer f.Close()
	err = Decode(f, format, &h.config)
	if err != nil {
		return
	}
	if err = h.setupConfig(); err != nil {
		return
	}

	// try and get the agent from the holochain instance
	agent, err := LoadAgent(root)
	if err != nil {
		// get the default if not available
		agent, err = LoadAgent(filepath.Dir(root))
	}
	if err != nil {
		return
	}
	h.agent = agent

	// once the agent is set up we can calculate the id
	h.id, err = peer.IDFromPrivateKey(agent.PrivKey())
	if err != nil {
		return
	}

	if err = h.prepareHashType(); err != nil {
		return
	}

	err = h.NewChainFromFile()
	if err != nil {
		return
	}

	// if the chain has been started there should be a DNAHashFile which
	// we can load to check against the actual hash of the DNA entry
	var b []byte
	b, err = readFile(h.rootPath, DNAHashFileName)
	if err == nil {
		h.dnaHash, err = NewHash(string(b))
		if err != nil {
			return
		}
		// @TODO compare value from file to actual hash
	}

	if h.chain.Length() > 0 {
		h.agentHash = h.chain.Headers[1].EntryLink
	}
	if err = h.prepare(); err != nil {
		return
	}

	hP = h
	return
}

// Agent exposes the agent element
func (h *Holochain) Agent() Agent {
	return h.agent
}

// Prepare sets up a holochain to run by:
// validating the DNA, loading the schema validators, setting up a Network node and setting up the DHT
func (h *Holochain) prepare() (err error) {

	Debug("Prepare")
	if h.RequiresVersion > Version {
		err = fmt.Errorf("Chain requires Holochain version %d", h.RequiresVersion)
		return
	}

	if err = h.prepareHashType(); err != nil {
		return
	}
	err = h.PrepareZomes(h.Zomes)
	return
}

// Activate fires up the holochain node
func (h *Holochain) Activate() (err error) {
	listenaddr := fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", h.config.Port)
	h.node, err = NewNode(listenaddr, h.id, h.Agent().PrivKey())
	if err != nil {
		return
	}

	if h.config.PeerModeDHTNode {
		if err = h.dht.StartDHT(); err != nil {
			return
		}
		e := h.BSpost()
		if e != nil {
			h.dht.dlog.Logf("error in BSpost: %s", e.Error())
		}
		e = h.BSget()
		if e != nil {
			h.dht.dlog.Logf("error in BSget: %s", e.Error())
		}
	}
	if h.config.PeerModeAuthor {
		if err = h.node.startValidate(h); err != nil {
			return
		}
	}
	return
}

// UIPath returns a holochain UI path
func (h *Holochain) UIPath() string {
	return h.rootPath + "/" + ChainUIDir
}

// DBPath returns a holochain DB path
func (h *Holochain) DBPath() string {
	//Debugf("dbpath %v", h.rootPath+"/"+ChainDataDir)
	return h.rootPath + "/" + ChainDataDir
}

// DNAPath returns a holochain DNA path
func (h *Holochain) DNAPath() string {
	return h.rootPath + "/" + ChainDNADir
}

// TestPath returns the path to a holochain's test directory
func (h *Holochain) TestPath() string {
	return h.rootPath + "/" + ChainTestDir
}

// DNAHash returns the hash of the DNA entry which is also the holochain ID
func (h *Holochain) DNAHash() (id Hash) {
	return h.dnaHash.Clone()
}

// AgentHash returns the hash of the Agent entry
func (h *Holochain) AgentHash() (id Hash) {
	return h.agentHash.Clone()
}

// Top returns a hash of top header or err if not yet defined
func (h *Holochain) Top() (top Hash, err error) {
	tp := h.chain.Hashes[len(h.chain.Hashes)-1]
	top = tp.Clone()
	return
}

// Started returns true if the chain has been gened
func (h *Holochain) Started() bool {
	return h.DNAHash().String() != ""
}

// GenChain establishes a holochain instance by creating the initial genesis entries in the chain
// It assumes a properly set up .holochain sub-directory with a config file and
// keys for signing.  See GenDev()
func (h *Holochain) GenChain() (headerHash Hash, err error) {

	Debug("GenChain")
	if h.Started() {
		err = mkErr("chain already started")
		return
	}
	defer ErrorHandler(err, "")

	if err = h.prepare(); err != nil {
		return
	}

	var buf bytes.Buffer
	err = h.EncodeDNA(&buf)

	e := EntryObj{C: buf.Bytes()}

	var dnaHeader *Header
	_, dnaHeader, err = h.NewEntry(time.Now(), DNAEntryType, &e)
	if err != nil {
		return
	}

	h.dnaHash = dnaHeader.EntryLink.Clone()

	var k AgentEntry
	k.Name = h.agent.Name()
	k.KeyType = h.agent.KeyType()

	pk := h.agent.PrivKey().GetPublic()

	k.Key, err = ic.MarshalPublicKey(pk)
	if err != nil {
		return
	}

	e.C = k
	var agentHeader *Header
	headerHash, agentHeader, err = h.NewEntry(time.Now(), AgentEntryType, &e)
	if err != nil {
		return
	}

	h.agentHash = agentHeader.EntryLink

	if err = writeFile(h.rootPath, DNAHashFileName, []byte(h.dnaHash.String())); err != nil {
		return
	}

	//Debugf("GenChain %v, %v", agentHeader, headerHash)
	err = h.dht.SetupDHT()
	if err != nil {
		return
	}

	// run the init functions of each zome
	for _, zome := range h.Zomes {
		h.registerNucleus(&zome)
	}
	return
}

// Clone copies DNA files from a source directory
func (s *Service) Clone(clonedPath string, root string, new bool) (hP *Holochain, err error) {
	hP, err = gen(root, func(root string) (hP *Holochain, err error) {

		srcDNAPath := clonedPath + "/" + ChainDNADir
		format, err := findDNA(srcDNAPath)
		if err != nil {
			return
		}

		f, err := os.Open(srcDNAPath + "/" + DNAFileName + "." + format)
		if err != nil {
			return
		}
		defer f.Close()
		h, err := decodeDNA(f, format)
		if err != nil {
			return
		}
		h.rootPath = root

		agent, err := LoadAgent(filepath.Dir(root))
		if err != nil {
			return
		}
		h.agent = agent

		// once the agent is set up we can calculate the id
		h.id, err = peer.IDFromPrivateKey(agent.PrivKey())
		if err != nil {
			return
		}

		// make a config file
		if err = makeConfig(h, s); err != nil {
			return
		}

		if new {
			// generate a new UUID
			var u uuid.UUID
			u, err = uuid.NewUUID()
			if err != nil {
				return
			}
			h.ID = u

			// use the path as the name
			h.Name = filepath.Base(root)
		}

		// copy any UI files
		srcUiPath := clonedPath + "/" + ChainUIDir
		if dirExists(srcUiPath) {
			if err = CopyDir(srcUiPath, h.UIPath()); err != nil {
				return
			}
		}

		// copy any test files
		srcTestDir := clonedPath + "/" + ChainTestDir
		if dirExists(srcTestDir) {
			if err = CopyDir(srcTestDir, root+"/"+ChainTestDir); err != nil {
				return
			}
		}

		// create the DNA directory and copy
		if err := os.MkdirAll(h.DNAPath(), os.ModePerm); err != nil {
			return nil, err
		}

		propertiesSchema := srcDNAPath + "/properties_schema.json"
		if fileExists(propertiesSchema) {
			if err = CopyFile(propertiesSchema, h.DNAPath()+"/properties_schema.json"); err != nil {
				return
			}
		}

		for _, z := range h.Zomes {
			var bs []byte
			srczpath := srcDNAPath + "/" + z.Name
			bs, err = readFile(srczpath, z.Code)
			if err != nil {
				return
			}
			zpath := h.ZomePath(&z)
			if err = os.MkdirAll(zpath, os.ModePerm); err != nil {
				return nil, err
			}
			if err = writeFile(zpath, z.Code, bs); err != nil {
				return
			}
			for name, e := range z.Entries {
				sc := e.Schema
				if sc != "" {
					Debugf("An entry %v, %v, %v", name, e, sc)
					if err = CopyFile(srczpath+"/"+sc, zpath+"/"+sc); err != nil {
						Debugf("copy error %v", err)
						return
					}
				}
			}
		}

		hP = h
		return
	})
	return
}

func (h *Holochain) setupConfig() (err error) {
	if err = h.config.Loggers.App.New(nil); err != nil {
		return
	}
	if err = h.config.Loggers.DHT.New(nil); err != nil {
		return
	}
	if err = h.config.Loggers.Gossip.New(nil); err != nil {
		return
	}
	if err = h.config.Loggers.TestPassed.New(nil); err != nil {
		return
	}
	if err = h.config.Loggers.TestFailed.New(nil); err != nil {
		return
	}
	if err = h.config.Loggers.TestInfo.New(nil); err != nil {
		return
	}
	return
}

func makeConfig(h *Holochain, s *Service) (err error) {
	h.config = Config{
		Port:            DefaultPort,
		PeerModeDHTNode: s.Settings.DefaultPeerModeDHTNode,
		PeerModeAuthor:  s.Settings.DefaultPeerModeAuthor,
		BootstrapServer: s.Settings.DefaultBootstrapServer,
		Loggers:         NewAppLoggers(),
	}

	p := h.rootPath + "/" + ConfigFileName + "." + h.encodingFormat
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()

	if err = Encode(f, h.encodingFormat, &h.config); err != nil {
		return
	}
	if err = h.setupConfig(); err != nil {
		return
	}
	return
}

// gen calls a make function which should build the holochain structure and supporting files
// internal genesis creating method, "initialized" a chain so it can services.
func gen(root string, makeH func(root string) (hP *Holochain, err error)) (h *Holochain, err error) {
	if dirExists(root) {
		return nil, mkErr(root + " already exists")
	}
	if err := os.MkdirAll(root, os.ModePerm); err != nil {
		return nil, err
	}

	// cleanup the directory if we enounter an error while generating
	defer func() {
		if err != nil {
			os.RemoveAll(root)
		}
	}()

	h, err = makeH(root)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(h.DBPath(), os.ModePerm); err != nil {
		return nil, err
	}

	err = h.NewChainFromFile()
	if err != nil {
		return nil, err
	}

	err = h.SaveDNA(false)
	if err != nil {
		return nil, err
	}

	return
}

// EncodeDNA encodes a holochain's DNA to an io.Writer
func (h *Holochain) EncodeDNA(writer io.Writer) (err error) {
	return Encode(writer, h.encodingFormat, &h)
}

// SaveDNA writes the holochain DNA to a file
func (h *Holochain) SaveDNA(overwrite bool) (err error) {
	p := h.DNAPath() + "/" + DNAFileName + "." + h.encodingFormat
	if !overwrite && fileExists(p) {
		return mkErr(p + " already exists")
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	err = h.EncodeDNA(f)
	return
}

// GenDNAHashes generates hashes for all the definition files in the DNA.
// This function should only be called by developer tools at the end of the process
// of finalizing DNA development or versioning
func (h *Holochain) GenDNAHashes() (err error) {
	for _, zome := range h.Zomes {
		zome.GenZomeDNA(h)
	}
	err = h.SaveDNA(true)
	return
}

// NewEntry adds an entry and it's header to the chain and returns the header and it's hash
func (h *Holochain) NewEntry(now time.Time, entryType string, entry Entry) (hash Hash, header *Header, err error) {
	var l int
	//Debugf("NewEntry %v, %v", now, entryType)
	l, hash, header, err = h.PrepareHeader(now, entryType, entry, h.agent.PrivKey())
	if err == nil {
		err = h.addEntry(l, hash, header, entry)
	}

	if err == nil {
		var e interface{} = entry
		if entryType == DNAEntryType {
			e = "<DNA>"
		}
		Debugf("NewEntry of %s added as: %s (entry: %v)", entryType, header.EntryLink, e)
	} else {
		Debugf("NewEntry of %s failed with: %s (entry: %v)", entryType, err, entry)
	}

	return
}

// Walk takes the argument fn which must be WalkerFn
// Every WalkerFn is of the form:
// func(key *Hash, h *Header, entry interface{}) error
func (h *Holochain) Walk(fn WalkerFn, entriesToo bool) (err error) {
	err = h.chain.Walk(fn)
	return
}

// Validate scans back through a chain to the beginning confirming that the last header points to DNA
// This is actually kind of bogus on your own chain, because theoretically you put it there!  But
// if the holochain file was copied from somewhere you can consider this a self-check
func (h *Holochain) Validate(entriesToo bool) (valid bool, err error) {

	err = h.Walk(func(key *Hash, header *Header, entry Entry) (err error) {
		// confirm the correctness of the header hash

		var bH Hash
		bH, _, err = header.Sum(h)
		if err != nil {
			return
		}

		if !bH.Equal(key) {
			return errors.New("header hash doesn't match")
		}

		// @TODO check entry hashes Etoo if entriesToo set
		if entriesToo {

		}
		return nil
	}, entriesToo)
	if err == nil {
		valid = true
	}
	return
}

// GetEntryDef returns an EntryDef of the given name
// @TODO this makes the incorrect assumption that entry type strings are unique across zomes
func (holo *Holochain) GetEntryDef(entryType string) (edef *EntryDef, err error) {
	Debugf("GetEntryDef %v, $v")
	edef, found := holo.entryDefs[entryType]
	if !found {
		err = errors.New("Entry definition not found " + entryType)
	}
	return
}

func prepareSources(sources []peer.ID) (srcs []string) {
	srcs = make([]string, 0)
	for _, s := range sources {
		srcs = append(srcs, peer.IDB58Encode(s))
	}
	return
}

// validatePrepare does system level validation and structure creation before app validation
// It checks that entry is not nil, and that it conforms to the entry schema in the definition
// It returns the entry definition and a nucleus vm object on which to call the app validation
func (h *Holochain) validatePrepare(entryType string, entry Entry, sources []peer.ID) (def *EntryDef, srcs []string, nuc Nucleus, err error) {
	if entry == nil {
		err = errors.New("nil entry invalid")
		return
	}
	var z *Zome
	def, err = h.GetEntryDef(entryType)
	if err != nil {
		return
	}

	// see if there is a schema validator for the entry type and validate it if so
	if def.validator != nil {
		var input interface{}
		if def.DataFormat == DataFormatJSON {
			if err = json.Unmarshal([]byte(entry.Content().(string)), &input); err != nil {
				return
			}
		} else {
			input = entry
		}
		Debugf("Validating %v against schema", input)
		if err = def.validator.Validate(input); err != nil {
			return
		}
	} else if def.DataFormat == DataFormatLinks {
		// Perform base validation on links entries, i.e. that all items exist and are of the right types
		// so first unmarshall the json, and then check that the hashes are real.
		var l struct{ Links []map[string]string }
		err = json.Unmarshal([]byte(entry.Content().(string)), &l)
		if err != nil {
			err = fmt.Errorf("invalid links entry, invalid json: %v", err)
			return
		}
		if len(l.Links) == 0 {
			err = errors.New("invalid links entry: you must specify at least one link")
			return
		}
		for _, link := range l.Links {
			h, ok := link["Base"]
			if !ok {
				err = errors.New("invalid links entry: missing Base")
				return
			}
			if _, err = NewHash(h); err != nil {
				err = fmt.Errorf("invalid links entry: Base %v", err)
				return
			}
			h, ok = link["Link"]
			if !ok {
				err = errors.New("invalid links entry: missing Link")
				return
			}
			if _, err = NewHash(h); err != nil {
				err = fmt.Errorf("invalid links entry: Link %v", err)
				return
			}
			_, ok = link["Tag"]
			if !ok {
				err = errors.New("invalid links entry: missing Tag")
				return
			}
		}

	}
	srcs = prepareSources(sources)

	// then run the nucleus (ie. "app" specific) validation rules
	nucObj, err := h.NewNucleus(def.zm)
	if err == nil {
		nuc = *nucObj
	} else {
		return
	}

	return
}

// ValidateCommit passes entry data to the chain's commit validation routine
// If the entry is valid err will be nil, otherwise it will contain some information about why the validation failed (or, possibly, some other system error)
func (h *Holochain) ValidateCommit(entryType string, entry Entry, header *Header, sources []peer.ID) (def *EntryDef, err error) {
	def, srcs, n, err := h.validatePrepare(entryType, entry, sources)
	if err != nil {
		return
	}
	err = n.ValidateCommit(entryType, entry, header, srcs)
	if err != nil {
		Debugf("ValidateCommit err:%v", err)
	}
	return
}

// ValidatePut passes entry data to the chain's put validation routine
// If the entry is valid err will be nil, otherwise it will contain some information about why the validation failed (or, possibly, some other system error)
func (h *Holochain) ValidatePut(entryType string, entry Entry, header *Header, sources []peer.ID) (err error) {
	var def *EntryDef
	var srcs []string
	var n Nucleus
	def, srcs, n, err = h.validatePrepare(entryType, entry, sources)
	if err != nil {
		return
	}
	err = n.ValidatePut(entryType, entry, header, srcs)
	if err != nil {
		Debugf("ValidatePut err:%v", err)
	}
	return
}

// ValidateDel passes entry data to the chain's put validation routine
// If the entry is valid err will be nil, otherwise it will contain some information about why the validation failed (or, possibly, some other system error)
func (h *Holochain) ValidateDel(entryType string, hash string, sources []peer.ID) (err error) {
	def, err := h.GetEntryDef(entryType)
	if err != nil {
		return
	}

	// run the nucleus' validation rules
	var nuc Nucleus
	nucObj, err := h.NewNucleus(def.zm)
	if err == nil {
		nuc = *nucObj
	} else {
		return
	}
	srcs := prepareSources(sources)
	err = nuc.ValidateDel(entryType, hash, srcs)

	if err != nil {
		Debugf("ValidateDel err:%v", err)
	}
	return
}

// ValidateLink passes link data to the chain's link validation routine
// If the link is valid err will be nil, otherwise it will contain some information about why the validation failed (or, possibly, some other system error)
func (h *Holochain) ValidateLink(linkingEntryType string, base string, link string, tag string, sources []peer.ID) (err error) {

	var def *EntryDef
	def, err = h.GetEntryDef(linkingEntryType)
	if err != nil {
		return
	}

	// run the nucleus (ie. "app" specific) validation rules
	var nuc Nucleus
	nucObj, err := h.NewNucleus(def.zm)
	if err == nil {
		nuc = *nucObj
	} else {
		return
	}
	srcs := prepareSources(sources)
	err = nuc.ValidateLink(linkingEntryType, base, link, tag, srcs)
	if err != nil {
		Debugf("ValidateLink err:%v", err)
	}
	return
}

// Call executes an exposed function
func (h *Holochain) Call(zomeType string, function string, arg NuclearData) (result NuclearData, err error) {
	nuc, err := h.getNucleus(zomeType)
	if err != nil {
		return
	}
	Debugf("Call %v, %v", zomeType, function)
	if err != nil {
		//result, err = nuc.Call(function, arg)
		result, err = nuc.Call(function, arg)
	}
	return
}

// GetProperty returns the value of a DNA property
func (h *Holochain) GetProperty(prop string) (property string, err error) {
	if prop == ID_PROPERTY || prop == AGENT_ID_PROPERTY || prop == AGENT_NAME_PROPERTY {
		ChangeAppProperty.Log()
	} else {
		property = h.Properties[prop]
	}
	return
}

// Reset deletes all chain and dht data and resets data structures
func (h *Holochain) Reset() (err error) {

	h.dnaHash = Hash{}
	h.agentHash = Hash{}

	if h.chain.s != nil {
		h.chain.s.Close()
	}

	err = os.RemoveAll(h.DBPath())
	if err == nil {
		if err = os.MkdirAll(h.DBPath(), os.ModePerm); err != nil {
			return
		}
	}

	if err = os.MkdirAll(h.DBPath(), os.ModePerm); err != nil {
		return
	}
	h.chain, err = NewChainFromFile(h.hashSpec, h.DBPath()+"/"+StoreFileName)
	if err != nil {
		return
	}

	err = os.RemoveAll(h.rootPath + "/" + DNAHashFileName)
	// shouldn't we have this in a defer to catch any error here?
	if err != nil {
		panic(err)
	}
	if h.dht != nil {
		close(h.dht.puts)
	}
	h.dht = NewDHT(h)

	return
}

// DHT exposes the DHT structure
func (h *Holochain) DHT() *DHT {
	return h.dht
}

// Send builds a message and either delivers it locally or over the network via node.Send
func (h *Holochain) Send(proto Protocol, to peer.ID, t MsgType, body interface{}) (response interface{}, err error) {
	message := h.node.NewMessage(t, body)
	if err != nil {
		return
	}
	// if we are sending to ourselves we should bypass the network mechanics and call
	// the receiver directly
	if to == h.node.HashAddr {
		Debugf("Sending message local:%v", message)
		response, err = proto.Receiver(h, message)
	} else {
		Debugf("Sending message net:%v", message)
		var r Message
		r, err = h.node.Send(proto, to, message)
		Debugf("send result: %v error:%v", r, err)

		if err != nil {
			return
		}
		if r.Type == ERROR_RESPONSE {
			err = fmt.Errorf("response error: %v", r.Body)
		} else {
			response = r.Body
		}
	}
	return
}

// ---------------------------------------------------------------------------------
// ---- These functions implement the required functions called by specific nuclei implementations

// Get services nucleus get routines
func (h *Holochain) Get(hash string) (entry Entry, err error) {

	var key Hash
	key, err = NewHash(hash)
	if err != nil {
		return
	}
	response, err := h.dht.SendGet(key)
	if err != nil {
		return
	}
	switch t := response.(type) {
	case *EntryObj:
		entry = t
	default:
		err = fmt.Errorf("unexpected response type from SendGet: %T", t)
	}
	return
}

// Commit services nucleus commit routines
// it check validity and adds a new entry to the chain, and also does any special actions,
// like put or link if these are shared entries
func (h *Holochain) Commit(entryType, entry string) (entryHash Hash, err error) {
	e := EntryObj{C: entry}
	var l int
	var hash Hash
	var header *Header
	l, hash, header, err = h.PrepareHeader(time.Now(), entryType, &e, h.agent.PrivKey())
	if err != nil {
		return
	}
	var d *EntryDef
	d, err = h.ValidateCommit(entryType, &e, header, []peer.ID{h.id})
	if err != nil {
		return
	}

	err = h.addEntry(l, hash, header, &e)
	if err != nil {
		return
	}
	entryHash = header.EntryLink

	if d.DataFormat == DataFormatLinks {
		// if this is a Link entry we have to send the DHT Link message
		var le LinksEntry
		err = json.Unmarshal([]byte(entry), &le)
		if err != nil {
			return
		}

		bases := make(map[string]bool)
		for _, l := range le.Links {
			_, exists := bases[l.Base]
			if !exists {
				b, _ := NewHash(l.Base)
				h.dht.SendLink(LinkReq{Base: b, Links: entryHash})
				bases[l.Base] = true
			}
		}
	} else if d.Sharing == Public {
		// otherwise we check to see if it's a public entry and if so send the DHT put message
		err = h.dht.SendPut(entryHash)
	}
	return
}

// GetLink services nucleus getlink routines
func (h *Holochain) GetLink(basestr string, tag string, options GetLinkOptions) (response *LinkQueryResp, err error) {
	var base Hash
	base, err = NewHash(basestr)
	if err == nil {
		var r interface{}
		r, err = h.dht.SendGetLink(LinkQuery{Base: base, T: tag})
		if err == nil {
			switch t := r.(type) {
			case *LinkQueryResp:
				response = t
				if options.Load {
					for i, _ := range response.Links {
						entry, err := h.Get(response.Links[i].H)
						if err == nil {
							response.Links[i].E = entry.(*EntryObj).C.(string)
						}
					}
				}
			default:
				err = fmt.Errorf("unexpected response type from SendGetLink: %T", t)
			}
		}
	}
	return
}

// Del services nucleus del routines
func (h *Holochain) Del(hash string) (err error) {
	var key Hash
	key, err = NewHash(hash)
	if err != nil {
		return
	}
	err = h.dht.SendDel(key)
	if err != nil {
		return
	}

	return
}
