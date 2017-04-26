// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------

// implements chain entry structures and functions

package holochain

import (
	"encoding/binary"
	//"encoding/json"
	"github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsval"
	"github.com/lestrrat/go-jsval/builder"
	"io"
)

const (
	DNAEntryType   = "%dna"
	AgentEntryType = "%agent"
	KeyEntryType   = "%%key" // virtual entry type, not actually on the chain
)

const (
	DataFormatLinks   = "links"
	DataFormatJSON    = "json"
	DataFormatString  = "string"
	DataFormatRawJS   = "js"
	DataFormatRawZygo = "zygo"
)

const (
	Public  = "public"
	Partial = "partial"
)

// AgentEntry structure for building KeyEntryType entries
type AgentEntry struct {
	Name    AgentName
	KeyType KeytypeType
	Key     []byte // marshaled public key
}

// LinksEntry holds one or more links
type LinksEntry struct {
	Links []Link
}

// Link structure for holding meta tagging of linking entry
type Link struct {
	Base string // hash of entry (perhaps elsewhere) tow which we are attaching the link
	Link string // hash of entry being linked to
	Tag  string // tag
}

// Entry describes serialization and deserialziation of entry data
type Entry interface {
	Marshal() ([]byte, error)
	Unmarshal([]byte) error
	Content() interface{}
	Sum(*Holochain) (Hash, error)
}

type EntryObj struct {
	C interface{}
}

// SchemaValidator interface for schema validation
type SchemaValidator interface {
	Validate(interface{}) error
}

// MarshalEntry serializes an entry to a writer
func (holo *Holochain) MarshalEntry(writer io.Writer, e Entry) (err error) {
	var b []byte
	b, err = e.Marshal()
	l := uint64(len(b))
	err = binary.Write(writer, binary.LittleEndian, l)
	if err != nil {
		return
	}
	err = binary.Write(writer, binary.LittleEndian, b)
	return
}

// UnmarshalEntry unserializes an entry from a reader
func (holo *Holochain) UnmarshalEntry(reader io.Reader) (e Entry, err error) {
	var l uint64
	err = binary.Read(reader, binary.LittleEndian, &l)
	if err != nil {
		return
	}
	var b = make([]byte, l)
	err = binary.Read(reader, binary.LittleEndian, b)
	if err != nil {
		return
	}

	var g EntryObj
	err = g.Unmarshal(b)

	e = &g
	return
}

// implementation of Entry interface with gobs

func (e *EntryObj) Marshal() (b []byte, err error) {
	b, err = ByteEncoder(&e.C)
	return
}
func (e *EntryObj) Unmarshal(b []byte) (err error) {
	err = ByteDecoder(b, &e.C)
	return
}

func (e *EntryObj) Content() interface{} { return e.C }

func (e *EntryObj) Sum(holo *Holochain) (hash Hash, err error) {
	// encode the entry into bytes
	marshaled, err := e.Marshal()
	if err != nil {
		return
	}

	// calculate the entry's hash and store it in the header
	err = hash.Sum(holo, marshaled)
	if err != nil {
		return
	}

	return
}

type JSONSchemaValidator struct {
	v *jsval.JSVal
}

// implementation of SchemaValidator with JSONSchema

func (v *JSONSchemaValidator) Validate(entry interface{}) (err error) {
	err = v.v.Validate(entry)
	return
}

// BuildJSONSchemaValidator builds a validator in an EntryDef
func (d *EntryDef) BuildJSONSchemaValidator(path string) (err error) {
	Debug("BuildJSON val\n")
	var s *schema.Schema
	s, err = schema.ReadFile(path + "/" + d.Schema)
	if err != nil {
		return
	}

	b := builder.New()
	var v JSONSchemaValidator
	v.v, err = b.Build(s)
	if err == nil {
		v.v.SetName(d.Schema)
		d.validator = &v
	}
	return
}
