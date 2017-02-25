// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------
// Service implements functions and data that provide Holochain services

package holochain

import (
	"crypto/ecdsa"
	"github.com/BurntSushi/toml"
	"io/ioutil"
	"os"
)

// System settings, directory, and file names
const (
	DirectoryName   string = ".holochain"  // Directory for storing config data
	DNAFileName     string = "dna.conf"    // Group context settings for holochain
	LocalFileName   string = "local.conf"  // Setting for your local data store
	SysFileName     string = "system.conf" // Server & System settings
	AgentFileName   string = "agent.txt"   // User ID info
	PubKeyFileName  string = "pub.key"     // ECDSA Signing key - public
	PrivKeyFileName string = "priv.key"    // ECDSA Signing key - private
	StoreFileName   string = "chain.db"    // Filename for local data store

	DefaultPort = 6283

	DNAEntryType = "_dna"
	KeyEntryType = "_key"
)

// Holochain service configuration, i.e. Active Subsystems: DHT and/or Datastore, network port, etc
type Config struct {
	Port            int
	PeerModeAuthor  bool
	PeerModeDHTNode bool
}

// Holochain service data structure
type Service struct {
	Settings     Config
	DefaultAgent Agent
	DefaultKey   *ecdsa.PrivateKey
	Path         string
}

//IsInitialized checks a path for a correctly set up .holochain directory
func IsInitialized(path string) bool {
	root := path + "/" + DirectoryName
	return dirExists(root) && fileExists(root+"/"+SysFileName) && fileExists(root+"/"+AgentFileName)
}

//Init initializes service defaults including a signing key pair for an agent
func Init(path string, agent Agent) (return_svc *Service, err error) {
	service_path := path + "/" + DirectoryName
	err = os.MkdirAll(service_path, os.ModePerm)
	if err != nil {
		return
	}
	service := Service{
		Settings: Config{
			Port:           DefaultPort,
			PeerModeAuthor: true,
		},
		DefaultAgent: agent,
		Path:         service_path,
	}

	err = writeToml(service_path, SysFileName, service.Settings, false)
	if err != nil {
		return
	}

	writeFile(service_path, AgentFileName, []byte(agent))
	if err != nil {
		return
	}

	key, err := GenKeys(service_path)
	if err != nil {
		return
	}
	service.DefaultKey = key

	return_svc = &service
	return
}

func LoadService(path string) (return_svc *Service, err error) {
	agent, key, err := LoadSigner(path)
	if err != nil {
		return
	}
	service := Service{
		Path:         path,
		DefaultAgent: agent,
		DefaultKey:   key,
	}

	_, err = toml.DecodeFile(path+"/"+SysFileName, &service.Settings)
	if err != nil {
		return
	}

	return_svc = &service
	return
}

// Load unmarshals a holochain structure for the named chain in a service
func (service *Service) Load(name string) (return_ptr *Holochain, err error) {
	var new_chain Holochain

	path := service.Path + "/" + name

	_, err = toml.DecodeFile(path+"/"+DNAFileName, &new_chain)
	if err != nil {
		return
	}
	new_chain.path = path

	// try and get the agent/private_key from the holochain instance
	agent, private_key, err := LoadSigner(path)
	if err != nil {
		// get the default if not available
		agent, private_key, err = LoadSigner(filepath.Dir(path))
	}
	if err != nil {
		return
	}
	new_chain.agent = agent
	new_chain.privKey = private_key

	new_chain.store, err = CreatePersister(BoltPersisterName, path+"/"+StoreFileName)
	if err != nil {
		return
	}

	err = new_chain.store.Init()
	if err != nil {
		return
	}

	return_ptr = &new_chain
	return
}

// IsConfigured checks a directory for holochain configuration files
// (working path)/<service name>/dna.conf // from parameter and constant
// (working path)/<service name>/chain.db // same parameter and another constant
// (working path)/<service name>/<service name> // for any not SelfDefining
// (working path)/<service name>/<schema code>  // path from service dir
// doesn't this belong in service.go ?
func (service *Service) IsConfigured(name string) (hol_chain *Holochain, err error) {
	path := service.Path + "/" + name
	dna_path := path + "/" + DNAFileName
	if !fileExists(dna_path) {
		return nil, errors.New("missing " + dna_path)
	}
	store_path := path + "/" + StoreFileName
	if !fileExists(store_path) {
		return nil, errors.New("chain store missing: " + store_path)
	}

	hol_chain, err = service.Load(name)
	if err != nil {
		return
	}

  // maybe move to schema.go as CheckSchemas(hol_chain.Schemas)
  // or should it be LoadSchemas, but it just looks for files below
  // for now there is always one of these configured in dna.conf
  // myData (the Type) -> "zygo" which is a SelfDefiningType
  // JSON is also defined, unclear if it is fully plumbed
	for _, schema := range hol_chain.Schemas {
		schema_file := schema.Schema
		if !SelfDescribingSchema(schema_file) {
			if !fileExists(path + "/" + schema_file) {
				return nil, errors.New("DNA specified schema missing: " + schema_file)
			}
		}
		code_file := schema.Code
		if !fileExists(path + "/" + code_file) {
			return nil, errors.New("DNA specified code missing: " + code_file)
		}
	}
	return
}

// ConfiguredChains returns a list of the configured chains for the given service
func (service *Service) ConfiguredChains() (chains map[string]*Holochain, err error) {
	files, err := ioutil.ReadDir(s.Path)
	if err != nil {
		return
	}
	chains = make(map[string]*Holochain)
	for _, f := range files {
		if f.IsDir() {
			hol_chain, err := service.IsConfigured(f.Name())
			if err == nil {
				chains[f.Name()] = hol_chain
			}
		}
	}
	return
}

