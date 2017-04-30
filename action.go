// Copyright (C) 2013-2017, The MetaCurrency Project (Eric Harris-Braun, Arthur Brock, et. al.)
// Use of this source code is governed by GPLv3 found in the LICENSE file
//----------------------------------------------------------------------------------------
//
package holochain

import (
	"encoding/json"
	"errors"
	"fmt"
	peer "github.com/libp2p/go-libp2p-peer"
	"reflect"
	"time"
)

// Action provides an abstraction for grouping all the aspects of a nucleus function, i.e.
// the validation,dht changing, etc
type Action interface {
	Name() string
	Do(h *Holochain) (response interface{}, err error)
	SysValidation(h *Holochain, d *EntryDef, sources []peer.ID) (err error)
	DHTReqHandler(dht *DHT, msg *Message) (response interface{}, err error)
}

var NonDHTAction error = errors.New("Not a DHT action")
var NonCallableAction error = errors.New("Not a callable action")

func prepareSources(sources []peer.ID) (srcs []string) {
	srcs = make([]string, 0)
	for _, s := range sources {
		srcs = append(srcs, peer.IDB58Encode(s))
	}
	return
}

// ValidateAction runs the different phases of validating an action
func (h *Holochain) ValidateAction(action Action, entryType string, sources []peer.ID) (def *EntryDef, err error) {
	var z *Zome
	z, def, err = h.GetEntryDef(entryType)
	if err != nil {
		return
	}

	// run the action's system level validations
	err = action.SysValidation(h, def, sources)
	if err != nil {
		Debugf("Sys ValidateAction(%T) err:%v\n", action, err)
		return
	}

	// run the action's app level validations
	var n Nucleus
	n, err = h.makeNucleus(z)
	if err != nil {
		return
	}

	err = n.ValidateAction(action, entryType, prepareSources(sources))
	if err != nil {
		Debugf("Nucleus ValidateAction(%T) err:%v\n", action, err)
	}
	return
}

// GetDHTReqAction generates an action from DHT request
// TODO this should be refactored into the Action interface
func (h *Holochain) GetDHTReqAction(msg *Message) (a Action, err error) {
	var t reflect.Type
	// TODO this can be refactored into Action
	switch msg.Type {
	case PUT_REQUEST:
		a = &ActionPut{}
		t = reflect.TypeOf(PutReq{})
	case GET_REQUEST:
		a = &ActionGet{}
		t = reflect.TypeOf(GetReq{})
	case DEL_REQUEST:
		a = &ActionDel{}
		t = reflect.TypeOf(DelReq{})
	case LINK_REQUEST:
		a = &ActionLink{}
		t = reflect.TypeOf(LinkReq{})
	case GETLINK_REQUEST:
		a = &ActionGetLink{}
		t = reflect.TypeOf(LinkQuery{})
	default:
		err = fmt.Errorf("message type %d not in holochain-dht protocol", int(msg.Type))
	}
	if err == nil && reflect.TypeOf(msg.Body) != t {
		err = fmt.Errorf("Unexpected request body type '%T' in %s request", msg.Body, a.Name())
	}
	return
}

//------------------------------------------------------------
// Get

type ActionGet struct {
	hash Hash
}

func NewGetAction(hash Hash) *ActionGet {
	a := ActionGet{hash: hash}
	return &a
}

func (a *ActionGet) Name() string {
	return "get"
}

func (a *ActionGet) Do(h *Holochain) (response interface{}, err error) {
	rsp, err := h.dht.SendGet(a.hash)
	if err != nil {
		return
	}
	var entry Entry
	switch t := rsp.(type) {
	case *GobEntry:
		entry = t
	default:
		err = fmt.Errorf("unexpected response type from SendGet: %T", t)
		return
	}
	response = entry
	return
}

func (a *ActionGet) SysValidation(h *Holochain, d *EntryDef, sources []peer.ID) (err error) {
	return
}

func (a *ActionGet) DHTReqHandler(dht *DHT, msg *Message) (response interface{}, err error) {
	var b []byte
	b, _, _, err = dht.get(msg.Body.(GetReq).H)
	if err == nil {
		var e GobEntry
		err = e.Unmarshal(b)
		if err == nil {
			response = &e
		}
	}
	return
}

//------------------------------------------------------------
// Commit

