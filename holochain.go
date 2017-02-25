// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//---------------------------------------------------------------------------------------

// Holochains are a distributed data store: DHT tightly bound to signed hash chains
// for provenance and data integrity.
package holochain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/boltdb/bolt"
	"github.com/google/uuid"
	"io"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

const Version string = "0.0.1"

// Unique user identifier in context of this holochain
type Agent string

// Holochain DNA settings, represents the instance
type Holochain struct {
	Version   int
	Id        uuid.UUID
	Name      string
	GroupInfo map[string]string
	HashType  string
	BasedOn   Hash // holochain hash for base schemas and code
	Schemas []Schema
	//---- private values not serialized; initialized on Load
	path    string
	agent   Agent
	privKey *ecdsa.PrivateKey
	store   Persister
}

// Holds content for a holochain
type Entry interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
	Content() interface{}
}

// GobEntry is a structure for implementing Gob encoding of Entry content
type GobEntry struct {
	C interface{}
}

// JSONEntry is a structure for implementing JSON encoding of Entry content
type JSONEntry struct {
	C interface{}
}

// ECDSA signature of an Entry
type Signature struct {
	R *big.Int
	S *big.Int
}

// Holochain entry header
type Header struct {
	Type        string
	Time        time.Time
	HeaderLink  Hash // link to previous header
	EntryLink   Hash // link to entry
	TypeLink    Hash // link to header of previous header of this type
	MySignature Signature
	Meta        interface{}
}

//	var service *holo.Service  a type in service.go
//	holo.Register()            This needs to be called first to initialize
//
// After initialization, you load services
//			service, err = holo.LoadService(root)
//  get chain from the service by name (uses Load, returns the service or error)
//	chain, err = service.IsConfigured(name)
//   IsConfigures does additional initialization and loads schemas, so names are iffy
//   Load does just the basics to load the service.
//						h, err := service.Load(name)
//  returns a name to *Holochain map of all configured chains.
//						chains, _ := service.ConfiguredChains()

// Register function that must be called once at startup by any client app
func Register() {
	gob.Register(Header{})
	gob.Register(KeyEntry{})
	RegisterBuiltinNucleii()
	RegisterBuiltinPersisters()
}

// NewHolochain creates a new holochain structure with a randomly generated ID and default values
func NewHolochain(agent Agent, private_key *ecdsa.PrivateKey, path string, defs ...Schema) Holochain {
	u, err := uuid.NewUUID()
	if err != nil {
		panic(err)
	}
	hol_chain := Holochain{
		Id:        u,
		HashType:  "SHA256",
		Schemas: defs,
		agent:     agent,
		privKey:   private_key,
		path:      path,
	}

	return hol_chain
}

// getMetaHash gets a value from the store that's a hash
func (hol_chain *Holochain) getMetaHash(key string) (hash Hash, err error) {
	hash_gotten, err := hol_chain.store.GetMeta(key)
	if err != nil {
		return
	}
	copy(hash[:], hash_gotten)
	if hash_gotten == nil {
		err = mkErr("Meta key '" + key + "' uninitialized")
	}
	return
}

// ID, Top and TopType are hash keys in the chain
// fetch them by fetching a hash from the id, top, or top_<typename> key

// ID returns a holochain ID hash or err if not yet defined
func (hol_chain *Holochain) ID() (id Hash, err error) {
	id, err = hol_chain.getMetaHash(IDMetaKey)
	return
}

// Top returns a hash of top header or err if not yet defined
func (hol_chain *Holochain) Top() (top Hash, err error) {
	top, err = hol_chain.getMetaHash(TopMetaKey)
	return
}

// Top returns a hash of top header of the given type or err if not yet defined
func (hol_chain *Holochain) TopType(type string) (top Hash, err error) {
	top, err = hol_chain.getMetaHash(TopMetaKey + "_" + type)
	return
}

// AddEntry adds an entry and it's header to the chain and returns the header and it's hash
// Return the header and its hash (or error)
func (prev_chain *Holochain) AddEntry(when time.Time, type string, new_entry Entry)
     (hdr_hash Hash, return_pointer *Header, err error) {
	var header Header
	header.Type = type
	header.Time = when

	prev_hdr_hash, err := prev_chain.Top()
	if err == nil {
		header.HeaderLink = prev_hdr_hash // header backlink 
	}
	prev_hdr_hash, err = prev_chain.TopType(type)
	if err == nil {
		header.TypeLink = prev_hdr_hash   //k
	}

	serialized, err := new_entry.Marshal()
	if err != nil {
		return
	}
	header.EntryLink = Hash(sha256.Sum256(serialized))

	r, s, err := ecdsa.Sign(rand.Reader, prev_chain.privKey, header.EntryLink[:])
	if err != nil {
		return
	}
	header.MySignature = Signature{R: r, S: s}

	gobbed_header, err := GobEncode(&header)
	if err != nil {
		return
	}
	hdr_hash = Hash(sha256.Sum256(gobbed_header))
	prev_chain.store.(*BoltPersister).DB().Update(func(bolt_trans *bolt.Tx) error {
		bkt := bolt_trans.Bucket([]byte(EntryBucket))

    // Bolt has a key : value storage model :: Put these pairs to bkt
    // EL(entry hash?) : entry_data (serialized as a gob(zygo) or json(JSON)
    // hdr_hash : gobbed_header
    // "top" (TopMetaKey) : hdr_hash
    // "type_<type>" : hdr_hash
		err = bkt.Put([]byte("top_"+type), hdr_hash)
		err = bkt.Put(header.EntryLink[:], serialized)
		if err != nil {
			return err
		}

		bkt = bolt_trans.Bucket([]byte(HeaderBucket))
		hdr_hash := hdr_hash[:] // ? copy to itself
		err = bkt.Put(hdr_hash, gobbed_header)
		if err != nil {
			return err
		}

		// don't use PutMeta because this has to be in the transaction
		bkt = bolt_trans.Bucket([]byte(MetaBucket))
		err = bkt.Put([]byte(TopMetaKey), hdr_hash)
		if err != nil {
			return err
		}
		err = bkt.Put([]byte("top_"+type), hdr_hash)
		if err != nil {
			return err
		}

		return nil
	})

	return_pointer = &header
	return
}

