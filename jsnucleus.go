// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------
// JSNucleus implements a javascript use of the Nucleus interface

package holochain

import (
	"encoding/json"
	"errors"
	"fmt"
	peer "github.com/libp2p/go-libp2p-peer"
	"github.com/robertkrimen/otto"
	"strings"
	"time"
)

const (
	JSNucleusType = "js"
)

type JSNucleus struct {
	vm         *otto.Otto
	lastResult *otto.Value
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

func prepareJSEntryArgs(entryType string, entry Entry, header *Header) (args string, err error) {
	def := h.GetEntryDef(entryType)
	entryStr := entry.Content().(string)
	switch def.DataFormat {
	case DataFormatRawJS:
		args = entryStr
	case DataFormatString:
		args = "\"" + jsSanitizeString(entryStr) + "\""
	case DataFormatLinks:
		fallthrough
	case DataFormatJSON:
		args = fmt.Sprintf(`JSON.parse("%s")`, jsSanitizeString(entryStr))
	default:
		err = errors.New("data format not implemented: " + def.DataFormat)
		return
	}

	hdr := fmt.Sprintf(
		`{"EntryLink":"%s","Type":"%s","Time":"%s"}`,
		header.EntryLink.String(),
		header.Type,
		header.Time.UTC().Format(time.RFC3339),
	)
	args += "," + hdr
	return
}

func prepareJSValidateArgs(action Action, entryType string) (args string, err error) {
	switch t := action.(type) {
	def := h.GetEntryDef(entryType)
	case *ActionPut:
		args, err = prepareJSEntryArgs(def, t.entry, t.header)
	case *ActionCommit:
		args, err = prepareJSEntryArgs(def, t.entry, t.header)
	case *ActionDel:
		args = fmt.Sprintf(`"%s"`, t.hash.String())
	case *ActionLink:
		var j []byte
		j, err = json.Marshal(t.links)
		if err == nil {
			args = fmt.Sprintf(`"%s",JSON.parse("%s")`, t.validationBase.String(), jsSanitizeString(string(j)))
		}

	default:
		err = fmt.Errorf("can't prepare args for %T: ", t)
		return
	}
	return
}

func buildJSValidateAction(action Action, entryType string, sources []string) (code string, err error) {
	fnName := "validate" + strings.Title(action.Name())
	var args string
	args, err = prepareJSValidateArgs(action, entryType)
	if err != nil {
		return
	}
	srcs := mkJSSources(sources)
	code = fmt.Sprintf(`%s("%s",%s,%s)`, fnName, entryType, args, srcs)

	return
}

// ValidateAction builds the correct validation function based on the action an calls it
func (z *JSNucleus) ValidateAction(action Action, entryType string, sources []string) (err error) {
	var code string
	code, err = buildJSValidateAction(action, entryType, sources)
	if err != nil {
		return
	}
	Debug(code)
	err = z.runValidate(action.Name(), code)
	return
}

func mkJSSources(sources []string) (srcs string) {
	srcs = `["` + strings.Join(sources, `","`) + `"]`
	return
}

func (z *JSNucleus) prepareJSValidateEntryArgs(def *EntryDef, entry Entry, sources []string) (e string, srcs string, err error) {
	c := entry.Content().(string)
	switch def.DataFormat {
	case DataFormatRawJS:
		e = c
	case DataFormatString:
		e = "\"" + jsSanitizeString(c) + "\""
	case DataFormatLinks:
		fallthrough
	case DataFormatJSON:
		e = fmt.Sprintf(`JSON.parse("%s")`, jsSanitizeString(c))
	default:
		err = errors.New("data format not implemented: " + def.DataFormat)
		return
	}
	srcs = mkJSSources(sources)
	return
}

func (z *JSNucleus) runValidate(fnName string, code string) (err error) {
	var v otto.Value
	v, err = z.vm.Run(code)
	if err != nil {
		err = fmt.Errorf("Error executing %s: %v", fnName, err)
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

	e, srcs, err := z.prepareJSValidateEntryArgs(entryType, entry, sources)
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
func (z *JSNucleus) Call(fn *FunctionDef, params interface{}) (result interface{}, err error) {
	var code string
	switch fn.CallingType {
	case STRING_CALLING:
		code = fmt.Sprintf(`%s("%s");`, fn.Name, jsSanitizeString(params.(string)))
	case JSON_CALLING:
		if params.(string) == "" {
			code = fmt.Sprintf(`JSON.stringify(%s());`, fn.Name)
		} else {
			p := jsSanitizeString(params.(string))
			code = fmt.Sprintf(`JSON.stringify(%s(JSON.parse("%s")));`, fn.Name, p)
		}
	default:
		err = errors.New("params type not implemented")
		return
	}
	Debugf("JS Call: %s", code)
	var v otto.Value
	v, err = z.vm.Run(code)
	if err == nil {
		if v.IsObject() && v.Class() == "Error" {
			Debugf("JS Error:\n%v", v)
			var message otto.Value
			message, err = v.Object().Get("message")
			if err == nil {
				err = errors.New(message.String())
			}
		} else {
			result, err = v.ToString()
		}
	}
	return
}

// NewJSNucleus builds a javascript execution environment with user specified code
func NewJSNucleus(h *Holochain, code string) (n Nucleus, err error) {
	var z JSNucleus
	z.vm = otto.New()

	err = z.vm.Set("property", func(call otto.FunctionCall) otto.Value {
		prop, _ := call.Argument(0).ToString()

		p, err := h.GetProperty(prop)
		if err != nil {
			return otto.UndefinedValue()
		}
		result, _ := z.vm.ToValue(p)
		return result
	})
	if err != nil {
		return nil, err
	}

	err = z.vm.Set("debug", func(call otto.FunctionCall) otto.Value {
		msg, _ := call.Argument(0).ToString()
		h.config.Loggers.App.p(msg)
		return otto.UndefinedValue()
	})

	err = z.vm.Set("commit", func(call otto.FunctionCall) otto.Value {
		entryType, _ := call.Argument(0).ToString()
		var entry string
		v := call.Argument(1)

		if v.IsString() {
			entry, _ = v.ToString()
		} else if v.IsObject() {
			v, _ = z.vm.Call("JSON.stringify", nil, v)
			entry, _ = v.ToString()
		} else {
			return z.vm.MakeCustomError("HolochainError", "commit expected entry to be string or object (second argument)")
		}

		var r interface{}
		e := GobEntry{C: entry}
		r, err = NewCommitAction(entryType, &e).Do(h)
		if err != nil {
			return z.vm.MakeCustomError("HolochainError", err.Error())
		}
		var entryHash Hash
		if r != nil {
			entryHash = r.(Hash)
		}

		result, _ := z.vm.ToValue(entryHash.String())
		return result
	})
	if err != nil {
		return nil, err
	}

	err = z.vm.Set("get", func(call otto.FunctionCall) (result otto.Value) {
		v := call.Argument(0)
		var hashstr string

		if v.IsString() {
			hashstr, _ = v.ToString()
		} else {
			return z.vm.MakeCustomError("HolochainError", "get expected string as argument")
		}

		var hash Hash
		hash, err = NewHash(hashstr)
		if err != nil {
			return
		}

		entry, err := NewGetAction(hash).Do(h)
		if err == nil {
			t := entry.(*GobEntry)
			result, err = z.vm.ToValue(t)
			return
		}

		if err != nil {
			result = z.vm.MakeCustomError("HolochainError", err.Error())
			return
		}
		panic("Shouldn't get here!")
	})
	if err != nil {
		return nil, err
	}

	err = z.vm.Set("getlink", func(call otto.FunctionCall) (result otto.Value) {
		l := len(call.ArgumentList)
		if l < 2 || l > 3 {
			return z.vm.MakeCustomError("HolochainError", "expected 2 or 3 arguments to getlink")
		}
		basestr, _ := call.Argument(0).ToString()
		tag, _ := call.Argument(1).ToString()
		options := GetLinkOptions{Load: false}
		if l == 3 {
			v := call.Argument(2)
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

		var base Hash
		base, err = NewHash(basestr)
		if err != nil {
			return
		}

		response, err = NewGetLinkAction(&LinkQuery{Base: base, T: tag}, &options).Do(h)

		if err == nil {
			result, err = z.vm.ToValue(response)
		} else {
			return z.vm.MakeCustomError("HolochainError", err.Error())
		}

		return
	})
	if err != nil {
		return nil, err
	}
	l := JSLibrary
	if h != nil {
		l += fmt.Sprintf(`var App = {Name:"%s",DNA:{Hash:"%s"},Agent:{Hash:"%s",String:"%s"},Key:{Hash:"%s"}};`, h.Name, h.dnaHash, h.agentHash, h.Agent().Name(), peer.IDB58Encode(h.id))
	}
	_, err = z.Run(l + code)
	if err != nil {
		return
	}
	n = &z
	return
}

// Run executes javascript code
func (z *JSNucleus) Run(code string) (result *otto.Value, err error) {
	v, err := z.vm.Run(code)
	if err != nil {
		err = errors.New("JS exec error: " + err.Error())
		return
	}
	z.lastResult = &v
	return
}
