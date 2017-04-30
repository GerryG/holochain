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

	// cache for code and nucleus
	//code string       // Instantiate
	//nuc  *ZygoNucleus // We may not need this now
}

// EntryDef struct holds an entry definition
type EntryDef struct {
	Name       string
	DataFormat string
	Schema     string // file name of schema or language schema directive
	SchemaHash Hash
	Sharing    string
	validator  SchemaValidator
	zm         *Zome
}

// FunctionDef holds the name and calling type of an DNA exposed function
type FunctionDef struct {
	Name        string
	CallingType string

	fn          *GoFunc
	Entries     map[string]EntryDef
	NucleusType string
	Functions   map[string]FunctionDef

	// cache for code and nucleus
	code string
	nuc  *Nucleus
}

// ZomePath returns the path to the zome dna data
// @todo sanitize the name value
func (h *Holochain) ZomePath(z *Zome) string {
	return h.DNAPath() + "/" + z.Name
}

func (holo *Holochain) registerNucleus(zome *Zome) error {
	nuc, err := holo.NewNucleus(zome)
	if err == nil {
		if holo.nuclei == nil {
			holo.nuclei = map[string]*Nucleus{}
		}
		if _, found := holo.nuclei[zome.Name]; found {
			return errors.New("Redefinition of zome " + zome.Name)
		}
		holo.nuclei[zome.Name] = nuc
	}
	return err
}

// makeNucleus creates a Nucleus object based on the zome type
func (h *Holochain) makeNucleus(t string) (nuc Nucleus, z *Zome, err error) {
	z, err = h.getZome(t)
	if err != nil {
		return
	}
	nucObj, err := h.NewNucleus(z)
	if err == nil {
		nuc = *nucObj
	} else {
		return
	}
	return
}

// getZome returns a zome structure given its name
func (h *Holochain) getZome(zName string) (z *Zome, err error) {
	for _, zome := range h.Zomes {
		if zome.Name == zName {
			z = &zome
			break
		}
	}
	if z == nil {
		err = errors.New("unknown zome: " + zName)
		return
	}
	return
}

// GetFunctionDef returns the exposed function spec for the given zome and function
func (h *Holochain) GetFunctionDef(zome *Zome, fnName string) (fnDef string, err error) {
	for _, fnDef := range zome.Functions {
		if fnDef.Name == fnName {
			return fnDef, nil
		}
	}
	err = errors.New("Function not found " + fnName + " in zome " + zome.Name)
	return
}

func (holo *Holochain) registerEntryDef(zome *Zome, edef *EntryDef) (err error) {
	ename := edef.Name
	Debugf("reg ED %v %v, %v", zome.Name, edef, ename)
	if holo.zomeDefs == nil {
		holo.zomeDefs = map[string]*Zome{}
	}
	_, found := holo.zomeDefs[ename]
	if found {
		err = errors.New("Redefinition of entry type " + ename + " zome " + zome.Name)
	} else {
		holo.zomeDefs[ename] = zome
		Debugf("New zome def %v, %v", ename, holo.zomeDefs[ename])
	}
	return
}

func (h *Holochain) PrepareZomes(zomes []Zome) (err error) {
	for _, z := range zomes {
		zpath := h.ZomePath(&z)
		Debugf("PZ path: %v\nCode:%v", h.ZomePath(&z), z.Code)
		if !fileExists(zpath + "/" + z.Code) {
			fmt.Printf("%v", z)
			err = errors.New("DNA specified code file missing: " + z.Code)
			return
		}
		Debugf("entries[%v] %v", z.Name, z.Entries)
		for idx, e := range z.Entries {
			sc := e.Schema
			Debugf("Zome Entry %v, %v", idx, sc)
			if sc != "" {
				if !fileExists(zpath + "/" + sc) {
					err = errors.New("DNA specified schema file missing: " + sc)
					return
				}
				if strings.HasSuffix(sc, ".json") {
					if err = e.BuildJSONSchemaValidator(zpath); err != nil {
						return
					}
					z.Entries[idx] = e
					err = h.registerEntryDef(&z, &e)
					if err != nil {
						return
					}
				} else {
					Debugf("Suffix[%v]? %v", idx, zpath+"/"+sc)
				}
			} else {
				Debugf("schema[%v]?", idx)
			}
		}
	}

	h.dht = NewDHT(h)

	return
}

// Add GoNucleus (to be subclassed to *Nucleus builting types) use zygo for now
func (holo *Holochain) NewNucleus(zome *Zome) (nuc *Nucleus, err error) {
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
		err = nucObj.ChainGenesis()
		if err == nil {
			nuc = &nucObj
		} else {
			err = fmt.Errorf("In '%s' zome: %s", zname, err.Error())
		}
	}
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

func (holo *Holochain) getNucleus(zname string) (nuc Nucleus, err error) {
	Debugf("GetNuc %v", zname)
	nucObj, found := holo.nuclei[zname]
	if found {
		nuc = *nucObj
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
	Debugf("GenZomeDNA path %v\nZ:%v\nCode:%v", zpath, zome, zome.Code)
	bytes, err = readFile(zpath, zome.Code)
	if err != nil {
		return
	}
	err = zome.CodeHash.Sum(holo, bytes)
	Debugf("Hash: %v", zome.CodeHash.String())
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
			err = entry.SchemaHash.Sum(holo, bytes)
			Debugf("Hashed %v, %v", name, string(bytes))
			if err != nil {
				return
			}
			Debugf("Store zome:%v\n%v", name, entry)
			zome.Entries[name] = entry
		}
	}
	Debugf("entries %v", zome.Entries)
	return
}
