// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//---------------------------------------------------------------------------------------

// Data integrity engine for distributed applications -- a validating monotonic
// Samples and data used in generating a generic starter chain. See use of GenDev() in hc.

package holochain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// GenDev generates starter holochain DNA files from which to develop a chain
func (s *Service) GenDev(root string, format string) (hP *Holochain, err error) {
	hP, err = gen(root, func(root string) (hP *Holochain, err error) {
		agent, err := LoadAgent(filepath.Dir(root))
		Debugf("Gat agent[%v], %v (%v)\n", err, agent, root)
		if err != nil {
			return
		}

		h := NewHolochain(agent, root, format)
		h.Zomes = SampleZomes

		if err = h.mkChainDirs(); err != nil {
			return nil, err
		}

		// use the path as the name
		h.Name = filepath.Base(root)

		if err = makeConfig(&h, s); err != nil {
			return
		}

		if err = writeFile(h.DNAPath(), "properties_schema.json", []byte(PropertiesSchema)); err != nil {
			return
		}

		h.PropertiesSchema = "properties_schema.json"
		h.Properties = map[string]string{
			"description": "a bogus test holochain",
			"language":    "en"}

		for fileName, fileText := range SampleUI {
			if err = writeFile(h.UIPath(), fileName, []byte(fileText)); err != nil {
				return
			}
		}

		code := make(map[string]string)
		code["zySampleZome"] = `
(defn testStrFn1 [x] (concat "result: " x))
(defn testStrFn2 [x] (+ (atoi x) 2))
(defn testJsonFn1 [x] (begin (hset x output: (* (-> x input:) 2)) x))
(defn testJsonFn2 [x] (unjson (raw "[{\"a\":\"b\"}]"))) (defn getDNA [x] App_DNA_Hash)
(defn addEven [x] (commit "evenNumbers" x))
(defn addPrime [x] (commit "primes" x))
(defn validateCommit [entryType entry header sources]
  (validate entryType entry header sources))
(defn validatePut [entryType entry header sources]
  (validate entryType entry header sources))
(defn validateDel [entryType hash sources] true)
(defn validate [entryType entry header sources]
  (cond (== entryType "evenNumbers")  (cond (== (mod entry 2) 0) true false)
        (== entryType "primes")  (isprime (hget entry %prime))
        (== entryType "profile") true
        false)
)
(defn validateLink [linkEntryType baseHash linkHash tag sources] true)
(defn genesis [] true)
`
		code["jsSampleZome"] = `
function testStrFn1(x) {return "result: "+x};
function testStrFn2(x){ return parseInt(x)+2};
function testJsonFn1(x){ x.output = x.input*2; return x;};
function testJsonFn2(x){ return [{a:'b'}] };

function getProperty(x) {return property(x)};
function addOdd(x) {return commit("oddNumbers",x);}
function addProfile(x) {return commit("profile",x);}
function validatePut(entry_type,entry,header,sources) {
  return validate(entry_type,entry,header,sources);
}
function validateDel(entry_type,hash,sources) {
  return true;
}
function validateCommit(entry_type,entry,header,sources) {
  if (entry_type == "rating") {return true}
  return validate(entry_type,entry,header,sources);
}
function validate(entry_type,entry,header,sources) {
  if (entry_type=="oddNumbers") {
    return entry%2 != 0
  }
  if (entry_type=="profile") {
    return true
  }
  return false
}
function validateLink(linkEntryType,baseHash,linkHash,tag,sources){return true}
function genesis() {return true}
`

		testPath := root + "/test"
		if err = os.MkdirAll(testPath, os.ModePerm); err != nil {
			return nil, err
		}

		for name, z := range h.Zomes {

			Debugf("Process Z: %v, %v\n", name, z)
			zpath := h.ZomePath(&z)

			if err = os.MkdirAll(zpath, os.ModePerm); err != nil {
				return nil, err
			}

			c, _ := code[z.Name]
			if err = writeFile(zpath, z.Code, []byte(c)); err != nil {
				return
			}

			// both zomes have the same profile schma, this will be generalized for
			// scaffold building code.
			if err = writeFile(zpath, "profile.json", []byte(SampleSchema)); err != nil {
				return
			}

		}

		// write out the tests
		for i, d := range SampleFixtures {
			fn := fmt.Sprintf("test_%d.json", i)
			var j []byte
			t := []TestData{d}
			j, err = json.Marshal(t)
			if err != nil {
				return
			}
			if err = writeFile(testPath, fn, j); err != nil {
				return
			}
		}

		// also write out some grouped tests
		fn := "grouped.json"
		var j []byte
		j, err = json.Marshal(SampleFixtures2)
		if err != nil {
			return
		}
		if err = writeFile(testPath, fn, j); err != nil {
			return
		}
		hP = &h
		return
	})
	return
}

