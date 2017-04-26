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
	Entries     map[string]EntryDef
	NucleusType string
	Functions   map[string]FunctionDef

	// cached stuff, not in dna
	code        string
	Entries     map[string]EntryDef
	NucleusType string
	Functions   map[string]FunctionDef

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

	// cache for code and nucleus
	code string
	nuc  *ZygoNucleus
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

func (holo *Holochain) registerNucleus(name string, zome *Zome) error {
	nuc, err := holo.NewNucleus(name, zome)
	if err == nil {
		holo.nuclei[name] = nuc
	}
	return err
}

func (holo *Holochain) registerEntryDef(zome *Zome, edef *EntryDef, ename string) (err error) {
	_, found := holo.zomeDefs[ename]
	if found {
		err = errors.New("Redefinition of entry type " + ename)
	} else {
		holo.zomeDefs[ename] = zome
	}
	return
}

func (h *Holochain) PrepareZomes(zomes map[string]Zome) (err error) {
	for _, z := range zomes {
		zpath := h.ZomePath(&z)
		Debugf("PZ path: %v\nCode:%v\n", h.ZomePath(&z), z.Code)
		if !fileExists(zpath + "/" + z.Code) {
			fmt.Printf("%v", z)
			err = errors.New("DNA specified code file missing: " + z.Code)
			return
		}
		Debugf("entries[%v] %v\n", z.Name, z.Entries)
		for name, e := range z.Entries {
			sc := e.Schema
			Debugf("Zome Entry %v, %v\n", name, sc)
			if sc != "" {
				if !fileExists(zpath + "/" + sc) {
					err = errors.New("DNA specified schema file missing: " + sc)
					return
				}
				if strings.HasSuffix(sc, ".json") {
					if err = e.BuildJSONSchemaValidator(zpath); err != nil {
						return
					}
					z.Entries[name] = e
					err = h.registerEntryDef(&z, &e, name)
					if err != nil {
						return
					}
				} else {
					Debugf("Suffix[%v]? %v\n", name, zpath+"/"+sc)
				}
			} else {
				Debugf("schema[%v]?\n", name)
			}
		}
	}

	h.dht = NewDHT(h)

	return
}

// Add GoNucleus (to be subclassed to *Nucleus builting types) use zygo for now
func (holo *Holochain) NewNucleus(zname string, zome *Zome) (nuc *Nucleus, err error) {
	// create new nucleus
	//check to see if we have a cached version of the code, otherwise read from disk
	if zome.code == "" {
		zpath := holo.ZomePath(zome)
		var code []byte

		code, err = readFile(zpath, zome.Code)
		if err != nil {
			return
		}
		zome.code = string(code)
	}

	nucObj, err := CreateNucleus(holo, zome.NucleusType, zome.code)
	if err == nil {
		nuc = nucObj.(*ZygoNucleus)
		err = nuc.ChainGenesis()
		if err != nil {
			err = fmt.Errorf("In '%s' zome: %s", zname, err.Error())
		}
	}
	return
}

func (holo *Holochain) GetNucleus(zname string) (nuc Nucleus, err error) {
	Debugf("GetNuc %v\n", zname)
	n, found := holo.nuclei[zname]
	if found {
		nuc = n
	} else {
		err = errors.New("No Nucleus " + zname)
	}
	return
}

func (zome *Zome) GetEntryDef(entryType string) (edef *EntryDef, err error) {
	edefObj, found := zome.Entries[entryType]
	if found {
		edef = &edefObj
	} else {
		err = errors.New("no definition for entry type: " + entryType)
	}
	return
}

func (zome *Zome) GenZomeDNA(holo *Holochain) (err error) {
	var bytes []byte
	zpath := holo.ZomePath(zome)
	Debugf("GenZomeDNA path %v\nZ:%v\nCode:%v\n", zpath, zome, zome.Code)
	bytes, err = readFile(zpath, zome.Code)
	if err != nil {
		return
	}
	err = zome.CodeHash.Sum(holo, bytes)
	Debugf("Hash: %v\n", zome.CodeHash.String())
	if err != nil {
		return
	}
	for name, entry := range zome.Entries {
		schema := entry.Schema
		Debugf("p schema %v %v\n", name, schema)
		if schema != "" {
			bytes, err = readFile(zpath, schema)
			if err != nil {
				return
			}
			err = entry.SchemaHash.Sum(holo, bytes)
			Debugf("Hashed %v, %v\n", name, string(bytes))
			if err != nil {
				return
			}
			Debugf("Store zome:%v\n%v\n", name, entry)
			zome.Entries[name] = entry
		}
	}
	Debugf("entries %v\n", zome.Entries)
	return
}
