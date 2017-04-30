// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------
// ZygoNucleus implements a zygomys use of the Nucleus interface

package holochain

import (
	"encoding/json"
	"errors"
	"fmt"
	zygo "github.com/glycerine/zygomys/repl"
	peer "github.com/libp2p/go-libp2p-peer"
	"math"
	"strconv"
	"strings"
	"time"
)

type ZygoNucleus struct {
	GoNucleus
	library string
}

// Type returns the string value under which this nucleus is registered
func (z *ZygoNucleus) Type() string { return ZygoNucleusType }

// ChainGenesis runs the application genesis function
// this function gets called after the genesis entries are added to the chain
func (z *ZygoNucleus) ChainGenesis() (err error) {
	err = z.vm.LoadString(`(genesis)`)
	if err != nil {
		return
	}
	result, err := z.vm.Run()
	if err != nil {
		err = fmt.Errorf("Error executing genesis: %v", err)
		return
	}
	switch result.(type) {
	case *zygo.SexpBool:
		r := result.(*zygo.SexpBool).Val
		if !r {
			err = fmt.Errorf("genesis failed")
		}
	case *zygo.SexpSentinel:
		err = errors.New("genesis should return boolean, got nil")

	default:
		err = errors.New("genesis should return boolean, got: " + fmt.Sprintf("%v", result))
	}
	return

}

// ValidateCommit checks the contents of an entry against the validation rules at commit time
func (z *ZygoNucleus) ValidateCommit(entryType string, entry Entry, header *Header, sources []string) (err error) {
	err = z.validateEntry("validateCommit", entryType, entry, header, sources)
	return
}

// ValidatePut checks the contents of an entry against the validation rules at DHT put time
func (z *ZygoNucleus) ValidatePut(def *EntryDef, entry Entry, header *Header, sources []string) (err error) {
	err = z.validateEntry("validatePut", def, entry, header, sources)
	return
}

// ValidateDel checks that marking an entry as deleted is valid
func (z *ZygoNucleus) ValidateDel(def *EntryDef, hash string, sources []string) (err error) {
	srcs := mkSources(sources)
	code := fmt.Sprintf(`(validateDel "%s" "%s" %s)`, def.Name, hash, srcs)
	Debug(code)
	err = z.runValidate("validateDel", code)
	return
}

// ValidateLink checks the link data against the validation rules
func (z *ZygoNucleus) ValidateLink(def *EntryDef, baseHash string, linkHash string, tag string, sources []string) (err error) {

	srcs := mkSources(sources)
	code := fmt.Sprintf(`(validateLink "%s" "%s" "%s" "%s" %s)`, def.Name, baseHash, linkHash, tag, srcs)
	Debug(code)

	err = z.runValidate("validateLink", code)
	return
}

func mkSources(sources []string) (srcs string) {
	var err error
	var b []byte
	b, err = json.Marshal(sources)
	if err != nil {
		return
	}
	srcs = fmt.Sprintf(`(unjson (raw "%s"))`, sanitizeString(string(b)))
	return
}

func (z *ZygoNucleus) prepareValidateArgs(entryType string, entry Entry, sources []string) (e string, srcs string, err error) {
	def, err := z.hc.GetEntryDef(entryType)
	if err != nil {
		return
	}
	c := entry.Content().(string)
	// @todo handle JSON if schema type is different
	switch def.DataFormat {
	case DataFormatRawZygo:
		e = c
	case DataFormatString:
		e = "\"" + sanitizeString(c) + "\""
	case DataFormatLinks:
		fallthrough
	case DataFormatJSON:
		e = fmt.Sprintf(`(unjson (raw "%s"))`, sanitizeString(c))
	default:
		err = errors.New("data format not implemented: " + def.DataFormat)
		return
	}
	srcs = mkSources(sources)
	return
}

