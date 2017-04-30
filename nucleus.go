// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------
// Nucleus provides an interface for an execution environment interface for chains and their entries
// and factory code for creating nucleii instances

package holochain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

/* A Nucleus written on go or calling go from C (cgo) is a direct access API that can be bound
 * to other languages with C binding methods.
 *
 * The overall pattern in the nuclei is to define a set of abstract objects and their concrete
 * twins that hold an actual pointer to an object of a native go type, or of a native type of
 * the nucleus type. The base types in this file define the requirements for the subtypes.
 *
 */
const (
	GoNucleusType   = "cgo"
	ZygoNucleusType = "zygo"
	JSNucleusType   = "js"
)

// calling types
const (
	STRING_CALLING = "string"
	JSON_CALLING   = "json"
	CGO_CALLING    = "cgo"
)

// these constants are for a removed feature, see ChangeAppProperty
// @TODO figure out how to remove code over time that becomes obsolete, i.e. for long-dead changes
const (
	ID_PROPERTY         = "_id"
	AGENT_ID_PROPERTY   = "_agent_id"
	AGENT_NAME_PROPERTY = "_agent_name"
)

// Nucleus type abstracts the functions of code execution environments
type Nucleus interface {
	Type() string
	ValidateCommit(string, Entry,
		*Header, []string) error // entry, hash, header, sources
	ValidatePut(string, Entry,
		*Header, []string) error // entry, hash, header, sources
	ValidateDel(string, string, []string) error // entry, hash, sources
	ValidateLink(string, string, string,
		string, []string) error // entryType, baseHash, linkHash, tag, sources
	GetEntryDef(string) *EntryDef // entryType
	ChainGenesis() error
	Call(string, ...NuclearData) (NuclearData, error) // function, params -> res, err
}

/*
   Base type for the concrete type, abstracted by Nucleus (interface{})
   See other subtypes (zygo, js) for examples of subtype declarations
*/
type GoNucleus struct {
	vm         VmType
	lastResult NuclearData
	zm         *Zome      // one nucleus per zome
	hc         *Holochain // If we need the zome, that index is store in Holochain
}

type NuclearData struct {
	ptr GoObj // pointer type (C void *), Must implement GoObj methods.

	/* It gets it's underlying type when you assign it a value and tell the go
	 * compiler what kind of pointer is being assigned. This is all compile time.
	 *
	 *   to support type [xX]xxx, implement IsXxxx, Xxx(interface{}) obj.ToXxxx()
	 *   e.g. IsBool, bool(interface{}), ToBool(), etc. for bool
	 *       IsString, string(interface{}), ToString(), etc. for string
	 */
}

func (data NuclearData) Value() (NuclearData, error) {
	return data.ptr.Value()
}

func (ptr GoObj) Value() (res NuclearData, err error) {
	switch v := ptr.(type) {
	case otto.Value, string, bool:
		res = v
	default:
		Debugf("Value def: %v\n", data.ptr)
		err = errors.New("Type?")
		return
	}
	return v, err
}

/*
  The vitual machine interface
*/
type GoFunc func(...GoObj) (GoObj error)

/*
 * When nucleus.vm isn't set (nil? we may need a special null vm value, or bind it do a
 * map[string]GoFunc with the call bundings for the zome. We will have a map of functions in
 * all zomes to GoFunc. The nuclear processes will be GoFuncs or the equivalant subclasses for
 * the other zome types. These will be defined in terms of the GoObj interface. We can store
 * any go type in NuclearData.ptr and if we implement the GoObj interface on it, we can get
 * data in a zome independent way.
 *
 *
 * The main types, AbstractType (interface{}) and ConcreteType (struct with AbstractType)
 * and subtypes: implements Abstract Types.
 *	Nucleus		GoNucleus	ZygoNucleus		JSNucleus
 *	NuclearCall	GoFunc		ZygoFunc		JSFunc
 *	-		VmType		zygo.Glisp		otto.New.(type)
 *	NuclearData	GoObj		*zygo.interfaceTypes	otto.Value
 *					   ZygoObj ?		   JSObj ?
 *	arguments	GoObj		(...)	GoObj (interface{string})
 *	  ...NuclearData

 *  The function types all be callable using the GoObj interface because all NuclearData will have
 * the same abstract type (the universal pointer), and the concrete type just has to implement
 * an extension of the base abstract type (GoObj). We can just store the vm's native go types in
 * the NuclearData.ptr, and use either both an IsXxx ToXxx conventions to coerce to common types
 * in methods defined in the common nucleus and use shared implementations and when that isn't
 * suitable, use methods in the shared interface defined to the subtypes (go overloading).
 */