type ActionCommit struct {
	entryType string
	entry     Entry
	header    *Header
}

func NewCommitAction(entryType string, entry Entry) *ActionCommit {
	a := ActionCommit{entryType: entryType, entry: entry}
	return &a
}

func (a *ActionCommit) Name() string {
	return "commit"
}

func (a *ActionCommit) Do(h *Holochain) (response interface{}, err error) {
	var l int
	var hash Hash
	var header *Header
	l, hash, header, err = h.chain.PrepareHeader(h.hashSpec, time.Now(), a.entryType, a.entry, h.agent.PrivKey())
	if err != nil {
		return
	}
	var d *EntryDef

	a.header = header
	d, err = h.ValidateAction(a, a.entryType, []peer.ID{h.id})
	if err != nil {
		if err == ValidationFailedErr {
			err = fmt.Errorf("Invalid entry: %v", a.entry.Content())
		}
		return
	}
	err = h.chain.addEntry(l, hash, header, a.entry)
	if err != nil {
		return
	}
	entryHash := header.EntryLink

	if d.DataFormat == DataFormatLinks {
		// if this is a Link entry we have to send the DHT Link message
		var le LinksEntry
		entryStr := a.entry.Content().(string)
		err = json.Unmarshal([]byte(entryStr), &le)
		if err != nil {
			return
		}

		bases := make(map[string]bool)
		for _, l := range le.Links {
			_, exists := bases[l.Base]
			if !exists {
				b, _ := NewHash(l.Base)
				h.dht.SendLink(LinkReq{Base: b, Links: entryHash})
				bases[l.Base] = true
			}
		}
	} else if d.Sharing == Public {
		// otherwise we check to see if it's a public entry and if so send the DHT put message
		err = h.dht.SendPut(entryHash)
	}
	response = entryHash
	return
}

// sysValidateEntry does system level validation for an entry
// It checks that entry is not nil, and that it conforms to the entry schema in the definition
// and if it's a Links entry that the contents are correctly structured
func sysValidateEntry(h *Holochain, d *EntryDef, entry Entry) (err error) {
	if entry == nil {
		err = errors.New("nil entry invalid")
		return
	}
	// see if there is a schema validator for the entry type and validate it if so
	if d.validator != nil {
		var input interface{}
		if d.DataFormat == DataFormatJSON {
			if err = json.Unmarshal([]byte(entry.Content().(string)), &input); err != nil {
				return
			}
		} else {
			input = entry
		}
		Debugf("Validating %v against schema", input)
		if err = d.validator.Validate(input); err != nil {
			return
		}
	} else if d.DataFormat == DataFormatLinks {
		// Perform base validation on links entries, i.e. that all items exist and are of the right types
		// so first unmarshall the json, and then check that the hashes are real.
		var l struct{ Links []map[string]string }
		err = json.Unmarshal([]byte(entry.Content().(string)), &l)
		if err != nil {
			err = fmt.Errorf("invalid links entry, invalid json: %v", err)
			return
		}
		if len(l.Links) == 0 {
			err = errors.New("invalid links entry: you must specify at least one link")
			return
		}
		for _, link := range l.Links {
			h, ok := link["Base"]
			if !ok {
				err = errors.New("invalid links entry: missing Base")
				return
			}
			if _, err = NewHash(h); err != nil {
				err = fmt.Errorf("invalid links entry: Base %v", err)
				return
			}
			h, ok = link["Link"]
			if !ok {
				err = errors.New("invalid links entry: missing Link")
				return
			}
			if _, err = NewHash(h); err != nil {
				err = fmt.Errorf("invalid links entry: Link %v", err)
				return
			}
			_, ok = link["Tag"]
			if !ok {
				err = errors.New("invalid links entry: missing Tag")
				return
			}
		}

	}
	return
}

func (a *ActionCommit) SysValidation(h *Holochain, d *EntryDef, sources []peer.ID) (err error) {
	err = sysValidateEntry(h, d, a.entry)
	return
}

func (a *ActionCommit) DHTReqHandler(dht *DHT, msg *Message) (response interface{}, err error) {
	err = NonDHTAction
	return
}

//------------------------------------------------------------
// Put

type ActionPut struct {
	entryType string
	entry     Entry
	header    *Header
}

func NewPutAction(entryType string, entry Entry, header *Header) *ActionPut {
	a := ActionPut{entryType: entryType, entry: entry, header: header}
	return &a
}