func (z *ZygoNucleus) runValidate(fnName string, code string) (err error) {
	err = z.vm.LoadString(code)
	if err != nil {
		return
	}
	result, err := z.vm.Run()
	if err != nil {
		err = fmt.Errorf("Error executing %s: %v", fnName, err)
		return
	}
	switch result.(type) {
	case *zygo.SexpBool:
		r := result.(*zygo.SexpBool).Val
		if !r {
			err = ValidationFailedErr
		}
	case *zygo.SexpSentinel:
		err = fmt.Errorf("%s should return boolean, got nil", fnName)

	default:
		err = fmt.Errorf("%s should return boolean, got: %v", fnName, result)
	}
	return
}

func (z *ZygoNucleus) validateEntry(fnName string, entryType string, entry Entry, header *Header, sources []string) (err error) {
	e, srcs, err := z.prepareValidateArgs(entryType, entry, sources)
	if err != nil {
		return
	}

	var hdr string
	if header != nil {
		hdr = fmt.Sprintf(
			`(hash EntryLink:"%s" Type:"%s" Time:"%s")`,
			header.EntryLink.String(),
			header.Type,
			header.Time.UTC().Format(time.RFC3339),
		)
	} else {
		hdr = `""`
	}

	code := fmt.Sprintf(`(%s "%s" %s %s %s)`, fnName, entryType, e, hdr, srcs)
	Debugf("%s: %s", fnName, code)

	err = z.runValidate(fnName, code)
	if err != nil && err == ValidationFailedErr {
		err = fmt.Errorf("Invalid entry: %v", entry.Content())
	}
	return
}

// sanatizeString makes sure all quotes are quoted
func sanitizeString(s string) string {
	s = strings.Replace(s, "\"", "\\\"", -1)
	return s
}

// Call calls the zygo function that was registered with expose
func (z *ZygoNucleus) Call(function string, params interface{}) (result interface{}, err error) {
	fnDef := z.zome.GetNucleus(function)
	var code string
	switch fnDef.CallingType {
	case STRING_CALLING:
		code = fmt.Sprintf(`(%s "%s")`, fnDef.Name, sanitizeString(params.(string)))
	case JSON_CALLING:
		if params.(string) == "" {
			code = fmt.Sprintf(`(json (%s (raw "%s")))`, fnDef.Name, sanitizeString(params.(string)))
		} else {
			code = fmt.Sprintf(`(json (%s (unjson (raw "%s"))))`, fnDef.Name, sanitizeString(params.(string)))
		}
	default:
		err = errors.New("params type not implemented")
		return
	}
	Debugf("Zygo Call: %s", code)
	err = z.vm.LoadString(code)
	if err != nil {
		return
	}
	result, err = z.vm.Run()
	if err == nil {
		switch fnDef.CallingType {
		case STRING_CALLING:
			switch t := result.(type) {
			case *zygo.SexpStr:
				result = t.S
			case *zygo.SexpInt:
				result = fmt.Sprintf("%d", t.Val)
			case *zygo.SexpRaw:
				result = string(t.Val)
			default:
				result = fmt.Sprintf("%v", result)
			}
		case JSON_CALLING:
			// type should always be SexpRaw
			switch t := result.(type) {
			case *zygo.SexpRaw:
				result = t.Val
			default:
				err = errors.New("expected SexpRaw return type")
			}
		}

	}
	return
}

// These are the zygo implementations of the library functions that must available in
// all Nucleii implementations.
var ZygoLibrary = `(def HC_Version "` + VersionStr + `")`

// get exposes DHTGet to zygo
func (z *ZygoNucleus) get(vm *zygo.Glisp, h *Holochain, hash string) (result *zygo.SexpHash, err error) {
	result, err = zygo.MakeHash(nil, "hash", vm)
	if err != nil {
		return nil, err
	}

	var entry interface{}
	entry, err = h.Get(hash)
	if err == nil {
		t := entry.(*EntryObj)
		// @TODO figure out encoding by entry type.
		j, err := json.Marshal(t.C)
		if err == nil {
			err = result.HashSet(vm.MakeSymbol("result"), &zygo.SexpStr{S: string(j)})
		}
	} else {
		err = result.HashSet(vm.MakeSymbol("error"), &zygo.SexpStr{S: err.Error()})
	}
	return result, err
}

