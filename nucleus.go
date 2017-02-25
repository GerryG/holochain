// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------
// Nucleus provides an interface for an execution environment interface for chains and
// their entries and factory code for creating nucleii instances
//
// Schema definitions configure the different possible nucleus implementations,
// so that is here too.

package holochain

import (
	"errors"
	"fmt"
	"strings"
)

type NucleusFactory func(code string) (Nucleus, error)

type InterfaceSchemaType int

// Holds a schema entry definition
type Schema struct {
	Name       string
	Schema     string // file name of schema or schema directive (path in service dir)
	Code       string // file name of DNA code (path in service dir)
	SchemaHash Hash  // hash of the schema file contents, or just the name if SDefng
	CodeHash   Hash  // hash of the file contents
}

// There needs to be a nucleus for each self-defining schema
SelfDescribingSchemas := map[string]bool{
	"JSON":         true,
	ZygoSchemaType: true,
}

func SelfDescribingSchema(schema string) bool {
	return SelfDescribingSchemas[schema]
}

// GetSchema returns an Schema of the given name from the holochain
func (holchain *Holochain) GetSchema(name string) (schema_def *Schema, err error) {
	for _, sch_def := range cur_chain.Schemas {
		if sch_def.Name == name {
			schema_def = &sch_def
			break
		}
	}
	if schema_def == nil {
		err = errors.New("no definition for type: " + name)
	}
	return
}

// ValidateEntry passes an entry data to the chain's validation routine
// If the entry is valid err will be nil, otherwise it will contain some information
// about why the validation failed (or, possibly, some other system error)
func (hol_chain *Holochain) ValidateEntry(type string, entry interface{}) (err error) {

	if entry == nil {
		return errors.New("nil entry invalid")
	}
	nucleus, err := hol_chain.MakeNucleus(type)
	if err != nil {
		return
	}
	err = nucleus.ValidateEntry(entry)
	return
}

func (hol_chain *Holochain) MakeNucleus(type string) (nucleus Nucleus, err error) {
	schema, err := hol_chain.GetSchema(type)
	if err != nil {
		return
	}
	var code []byte
	code, err = readFile(hol_chain.path, schema.Code)
	if err != nil {
		return
	}

	// which nucleus to use is inferred from the schema type
	nucleus, err = CreateNucleus(schema.Schema, string(code))

	return
}

const (
	STRING InterfaceSchemaType = iota
	JSON
)

type Interface struct {
	Name   string
	Schema InterfaceSchemaType
}

type Nucleus interface {
	Name() string
	ValidateEntry(entry interface{}) error
	expose(iface Interface) error
	Interfaces() (i []Interface)
	Call(iface string, params interface{}) (interface{}, error)
}

var nucleusFactories = make(map[string]NucleusFactory)

// RegisterNucleus sets up a Nucleus to be used by the CreateNucleus function
func RegisterNucleus(name string, factory NucleusFactory) {
	if factory == nil {
		panic("Nucleus factory %s does not exist." + name)
	}
	_, registered := nucleusFactories[name]
	if registered {
		panic("Nucleus factory %s already registered. " + name)
	}
	nucleusFactories[name] = factory
}

// RegisterBuiltinNucleii adds the built in nucleus types to the factory hash
func RegisterBuiltinNucleii() {
	RegisterNucleus(ZygoSchemaType, NewZygoNucleus)
}

// CreateNucleus returns a new Nucleus of the given type
func CreateNucleus(schema string, code string) (Nucleus, error) {

	factory, ok := nucleusFactories[schema]
	if !ok {
		// Factory has not been registered.
		// Make a list of all available datastore factories for logging.
		available := make([]string, 0)
		for k, _ := range nucleusFactories {
			available = append(available, k)
		}
		return nil, errors.New(fmt.Sprintf("Invalid nucleus name. Must be one of: %s", strings.Join(available, ", ")))
	}

	return factory(code)
}