func (a *ActionPut) Name() string {
	return "put"
}

func (a *ActionPut) Do(h *Holochain) (response interface{}, err error) {
	err = NonCallableAction
	return
}

func (a *ActionPut) SysValidation(h *Holochain, d *EntryDef, sources []peer.ID) (err error) {
	err = sysValidateEntry(h, d, a.entry)
	return
}

func (a *ActionPut) DHTReqHandler(dht *DHT, msg *Message) (response interface{}, err error) {
	//dht.puts <- *m  TODO add back in queueing
	err = dht.handleChangeReq(msg)
	response = "queued"
	return
}

//------------------------------------------------------------
// Del

type ActionDel struct {
	entryType string
	hash      Hash
}

func NewDelAction(entryType string, hash Hash) *ActionDel {
	a := ActionDel{entryType: entryType, hash: hash}
	return &a
}

func (a *ActionDel) Name() string {
	return "del"
}

func (a *ActionDel) Do(h *Holochain) (response interface{}, err error) {
	err = NonCallableAction
	return
}

func (a *ActionDel) SysValidation(h *Holochain, d *EntryDef, sources []peer.ID) (err error) {
	return
}

func (a *ActionDel) DHTReqHandler(dht *DHT, msg *Message) (response interface{}, err error) {
	//dht.puts <- *m  TODO add back in queueing
	err = dht.handleChangeReq(msg)
	response = "queued"
	return
}

//------------------------------------------------------------
// Link

type ActionLink struct {
	entryType      string
	links          []Link
	validationBase Hash
}

func NewLinkAction(entryType string, links []Link) *ActionLink {
	a := ActionLink{entryType: entryType, links: links}
	return &a
}

func (a *ActionLink) Name() string {
	return "link"
}

func (a *ActionLink) Do(h *Holochain) (response interface{}, err error) {
	err = NonCallableAction
	return
}

func (a *ActionLink) SysValidation(h *Holochain, d *EntryDef, sources []peer.ID) (err error) {
	//@TODO what sys level links validation?  That they are all valid hash format for the DNA?
	return
}

func (a *ActionLink) DHTReqHandler(dht *DHT, msg *Message) (response interface{}, err error) {
	base := msg.Body.(LinkReq).Base
	err = dht.exists(base)
	if err == nil {
		//h.dht.puts <- *m  TODO add back in queueing
		err = dht.handleChangeReq(msg)

		response = "queued"
	} else {
		dht.dlog.Logf("DHTReceiver key %v doesn't exist, ignoring", base)
	}
	return
}

//------------------------------------------------------------
// GetLink

type ActionGetLink struct {
	linkQuery *LinkQuery
	options   *GetLinkOptions
}

func NewGetLinkAction(linkQuery *LinkQuery, options *GetLinkOptions) *ActionGetLink {
	a := ActionGetLink{linkQuery: linkQuery, options: options}
	return &a
}

func (a *ActionGetLink) Name() string {
	return "getlink"
}

func (a *ActionGetLink) Do(h *Holochain) (response interface{}, err error) {
	var r interface{}
	r, err = h.dht.SendGetLink(*a.linkQuery)
	if err == nil {
		switch t := r.(type) {
		case *LinkQueryResp:
			response = t
			if a.options.Load {
				for i := range t.Links {
					var hash Hash
					hash, err = NewHash(t.Links[i].H)
					if err != nil {
						return
					}
					entry, err := NewGetAction(hash).Do(h)
					if err == nil {
						t.Links[i].E = entry.(*GobEntry).C.(string)
					}
					//TODO better error handling here, i.e break out of the loop and return if error?
				}
			}
		default:
			err = fmt.Errorf("unexpected response type from SendGetLink: %T", t)
		}
	}
	return
}

func (a *ActionGetLink) SysValidation(h *Holochain, d *EntryDef, sources []peer.ID) (err error) {
	//@TODO what sys level getlinks validation?  That they are all valid hash format for the DNA?
	return
}

func (a *ActionGetLink) DHTReqHandler(dht *DHT, msg *Message) (response interface{}, err error) {
	lq := msg.Body.(LinkQuery)
	var r LinkQueryResp
	r.Links, err = dht.getLink(lq.Base, lq.T)
	response = &r

	return
}