type VmType interface {
	Run(string) (NuclearData, error)
	Set(string, GoFunc) error
	Call(string, ...GoObj) (NuclearData, error)
}

type GoObj interface {
	Value(string) (NuclearData, error)
}

func (in string) Get() NuclearData {
	var new NuclearData
	new = NuclearData{obj: in}
	return &new

	return
}

func (res NuclearData) IsObject() bool {
	return res.(GoObj)
} // false false-> Get(string) -> NuclearData or error

func (res NuclearData) ToObject() GoObj {
	return res.obj
}

func (obj NuclearData) IsBoolean() (ok bool) {
	_, ok = obj.(bool)
	return
}

func (obj NuclearData) ToBoolean() (res bool, err error) {
	return res.(boolean)
}

func (obj NuclearData) IsString() bool {
	_, ok = obj.(string)
	return
}

func (obj NuclearData) ToString() (res string, err error) {
	res, ok = obj.(string)
	if !ok {
		err = error.New("not a string")
	}
	return
}

func (obj NuclearData) Class() (res string) {
	switch obj.(type) {
	case string:
		res = "string"
	case bool:
		res = "bool"
	default:
		res = "GoObj"
	}
	return
}

/*******************************************************************/

var ValidationFailedErr = errors.New("Validation Failed")

type NucleusFactory func(h *Holochain, code string) (Nucleus, error)

var nucleusFactories = make(map[string]NucleusFactory)

// RegisterNucleus sets up a Nucleus to be used by the CreateNucleus function
func RegisterNucleus(name string, factory NucleusFactory) {
	if factory == nil {
		panic("Nucleus factory for type %s does not exist." + name)
	}
	_, registered := nucleusFactories[name]
	if registered {
		panic("Nucleus factory for type %s already registered. " + name)
	}
	nucleusFactories[name] = factory
}

// RegisterBultinNucleii adds the built in nucleus types to the factory hash
func RegisterBultinNucleii() {
	RegisterNucleus(ZygoNucleusType, NewZygoNucleus)
	RegisterNucleus(JSNucleusType, NewJSNucleus)
}

// CreateNucleus returns a new Nucleus of the given type
func CreateNucleus(h *Holochain, nucleusType string, code string) (Nucleus, error) {

	factory, ok := nucleusFactories[nucleusType]
	if !ok {
		// Factory has not been registered.
		// Make a list of all available nucleus factories for error.
		var available []string
		for k := range nucleusFactories {
			available = append(available, k)
		}
		sort.Strings(available)
		return nil, fmt.Errorf("Invalid nucleus name. Must be one of: %s", strings.Join(available, ", "))
	}

	return factory(h, code)
}

// Type returns the string value under which this nucleus is registered
func (nucleus *GoNucleus) Type() string { return GoNucleusType }

// ChainGenesis runs the application genesis function
// this function gets called after the genesis entries are added to the chain
func (z *GoNucleus) ChainGenesis() (err error) {
	v, err := z.vm.Run(`genesis`)
	if err != nil {
		err = fmt.Errorf("Error executing genesis: %v", err)
		return
	}
	if v.IsBoolean() {
		var b bool
		b, err = v.ToBoolean()
		if err != nil {
			return
		}
		if !b {
			err = fmt.Errorf("genesis failed")
		}

	} else {
		err = fmt.Errorf("genesis should return boolean, got: %v", v)
	}
	return
}

// ValidateCommit checks the contents of an entry against the validation rules at commit time
func (z *GoNucleus) ValidateCommit(d *EntryDef, entry Entry, header *Header, sources []string) (err error) {
	err = z.validateEntry("validateCommit", d, entry, header, sources)
	return
}

