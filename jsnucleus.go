// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------
// JSNucleus implements a javascript use of the Nucleus interface

package holochain

import (
	"errors"
	"fmt"
	peer "github.com/libp2p/go-libp2p-peer"
	"github.com/robertkrimen/otto"
	"strings"
	"time"
)

type JSNucleus struct {
	GoNucleus
}

type JSNuclearData struct {
	ptr *otto.Value
}

func (data *JSNuclearData) Value() {
	data.(otto.Value)
}

// Type returns the string value under which this nucleus is registered
func (z *JSNucleus) Type() string { return JSNucleusType }

// ChainGenesis runs the application genesis function
// this function gets called after the genesis entries are added to the chain
func (z *JSNucleus) ChainGenesis() (err error) {
	v, err := z.vm.Run(`genesis()`)
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
func (z *JSNucleus) ValidateCommit(entryType string, entry Entry, header *Header, sources []string) (err error) {
	err = z.validateEntry("validateCommit", entryType, entry, header, sources)
	return
}

// ValidatePut checks the contents of an entry against the validation rules at DHT put time
func (z *JSNucleus) ValidatePut(entryType string, entry Entry, header *Header, sources []string) (err error) {
	err = z.validateEntry("validatePut", entryType, entry, header, sources)
	return
}

// ValidateDel checks that marking an entry as deleted is valid
func (z *JSNucleus) ValidateDel(entryType string, hash string, sources []string) (err error) {
	srcs := mkJSSources(sources)
	code := fmt.Sprintf(`validateDel("%s","%s",%s)`, entryType, hash, srcs)
	Debug(code)

	err = z.runValidate("validateDel", code)

	return
}

// ValidateLink checks the linking data against the validation rules
func (z *JSNucleus) ValidateLink(entryType string, baseHash string, linkHash string, tag string, sources []string) (err error) {
	srcs := mkJSSources(sources)
	code := fmt.Sprintf(`validateLink("%s","%s","%s","%s",%s)`, entryType, baseHash, linkHash, tag, srcs)
	Debug(code)

	err = z.runValidate("validateLink", code)
	return
}

func mkJSSources(sources []string) (srcs string) {
	srcs = `["` + strings.Join(sources, `","`) + `"]`
	return
}

func (z *JSNucleus) prepareValidateEntryArgs(entryType string, entry Entry, sources []string) (e string, srcs string, err error) {
	edef, err := z.hc.GetEntryDef(entryType)
	if err != nil {
		return
	}
	dataFormat := edef.DataFormat
	c := entry.Content().(string)
	switch dataFormat {
	case DataFormatRawJS:
		e = c
	case DataFormatString:
		e = "\"" + jsSanitizeString(c) + "\""
	case DataFormatLinks:
		fallthrough
	case DataFormatJSON:
		e = fmt.Sprintf(`JSON.parse("%s")`, jsSanitizeString(c))
	default:
		err = errors.New("data format not implemented: " + dataFormat)
		return
	}
	srcs = mkJSSources(sources)
	return
}

func (z *JSNucleus) runValidate(fnName string, code string) (err error) {
	data, err := z.vm.Run(code)
	if err != nil {
		err = fmt.Errorf("Error executing %s: %v", fnName, err)
		return
	}
	v, err := *data.Value()
	if err != nil {
		err = fmt.Errorf("Error converting return value from %s: %v", fnName, err)
		return
	}
	if v.IsBoolean() {
		if v.IsBoolean() {
			var b bool
			b, err = v.ToBoolean()
			if err != nil {
				return
			}
			if !b {
				err = ValidationFailedErr
			}
		}
	} else {
		err = fmt.Errorf("%s should return boolean, got: %v", fnName, v)
	}
	return
}

func (z *JSNucleus) validateEntry(fnName string, entryType string, entry Entry, header *Header, sources []string) (err error) {

	e, srcs, err := z.prepareValidateEntryArgs(entryType, entry, sources)
	if err != nil {
		return
	}

	hdr := fmt.Sprintf(
		`{"EntryLink":"%s","Type":"%s","Time":"%s"}`,
		header.EntryLink.String(),
		header.Type,
		header.Time.UTC().Format(time.RFC3339),
	)

	code := fmt.Sprintf(`%s("%s",%s,%s,%s)`, fnName, entryType, e, hdr, srcs)
	Debugf("%s: %s", fnName, code)
	err = z.runValidate(fnName, code)
	if err != nil && err == ValidationFailedErr {
		err = fmt.Errorf("Invalid entry: %v", entry.Content())
	}

	return
}

const (
	JSLibrary = `var HC={Version:` + `"` + VersionStr + `"};`
)

// jsSanatizeString makes sure all quotes are quoted and returns are removed
func jsSanitizeString(s string) string {
	s0 := strings.Replace(s, "\n", "", -1)
	s1 := strings.Replace(s0, "\r", "", -1)
	s2 := strings.Replace(s1, "\"", "\\\"", -1)
	return s2
}

// Call calls the zygo function that was registered with expose
func (z *JSNucleus) Call(function string, params interface{}) (result interface{}, err error) {
	callingType := z.GetFunctionDef(function).CallingType
	var code string
	switch CallingType {
	case STRING_CALLING:
		code = fmt.Sprintf(`%s("%s");`, function, jsSanitizeString(params.(string)))
	case JSON_CALLING:
		if params.(string) == "" {
			code = fmt.Sprintf(`JSON.stringify(%s());`, function)
		} else {
			p := jsSanitizeString(params.(string))
			code = fmt.Sprintf(`JSON.stringify(%s(JSON.parse("%s")));`, function, p)
		}
	default:
		err = errors.New("params type not implemented")
		return
	}
	Debugf("JS Call: %s", code)
	goObj, err := z.vm.Run(code)
	if err != nil {
		return
	}
	v, err := goObj.(otto.Value)
	if err != nil {
		err = fmt.Errorf("Error converting return value from %s: %v", fnName, err)
		return
	}
	if v.IsObject() && v.Class() == "Error" {
		Debugf("JS Error:\n%v", v)
		var message JSNuclearData
		message, err = v.Object().Get("message")
		if err == nil {
			err = errors.New(message.String())
		}
	} else {
		result, err = v.ToString()
	}
	return
}

// NewJSNucleus builds a javascript execution environment with user specified code
func NewJSNucleus(holo *Holochain, code string) (n Nucleus, err error) {
	var z = JSNucleus{vm: otto.New()}

	for fnName, jsFunc := range JSNucleusFuncs {
		err = z.vm.Set(fnName, func(call otto.FunctionCall) JSNuclearData {
			return z.vm.ToValue(jsFunc(call.ArgumentList))
		})
		if err != nil {
			return
		}
	}
	if holo != nil {
		l := fmt.Sprintf(
			`%svar App = {Name:"%s",DNA:{Hash:"%s"},Agent:{Hash:"%s",String:"%s"},Key:{Hash:"%s"}};`,
			JSLibrary,
			holo.Name,
			holo.dnaHash,
			holo.agentHash,
			holo.Agent().Name(),
			peer.IDB58Encode(holo.id))
	}
	_, err = z.Run(l + code)
	if err != nil {
		return
	}
	n = &z
	return
}

type JSFunc func(...otto.Value) (res interface{}, err error)

var JSNucleusFuncs = map[string]JSFunc{
	"property": property,
	"debug":    debug,
	"commit":   commit,
	"get":      get,
	"getlink":  getlink,
}

func property(args ...otto.Value) (prop JSNuclearData, err error) {
	propName, _ := call.args[0].ToString()

	prop, err = h.GetProperty(propName)
	return
}

func debug(args ...otto.Value) (res JSNuclearData, err error) {
	msg, _ := call.args[0].ToString()
	h.config.Loggers.App.p(msg)
	return otto.UndefinedValue()
}

func commit(args ...otto.Value) (res JSNuclearData, err error) {
	entryType, _ := call.args[0].ToString()
	var entry string
	v := call.args[1]

	if v.IsString() {
		entry, _ = v.ToString()
	} else if v.IsObject() {
		v, _ = z.vm.Call("JSON.stringify", nil, v)
		entry, _ = v.ToString()
	} else {
		err = errors.New("HolochainError: commit expected entry to be string or object (second argument)")
		return
	}
	var entryHash Hash
	entryHash, err = h.Commit(entryType, entry)
	if err != nil {
		return
	}

	res, _ = entryHash.String()
	return
}

func get(args ...otto.Value) (res JSNuclearData, err error) {
	var hashstr string

	if v.IsString() {
		hashstr, _ = v.ToString()
	} else {
		err = errors.New("get expected string as argument")
		return
	}

	entry, err := h.Get(hashstr)
	if err == nil {
		result = entry.(*EntryObj)
		return
	}

	if err != nil {
		result = errors.New("HolochainError:" + err.Error())
		return
	}
	panic("Shouldn't get here!")
}

func getlink(args ...otto.Value) (result JSNuclearData, err error) {
	l := len(call.ArgumentList)
	if l < 2 || l > 3 {
		return z.vm.MakeCustomError("HolochainError", "expected 2 or 3 arguments to getlink")
	}
	base, _ := args[0].ToString()
	tag, _ := call.args[1].ToString()
	options := GetLinkOptions{Load: false}
	if l == 3 {
		v := call.args[2]
		if v.IsObject() {
			loadv, _ := v.Object().Get("Load")
			if loadv.IsBoolean() {
				load, _ := loadv.ToBoolean()
				options.Load = load
			}
		} else {
			return z.vm.MakeCustomError("HolochainError", "getlink expected options to be object (third argument)")
		}
	}

	var response interface{}
	response, err = h.GetLink(base, tag, options)
	if err == nil {
		result, err = z.vm.ToValue(response)
	} else {
		return z.vm.MakeCustomError("HolochainError", err.Error())
	}

	return
}

// Run executes javascript code
func (z *JSNucleus) Run(code string) (result JSNuclearData, err error) {
	v, err := z.vm.Run(code)
	if err != nil {
		err = errors.New("JS exec error: " + err.Error())
		return
	}
	z.lastResult = &v
	return
}