// getlink exposes GetLink to zygo
func (z *ZygoNucleus) getlink(vm *zygo.Glisp, h *Holochain, base string, tag string, options GetLinkOptions) (result *zygo.SexpHash, err error) {
	result, err = zygo.MakeHash(nil, "hash", vm)
	if err != nil {
		return nil, err
	}

	var response *LinkQueryResp
	response, err = h.GetLink(base, tag, options)

	if err == nil {
		var j []byte
		j, err = json.Marshal(response.Links)
		if err == nil {
			err = result.HashSet(vm.MakeSymbol("result"), &zygo.SexpStr{S: string(j)})
		}
	} else {
		err = result.HashSet(vm.MakeSymbol("error"), &zygo.SexpStr{S: err.Error()})
	}
	return result, err
}

// NewZygoNucleus builds an zygo execution vm with user specified code
func NewZygoNucleus(h *Holochain, code string) (n Nucleus, err error) {
	var z ZygoNucleus
	z.vm = zygo.NewGlispSandbox()
	z.vm.AddFunction("version",
		func(vm *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			return &zygo.SexpStr{S: VersionStr}, nil
		})

	addExtras(&z)

	// use a closure so that the registered zygo function can call Expose on the correct ZygoNucleus obj

	z.vm.AddFunction("debug",
		func(vm *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 1 {
				return zygo.SexpNull, zygo.WrongNargs
			}

			var msg string

			switch t := args[0].(type) {
			case *zygo.SexpStr:
				msg = t.S
			case *zygo.SexpInt:
				msg = fmt.Sprintf("%d", t.Val)
			case *zygo.SexpHash:
				msg = zygo.SexpToJson(t)
			case *zygo.SexpArray:
				msg = zygo.SexpToJson(t)
			default:
				return zygo.SexpNull,
					fmt.Errorf("can't convert argument type %T", t)
			}

			h.config.Loggers.App.p(msg)
			return zygo.SexpNull, err
		})

	z.vm.AddFunction("property",
		func(vm *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 1 {
				return zygo.SexpNull, zygo.WrongNargs
			}

			var prop string

			switch t := args[0].(type) {
			case *zygo.SexpStr:
				prop = t.S
			default:
				return zygo.SexpNull,
					errors.New("1st argument of expose should be string")
			}

			p, err := h.GetProperty(prop)
			if err != nil {
				return zygo.SexpNull, err
			}
			result := zygo.SexpStr{S: p}
			return &result, err
		})

	z.vm.AddFunction("commit",
		func(vm *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 2 {
				return zygo.SexpNull, zygo.WrongNargs
			}

			var entryType string
			var entry string

			switch t := args[0].(type) {
			case *zygo.SexpStr:
				entryType = t.S
			default:
				return zygo.SexpNull,
					errors.New("1st argument of commit should be string")
			}

			switch t := args[1].(type) {
			case *zygo.SexpStr:
				entry = t.S
			case *zygo.SexpHash:
				entry = zygo.SexpToJson(t)
			default:
				return zygo.SexpNull,
					errors.New("2nd argument of commit should be string or hash")
			}

			var entryHash Hash
			entryHash, err = h.Commit(entryType, entry)

			if err != nil {
				return zygo.SexpNull, err
			}
			var result = zygo.SexpStr{S: entryHash.String()}
			return &result, nil
		})

	z.vm.AddFunction("get",
		func(vm *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			if len(args) != 1 {
				return zygo.SexpNull, zygo.WrongNargs
			}

			var hashstr string
			switch t := args[0].(type) {
			case *zygo.SexpStr:
				hashstr = t.S
			default:
				return zygo.SexpNull,
					errors.New("argument of get should be string")
			}
			result, err := z.get(vm, h, hashstr)
			return result, err
		})

	z.vm.AddFunction("getlink",
		func(vm *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {
			l := len(args)
			if l < 2 || l > 3 {
				return zygo.SexpNull, zygo.WrongNargs
			}

			var hashstr string
			switch t := args[0].(type) {
			case *zygo.SexpStr:
				hashstr = t.S
			default:
				return zygo.SexpNull,
					errors.New("1st argument of getlink should be string")
			}

			var typestr string
			switch t := args[1].(type) {
			case *zygo.SexpStr:
				typestr = t.S
			default:
				return zygo.SexpNull,
					errors.New("2nd argument of getlink should be string")
			}

			options := GetLinkOptions{Load: false}
			if l == 3 {
				switch t := args[2].(type) {
				case *zygo.SexpHash:
					r, err := t.HashGet(z.vm, z.vm.MakeSymbol("Load"))
					if err == nil {
						switch t := r.(type) {
						case *zygo.SexpBool:
							options.Load = t.Val
						default:
							return zygo.SexpNull,
								errors.New("Load must be a boolean")
						}
					}
				default:
					return zygo.SexpNull,
						errors.New("3rd argument of getlink should be hash")
				}

			}
			result, err := z.getlink(vm, h, hashstr, typestr, options)
			return result, err
		})

	l := ZygoLibrary
	if h != nil {
		l += fmt.Sprintf(`(def App_Name "%s")(def App_DNA_Hash "%s")(def App_Agent_Hash "%s")(def App_Agent_String "%s")(def App_Key_Hash "%s")`, h.Name, h.dnaHash, h.agentHash, h.Agent().Name(), peer.IDB58Encode(h.id))
	}
	z.library = l

	_, err = z.Run(l + code)
	if err != nil {
		return
	}
	n = &z
	return
}

type ZygoFunc func(...interface{}) (res interface{}, err errors)

// Run executes zygo code
func (z *ZygoNucleus) Run(code string) (result zygo.Sexp, err error) {
	c := fmt.Sprintf("(begin %s %s)", z.library, code)
	err = z.vm.LoadString(c)
	if err != nil {
		err = errors.New("Zygomys load error: " + err.Error())
		return
	}
	result, err = z.vm.Run()
	if err != nil {
		err = errors.New("Zygomys exec error: " + err.Error())
		return
	}
	z.lastResult = result
	return
}

// extra functions we want to have available for app developers in zygo

func isPrime(t int64) bool {

	// math.Mod requires floats.
	x := float64(t)

	// 1 or less aren't primes.
	if x <= 1 {
		return false
	}

	// Solve half of the integer set directly
	if math.Mod(x, 2) == 0 {
		return x == 2
	}

	// Main loop. i needs to be float because of math.Mod.
	for i := 3.0; i <= math.Floor(math.Sqrt(x)); i += 2.0 {
		if math.Mod(x, i) == 0 {
			return false
		}
	}

	// It's a prime!
	return true
}

func addExtras(z *ZygoNucleus) {
	z.vm.AddFunction("isprime",
		func(vm *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {

			switch t := args[0].(type) {
			case *zygo.SexpInt:
				return &zygo.SexpBool{Val: isPrime(t.Val)}, nil
			default:
				return zygo.SexpNull,
					errors.New("argument to isprime should be int")
			}
		})
	z.vm.AddFunction("atoi",
		func(vm *zygo.Glisp, name string, args []zygo.Sexp) (zygo.Sexp, error) {

			var i int64
			var e error
			switch t := args[0].(type) {
			case *zygo.SexpStr:
				i, e = strconv.ParseInt(t.S, 10, 64)
				if e != nil {
					return zygo.SexpNull, e
				}
			default:
				return zygo.SexpNull,
					errors.New("argument to atoi should be string")
			}

			return &zygo.SexpInt{Val: i}, nil
		})
}