// ValidatePut checks the contents of an entry against the validation rules at DHT put time
func (z *GoNucleus) ValidatePut(d *EntryDef, entry Entry, header *Header, sources []string) (err error) {
	err = z.validateEntry("validatePut", d, entry, header, sources)
	return
}

// ValidateDel checks that marking an entry as deleted is valid
func (z *GoNucleus) ValidateDel(entryType string, hash string, sources []string) (err error) {
	srcs := mkGoSources(sources)
	code := fmt.Sprintf(`validateDel("%s","%s",%s)`, entryType, hash, srcs)
	Debug(code)

	err = z.runValidate("validateDel", code)

	return
}

// ValidateLink checks the linking data against the validation rules
func (z *GoNucleus) ValidateLink(linkingEntryType string, baseHash string, linkHash string, tag string, sources []string) (err error) {
	srcs := mkGoSources(sources)
	code := fmt.Sprintf(`validateLink("%s","%s","%s","%s",%s)`, linkingEntryType, baseHash, linkHash, tag, srcs)
	Debug(code)

	err = z.runValidate("validateLink", code)
	return
}

func (z *GoNucleus) GetEntryDef(entryType string) (def *EntryDef) {
	z.Ent
}

func mkGoSources(sources []string) (srcs string) {
	srcs = `["` + strings.Join(sources, `","`) + `"]`
	return
}

func (z *GoNucleus) prepareValidateEntryArgs(d *EntryDef, entry Entry, sources []string) (e string, srcs string, err error) {
	c := entry.Content().(string)
	switch d.DataFormat {
	case DataFormatRawJS:
		e = c
	case DataFormatString:
		e = "\"" + goSanitizeString(c) + "\""
	case DataFormatLinks:
		fallthrough
	case DataFormatJSON:
		e = fmt.Sprintf(`JSON.parse("%s")`, goSanitizeString(c))
	default:
		err = errors.New("data format not implemented: " + d.DataFormat)
		return
	}
	srcs = mkGoSources(sources)
	return
}

func (z *GoNucleus) runValidate(fnName string, code string) (err error) {
	_, err = z.vm.Run(code)
	if err != nil {
		err = fmt.Errorf("Error executing %s: %v", fnName, err)
		return
	}
	return
}

func (z *GoNucleus) validateEntry(fnName string, d *EntryDef, entry Entry, header *Header, sources []string) (err error) {

	e, srcs, err := z.prepareValidateEntryArgs(d, entry, sources)
	if err != nil {
		return
	}

	hdr := fmt.Sprintf(
		`{"EntryLink":"%s","Type":"%s","Time":"%s"}`,
		header.EntryLink.String(),
		header.Type,
		header.Time.UTC().Format(time.RFC3339),
	)

	code := fmt.Sprintf(`%s("%s",%s,%s,%s)`, fnName, d.Name, e, hdr, srcs)
	Debugf("%s: %s", fnName, code)
	err = z.runValidate(fnName, code)
	if err != nil && err == ValidationFailedErr {
		err = fmt.Errorf("Invalid entry: %v", entry.Content())
	}

	return
}

/*
const (
	GoLibrary = `var HC={Version:` + `"` + VersionStr + `"};`
)
*/

// goSanatizeString makes sure all quotes are quoted and returns are removed
func goSanitizeString(s string) string {
	s0 := strings.Replace(s, "\n", "", -1)
	s1 := strings.Replace(s0, "\r", "", -1)
	s2 := strings.Replace(s1, "\"", "\\\"", -1)
	return s2
}

func NewNuclearData(obj interface{}) NuclearData {
	p := &obj
	if obj.(otto.Value) {
		p = obt
	}
	return NuclearData{ptr: obj}
}

