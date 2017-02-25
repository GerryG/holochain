//
// Chain origin and related signatures
//

package holochain

// Signing key structure for building KEYEntryType entries
type KeyEntry struct {
	ID  Agent
	Key []byte // marshaled x509 public key
}

// GenChain establishes a holochain instance by creating the initial genesis entries in the chain
// It assumes a properly set up .holochain sub-directory with a config file and
// keys for signing.  See GenDev()
func (hol_chain *Holochain) GenChain() (key_hash Hash, err error) {

	_, err = hol_chain.ID()
	if err == nil {
		err = mkErr("chain already started")
		return
	}

	var dna_gob_buffer bytes.Buffer
	err = hol_chain.EncodeDNA(&dna_gob_buffer)

	gob_entry := GobEntry{C: dna_gob_buffer.Bytes()}

	_, dna_header, err := hol_chain.AddEntry(time.Now(), DNAEntryType, &gob_entry)
	if err != nil {
		return
	}

	var key_entry KeyEntry
	key_entry.ID = hol_chain.agent

	public_key, err := x509.MarshalPKIXPublicKey(h.privKey.Public().(*ecdsa.PublicKey))
	if err != nil {
		return
	}
	key_entry.Key = public_key

	gob_entry.C = key_entry
	key_hash, _, err = hol_chain.AddEntry(time.Now(), KeyEntryType, &gob_entry)
	if err != nil {
		return
	}

	err = hol_chain.store.PutMeta(IDMetaKey, dna_header.EntryLink[:])
	if err != nil {
		return
	}

	return
}

// Gen adds template files suitable for editing to the given path
func GenDev(path string) (return_pointer *Holochain, err error) {
	var hol_chain Holochain
	if err := os.MkdirAll(path, os.ModePerm); err != nil {
		return nil, err
	}
	agent, key, err := LoadSigner(filepath.Dir(path))
	if err != nil {
		return
	}

	defs := []Schema{
		Schema{Name: "myData", Schema: "zygo"},
	}

	hol_chain = NewHolochain(agent, key, path, defs...)

	hol_chain.Name = filepath.Base(path)
	//	if err = writeFile(path,"myData.cp",[]byte(s)); err != nil {return}  //if captain proto...

	entries := make(map[string][]string)
	mde := [2]string{"2", "4"}
	entries["myData"] = mde[:]

	code := make(map[string]string)
	code["myData"] = `
(defn validate [entry] (cond (== (mod entry 2) 0) true false))
(defn validateChain [entry user_data] true)
`
	testPath := path + "/test"
	if err := os.MkdirAll(testPath, os.ModePerm); err != nil {
		return nil, err
	}

	for idx, d := range defs {
		entry_type := d.Name
		fn := fmt.Sprintf("valid_%s.zy", entry_type)
		hol_chain.Schemas[idx].Code = fn
		v, _ := code[entry_type]
		if err = writeFile(path, fn, []byte(v)); err != nil {
			return
		}
		for i, e := range entries[entry_type] {
			fn = fmt.Sprintf("%d_%s.zy", i, entry_type)
			if err = writeFile(testPath, fn, []byte(e)); err != nil {
				return
			}
		}
	}

	hol_chain.store, err = CreatePersister(BoltPersisterName, path+"/"+StoreFileName)
	if err != nil {
		return
	}

	err = hol_chain.store.Init()
	if err != nil {
		return
	}

	err = hol_chain.SaveDNA(false)
	if err != nil {
		return
	}

	return_pointer = &hol_chain
	return
}

// EncodeDNA encodes a holochain's DNA to an io.Writer
// we use toml so that the DNA is human readable
func (hol_chain *Holochain) EncodeDNA(writer io.Writer) (err error) {
	enc := toml.NewEncoder(writer)
	err = enc.Encode(hol_chain)
	return
}

// SaveDNA writes the holochain DNA to a file
func (hol_chain *Holochain) SaveDNA(overwrite bool) (err error) {
	p := hol_chain.path + "/" + DNAFileName
	if !overwrite && fileExists(p) {
		return mkErr(p + " already exists")
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	err = hol_chain.EncodeDNA(f)
	return
}

// GenDNAHashes generates hashes for all the definition files in the DNA.
// This function should only be called by developer tools at the end of the process
// of finalizing DNA development or versioning
func (hol_chain *Holochain) GenDNAHashes() (err error) {
	var b []byte
	for _, edef := range hol_chain.Schemas {
		schema_file := edef.Schema
		if !SelfDescribingSchema(schema_file) {
			b, err = readFile(hol_chain.path, schema_file)
			if err != nil {
				return
			}
      // should be using a wrapper class that selects the configured sig
			edef.SchemaHash = Hash(sha256.Sum256(b))
		}
		schema_file = edef.Code // ???
		b, err = readFile(hol_chain.path, schema_file)
		if err != nil {
			return
		}
		edef.CodeHash = Hash(sha256.Sum256(b))
	}
	err = hol_chain.SaveDNA(true)
	return
}

//LoadSigner gets the agent and signing key from the specified directory
func LoadSigner(path string) (agent Agent, private_key *ecdsa.PrivateKey, err error) {
	a, err := readFile(path, AgentFileName)
	if err != nil {
		return
	}
	agent = Agent(a)
	private_key, err = UnmarshalPrivateKey(path, PrivKeyFileName)
	return
}
