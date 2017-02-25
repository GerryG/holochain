// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------
// Persister implements a persistence engine interface for storing data
// additionally it implements a bolt use of that interface

package holochain

import (
	"errors"
	_ "errors"
	"fmt"
	"github.com/boltdb/bolt"
	"os"
	"strings"
	"time"
)

const (
	IDMetaKey  = "id"
	TopMetaKey = "top"

	MetaBucket   = "M"
	HeaderBucket = "H"
	EntryBucket  = "E"
)

const (
	BoltPersisterName = "bolt"
)

type Persister interface {
	Open() error
	Close()
	Init() error
	GetMeta(string) ([]byte, error)
	PutMeta(key string, value []byte) (err error)
	Get(hash Hash, getEntry bool) (header Header, entry interface{}, err error)
	Remove() error
	Name() string
}

type BoltPersister struct {
	path string
	db   *bolt.DB
}

// Name returns the data store name
func (persister *BoltPersister) Name() string {
	return BoltPersisterName
}

// Open opens the data store
func (persister *BoltPersister) Open() (err error) {
	persister.db, err = bolt.Open(persister.path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return
	}
	return
}

// Close closes the data store
func (persister *BoltPersister) Close() {
	persister.db.Close()
	persister.db = nil
}

// Init opens the store (if it isn't already open) and initializes buckets
func (persister *BoltPersister) Init() (err error) {
	if persister.db == nil {
		err = persister.Open()
	}
	if err != nil {
		return
	}

	defer func() {
		if err != nil {
			persister.db.Close()
			persister.db = nil
		}
	}()
	var initialized bool
	err = persister.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(MetaBucket))
		initialized = b != nil
		return nil
	})
	if err != nil {
		return
	}
	if !initialized {
		err = persister.db.Update(func(tx *bolt.Tx) (err error) {
			_, err = tx.CreateBucketIfNotExists([]byte(EntryBucket))
			if err != nil {
				return
			}
			_, err = tx.CreateBucketIfNotExists([]byte(HeaderBucket))
			if err != nil {
				return
			}
			_, err = tx.CreateBucketIfNotExists([]byte(MetaBucket))
			return
		})
	}

	return
}

// Get returns a header, and (optionally) it's entry if getEntry is true
func (persister *BoltPersister) Get(hash Hash, getEntry bool) (header Header, entry interface{}, err error) {
	err = persister.db.View(func(tx *bolt.Tx) error {
		hb := tx.Bucket([]byte(HeaderBucket))
		eb := tx.Bucket([]byte(EntryBucket))
		header, entry, err = get(hb, eb, hash[:], getEntry)
		return err
	})
	return
}

// GetMeta returns the Hash stored at key
func (persister *BoltPersister) GetMeta(key string) (hash []byte, err error) {
	err = persister.db.View(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte(MetaBucket))
		hash = bkt.Get([]byte(key))
		return nil
	})
	return
}

// PutMeta stores a Hash value at key
func (persister *BoltPersister) PutMeta(key string, value []byte) (err error) {
	err = persister.db.Update(func(tx *bolt.Tx) (err error) {
		bkt := tx.Bucket([]byte(MetaBucket))
		err = bkt.Put([]byte(key), value)
		return err
	})
	return
}

// Remove deletes all data in the datastore
func (persister *BoltPersister) Remove() (err error) {
	os.Remove(persister.path)
	persister.db = nil
	return nil
}

// NewBoltPersister returns a Bolt implementation of the Persister type
// always return no error because in this case any errors would happen at Init or Open time
func NewBoltPersister(path string) (pptr Persister, err error) {
	var persister BoltPersister
	persister.path = path
	pptr = &persister
	return
}

// DB returns the bolt db to give clients direct accesses to the bolt store
func (persister *BoltPersister) DB() *bolt.DB {
	return persister.db
}

type PersisterFactory func(config string) (Persister, error)

var persistorFactories = make(map[string]PersisterFactory)

// RegisterBuiltinPersisters adds the built in persister types to the factory hash
func RegisterBuiltinPersisters() {
	RegisterPersister(BoltPersisterName, NewBoltPersister)
}

// RegisterPersister sets up a Persister to be used by the CreatePersister function
func RegisterPersister(name string, factory PersisterFactory) {
	if factory == nil {
		panic("Datastore factory %s does not exist." + name)
	}
	_, registered := nucleusFactories[name]
	if registered {
		panic("Datastore factory %s already registered. " + name)
	}
	persistorFactories[name] = factory
}

// CreatePersister returns a new Persister of the given type
func CreatePersister(ptype string, config string) (Persister, error) {

	factory, ok := persistorFactories[ptype]
	if !ok {
		// Factory has not been registered.
		// Make a list of all available datastore factories for logging.
		available := make([]string, 0)
		for k, _ := range persistorFactories {
			available = append(available, k)
		}
		return nil, errors.New(fmt.Sprintf("Invalid persister name. Must be one of: %s", strings.Join(available, ", ")))
	}

	return factory(config)
}