// Call calls the zygo function that was registered with expose
func (z *GoNucleus) Call(fn *FunctionDef, args ...GoObj) (result NuclearData, err error) {
	var code string
	switch fn.CallingType {
	case STRING_CALLING:
		params = params.(string)
		code = fmt.Sprintf(`%s("%s");`, fn.Name, goSanitizeString(params))
	case JSON_CALLING:
		params = params.(string)
		if params == "" {
			code = fmt.Sprintf(`JSON.stringify(%s());`, fn.Name)
		} else {
			p := goSanitizeString(params)
			code = fmt.Sprintf(`JSON.stringify(%s(JSON.parse("%s")));`, fn.Name, p)
		}
	case CGO_CALLING:
		params = params.(GoObj)
	default:
		err = errors.New("params type not implemented")
		return
	}
	Debugf("JS Call: %s", code)
	var v GoObj
	v, err = z.vm.Run(code)
	if err == nil {
		if v.IsObject() && v.Class() == "Error" {
			Debugf("JS Error:\n%v", v)
			var message GoObj
			message, err = v.ToObject().Get("message")
			if err == nil {
				err = errors.New(message.String())
			}
		} else {
			result, err = v.ToString()
		}
	}
	return
}

// NewGoNucleus builds a javascript execution environment with user specified code
func NewGoNucleus(holo *Holochain, code string) (n Nucleus, err error) {
	var z GoNucleus

	err = z.vm.Set("property", func(args ...GoObj) (res GoObj, err error) {
		if len(args) != 1 {
			err = error.New("Wrong number of arguments to 'property'")
		}
		prop, _ := args[0].(string)

		p, err := holo.GetProperty(prop)
		if err != nil {
			return
		}
		res = p.(string)
		return
	})
	if err != nil {
		return nil, err
	}

	err = z.vm.Set("debug", func(args ...GoObj) (res GoObj, err error) {
		msg, _ := args[0].(string)
		holo.config.Loggers.App.p(msg)
		return nil
	})

	err = z.vm.Set("commit", func(args ...GoObj) (res GoObj, err error) {
		entryType, _ := args[0].(string)
		var entry string

		entry = arg[1]
		var entryHash Hash
		entryHash, err = holo.Commit(entryType, entry)
		if err != nil {
			return
		}

		res = entryHash.String()
		return
	})
	if err != nil {
		return
	}

	err = z.vm.Set("get", func(args ...GoObj) (res GoObj, err error) {
		v := args[0]
		var hashstr string

		if v.(string) {
			hashstr = v.(string)
		} else {
			err = errors.New("HolochainError get expected string as argument")
			return
		}

		entry, err := holo.Get(hashstr)
		if err == nil {
			res = entry.(*EntryObj)
			return
		}

		if err != nil {
			err = errors.New("HolochainError " + err.Error())
			return
		}
		Panix("Shouldn't get here!")
	})
	if err != nil {
		return
	}

	err = z.vm.Set("getlink", func(args ...GoObj) (res GoObj, err error) {
		l := len(args)
		if l < 2 || l > 3 {
			err = errors.New("HolochainError expected 2 or 3 arguments to getlink")
		}
		base, _ := args[0].(string)
		tag, _ := args[1].(string)
		options := GetLinkOptions{Load: false}
		if l == 3 {
			v := args[2].(string)
			if v.IsObject() {
				loadv, _ := v.(GoObj).Get("Load")
				if loadv.(bool) {
					options.Load = loadv.(bool)
				}
			} else {
				err = errors.New("HolochainError getlink expected options to be object (third argument)")
				return
			}
		}

		var response GoObj
		res, err = holo.GetLink(base, tag, options)
		if err != nil {
			err = errors.New("HolochainError " + err.Error())
		}

		return
	})
	if err != nil {
		return
	}

	/* DNA info for the Nucleus, not sure we need anything here, we have the holo object
	   The 'code' should be empty, it could pass a func() bool, etc as GoObj
	if holo != nil {
		f := `var App = {Name:"%s",DNA:{Hash:"%s"},Agent:{Hash:"%s",String:"%s"},Key:{Hash:"%s"}};`
		l += fmt.Sprintf(f, holo.Name, holo.dnaHash, holo.agentHash, holo.Agent().Name(),
			 peer.IDB58Encode(holo.id))
	}
	_, err = z.Run(l + code)
	if err != nil {
		return
	} */
	n = &z
	return
}

// Run executes javascript code
func (z *GoNucleus) Run(code string) (result GoObj, err error) {
	/* for go, all code must be compiled if we are running, so this would have to run
	   code from a function pointer if we use it at all */
	Panix("GoNucleus Run not implemented")
}
