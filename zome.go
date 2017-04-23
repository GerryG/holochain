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