// maybe put in sample.go ?
// for generic sample
var SampleZomes = map[string]Zome{
	"zySampleZome": {
		Name:        "zySampleZome",
		Code:        "zySampleZome.zy",
		Description: "this is a zygomas test zome",
		NucleusType: ZygoNucleusType,
		Entries: map[string]EntryDef{
			"evenNumbers": {Name: "evenNumbers", DataFormat: DataFormatRawZygo, Sharing: Public},
			"primes":      {Name: "primes", DataFormat: DataFormatJSON, Sharing: Public},
			"profile":     {Name: "profile", DataFormat: DataFormatJSON, Schema: "profile.json", Sharing: Public},
		},
		Functions: map[string]FunctionDef{
			"getDNA":      {Name: "getDNA", CallingType: STRING_CALLING},
			"addEven":     {Name: "addEven", CallingType: STRING_CALLING},
			"addPrime":    {Name: "addPrime", CallingType: JSON_CALLING},
			"testStrFn1":  {Name: "testStrFn1", CallingType: STRING_CALLING},
			"testStrFn2":  {Name: "testStrFn2", CallingType: STRING_CALLING},
			"testJsonFn1": {Name: "testJsonFn1", CallingType: JSON_CALLING},
			"testJsonFn2": {Name: "testJsonFn2", CallingType: JSON_CALLING},
		},
	},
	"jsSampleZome": {
		Name:        "jsSampleZome",
		Code:        "jsSampleZome.js",
		Description: "this is a javascript test zome",
		NucleusType: JSNucleusType,
		Entries: map[string]EntryDef{
			"oddNumbers": {Name: "oddNumbers", DataFormat: DataFormatRawJS, Sharing: Public},
			"profile":    {Name: "profile", DataFormat: DataFormatJSON, Schema: "profile.json", Sharing: Public},
			"rating":     {Name: "rating", DataFormat: DataFormatLinks},
		},
		Functions: map[string]FunctionDef{
			"oddNumbers":  {Name: "getProperty", CallingType: STRING_CALLING},
			"oddOdd":      {Name: "addOdd", CallingType: STRING_CALLING},
			"oddProfile":  {Name: "addProfile", CallingType: JSON_CALLING},
			"testStrFn1":  {Name: "testStrFn1", CallingType: STRING_CALLING},
			"testStrFn2":  {Name: "testStrFn2", CallingType: STRING_CALLING},
			"testJsonFn1": {Name: "testJsonFn1", CallingType: JSON_CALLING},
			"testJsonFn2": {Name: "testJsonFn2", CallingType: JSON_CALLING},
		}},
}

var PropertiesSchema = `{
	"title": "Properties Schema",
	"type": "object",
	"properties": {
		"description": {
			"type": "string"
		},
		"language": {
			"type": "string"
		}
	}
}`

var SampleSchema = `{
	"title": "Profile Schema",
	"type": "object",
	"properties": {
		"firstName": {
			"type": "string"
		},
		"lastName": {
			"type": "string"
		},
		"age": {
			"description": "Age in years",
			"type": "integer",
			"minimum": 0
		}
	},
	"required": ["firstName", "lastName"]
}`

var SampleFixtures = [7]TestData{
	{
		Zome:   "zySampleZome",
		FnName: "addEven",
		Input:  "2",
		Output: "%h%"},
	{
		Zome:   "zySampleZome",
		FnName: "addEven",
		Input:  "4",
		Output: "%h%"},
	{
		Zome:   "zySampleZome",
		FnName: "addEven",
		Input:  "5",
		Err:    "Error calling 'commit': Invalid entry: 5"},
	{
		Zome:   "zySampleZome",
		FnName: "addPrime",
		Input:  "{\"prime\":7}",
		Output: "\"%h%\""}, // quoted because return value is json
	{
		Zome:   "zySampleZome",
		FnName: "addPrime",
		Input:  "{\"prime\":4}",
		Err:    `Error calling 'commit': Invalid entry: {"Atype":"hash", "prime":4, "zKeyOrder":["prime"]}`},
	{
		Zome:   "jsSampleZome",
		FnName: "addProfile",
		Input:  `{"firstName":"Art","lastName":"Brock"}`,
		Output: `"%h%"`},
	{
		Zome:   "zySampleZome",
		FnName: "getDNA",
		Input:  "",
		Output: "%dna%"},
}

var SampleFixtures2 = [2]TestData{
	{
		Zome:   "jsSampleZome",
		FnName: "addOdd",
		Input:  "7",
		Output: "%h%"},
	{
		Zome:   "jsSampleZome",
		FnName: "addOdd",
		Input:  "2",
		Err:    "Invalid entry: 2"},
}