// get low level access to entries/headers (only works inside a bolt transaction)
func get(hb *bolt.Bucket, eb *bolt.Bucket, key []byte, getEntry bool) (header Header, entry interface{}, err error) {
	v := hb.Get(key)

	err = GobDecode(v, &header)
	if err != nil {
		return
	}
	if getEntry {
		v = eb.Get(header.EntryLink[:])
		var g GobEntry
		err = g.Unmarshal(v)
		if err != nil {
			return
		}
		entry = g.C
	}
	return
}

func (cur_chain *Holochain) Walk(fn func(hdr_hash *Hash, h *Header, entry interface{}) error, entriesToo bool) (err error) {
	var nullHash Hash
	var nullHashBytes = nullHash[:]
	err = cur_chain.store.(*BoltPersister).DB().View(func(bolt_trans *bolt.Tx) error {
		hb := bolt_trans.Bucket([]byte(HeaderBucket))
		eb := bolt_trans.Bucket([]byte(EntryBucket))
		mb := bolt_trans.Bucket([]byte(MetaBucket))
		meta_data := mb.Get([]byte(TopMetaKey))

		var hdr_hashH Hash
		var header Header
		var visited = make(map[string]bool)
		for err == nil && !bytes.Equal(nullHashBytes, key) {
			copy(hdr_hashH[:], hdr_hash)
			// build up map of all visited headers to prevent loops
      hdr_hash_string = string(hdr_hash)
			_, present := visited[hdr_hash_string]
			if present {
				err = errors.New("loop detected in walk")
			} else {
				visited[hdr_hash_string] = true
				var e interface{}
				header, e, err = get(hb, eb, key, entriesToo)
				if err == nil {
					err = fn(&keyH, &header, e)
					key = header.HeaderLink[:]
				}
			}
		}
		if err != nil {
			return err
		}
		// if the last item doesn't gets us to bottom, i.e. the header who's entry link is
		// the same as ID key then, the chain is invalid...
		if !bytes.Equal(header.EntryLink[:], mb.Get([]byte(IDMetaKey))) {
			return errors.New("chain didn't end at DNA!")
		}
		return err
	})
	return
}

// Validate scans back through a chain to the beginning confirming that the last header points to DNA
// This is actually kind of bogus on your own chain, because theoretically you put it there!  But
// if the holochain file was copied from somewhere you can consider this a self-check
func (cur_chain *Holochain) Validate(entriesToo bool) (valid bool, err error) {

	err = cur_chain.Walk(func(store_key *Hash, header *Header, entry interface{}) (err error) {
		// confirm the correctness of the header hash
		gobbed, err := GobEncod(&header)
		if err != nil {
			return err
		}
    // not sure why we have to calculate (string? or what type?)
    // then copy to a Hash, then compare using a string match, is that just the
    // easiest way to express the byte by byte equals?
		gobbed_sha256 := sha256.Sum256(gobbed)
		var gobbed_hash Hash
		copy(gobbed_hash[:], sha256[:])
    // Does the calculated key match what we looked up?
		if gobbed_hash.String() != (*store_key).String() {
			return errors.New("header hash doesn't match")
		}

		// TODO check entry hashes too if entriesToo set
		if entriesToo {

		}
		return nil
	}, entriesToo)
	if err == nil {
		valid = true
	}
	return
}

// Test validates test data against the current validation rules.
// This function is useful only in the context of developing a holochain and will return
// an error if the chain has already been started (i.e. has genesis entries)
func (hol_chain *Holochain) Test() (err error) {
	_, err = hol_chain.ID()
	if err == nil {
		err = errors.New("chain already started")
		return
	}
	chain_path := hol_chain.path + "/test"
	files, err := ioutil.ReadDir(chain_path)
	if err != err {
		return
	}

	if len(files) == 0 {
		return errors.New("no test data found in: " + hol_chain.path + "/test")
	}

	// setup the genesis entries
	hol_chain.GenChain()

	// and make sure the store gets reset to null after the test runs
	defer func() {
		err = hol_chain.store.Remove()
		if err != nil {
			panic(err)
		}
		err = hol_chain.store.Init()
		if err != nil {
			panic(err)
		}
	}()

	// load up the entries into hashes
	re := regexp.MustCompile(`([0-9])+_(.*)\.(.*)`)
	var entryTypes = make(map[int]string)
	var entryValues = make(map[int]string)
	for _, file := range files {
		if file.Mode().IsRegular() {
			match := re.FindStringSubmatch(file.Name())
			if len(match) > 0 {
				var i int
				i, err = strconv.Atoi(match[1])
				if err != nil {
					return
				}
				entryTypes[i] = match[2]
				var v []byte
				v, err = readFile(chain_path, match[0])
				if err != nil {
					return
				}
				entryValues[i] = string(v)
			}
		}
	}

	var keys []int
	for key := range entryValues {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	for key := range keys {
		idx := keys[key]
		e := GobEntry{C: entryValues[idx]}
		var header *Header
		_, header, err = hol_chain.AddEntry(time.Now(), entryTypes[idx], &e)
		if err != nil {
			return
		}
		//TODO: really we should be running hol_chain.Validate to test headers and genesis too
		err = hol_chain.ValidateEntry(header, e.C)
		if err != nil {
			return
		}
	}
	return
}
