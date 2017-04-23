// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//---------------------------------------------------------------------------------------

// Data integrity engine for distributed applications -- a validating monotonic
//
// Zome struct encapsulates logically related code, from "chromosome"

package holochain

import (
	"errors"
	"fmt"
	"strings"
)

type Zome struct {
	Name        string
	Description string
	Code        string // file name of DNA code
	CodeHash    Hash
	Entries     []EntryDef
	NucleusType string
	Functions   []FunctionDef

	// cache for code
	code string
	nuc  *Nucleus
}

// EntryDef struct holds an entry definition
type EntryDef struct {
	Name       string
	DataFormat string
	Schema     string // file name of schema or language schema directive
	SchemaHash Hash
	Sharing    string
	validator  SchemaValidator
}

// FunctionDef holds the name and calling type of an DNA exposed function
type FunctionDef struct {
	Name        string
	CallingType string
}

// ZomePath returns the path to the zome dna data
// @todo sanitize the name value
func (h *Holochain) ZomePath(z *Zome) string {
	return h.DNAPath() + "/" + z.Name
}

func (h *Holochain) PrepareZomes(zomes []Zome) (err error) {
	for _, z := range zomes {
		zpath := h.ZomePath(&z)
		if !fileExists(zpath + "/" + z.Code) {
			fmt.Printf("%v", z)
			err = errors.New("DNA specified code file missing: " + z.Code)
			return
		}
		for i, e := range z.Entries {
			sc := e.Schema
			if sc != "" {
				if !fileExists(zpath + "/" + sc) {
					err = errors.New("DNA specified schema file missing: " + sc)
					return
				}
				if strings.HasSuffix(sc, ".json") {
					if err = e.BuildJSONSchemaValidator(zpath); err != nil {
						return
					}
					z.Entries[i] = e
				}
			}
		}
	}

	h.dht = NewDHT(h)

	return
}

func (zome *Zome) GetNucleus(chain *Holochain) (err error) {
	var n Nucleus
	n, err = chain.makeNucleus(zome)
	if err == nil {
		err = n.ChainGenesis()
		if err != nil {
			err = fmt.Errorf("In '%s' zome: %s", zome.Name, err.Error())
		}
	}
	return
}

func (zome *Zome) GenZomeDNA(chain *Holochain) (err error) {
	var bytes []byte
	zpath := chain.ZomePath(zome)
	bytes, err = readFile(zpath, zome.Code)
	if err != nil {
		return
	}
	err = zome.CodeHash.Sum(chain.hashSpec, bytes)
	if err != nil {
		return
	}
	for name, entry := range zome.Entries {
		schema := entry.Schema
		if schema != "" {
			bytes, err = readFile(zpath, schema)
			if err != nil {
				return
			}
			err = entry.SchemaHash.Sum(chain.hashSpec, bytes)
			if err != nil {
				return
			}
			zome.Entries[name] = entry
		}
	}
	return
}
