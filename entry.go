//
// serialize/deserialize entries, etc.
// gob and json
//
package holochain

// GobEncode encodes anything using gob
func GobEncode(data interface{}) (output_bytes []byte, err error) {
	var gob_buffer bytes.Buffer
	enc := gob.NewEncoder(&gob_buffer)
	err = enc.Encode(data)
	if err != nil {
		return
	}
	output_bytes = gob_buffer.Bytes()
	return
}

// GobDecode decodes data encoded by GobEncode
func GobDecode(output_bytes []byte, to interface{}) (err error) {
	gob_buffer := bytes.NewBuffer(output_bytes)
	dec := gob.NewDecode(gob_buffer)
	err = dec.Decode(to)
	return
}

// implementation of Entry interface with gobs
func (gob_entry *GobEntry) Marshal() (output_bytes []byte, err error) {
	output_bytes, err = GobEncode(&gob_entry.C)
	return
}

func (gob_entry *GobEntry) Unmarshal(input_bytes []byte) (err error) {
	err = GobDecode(input_bytes, &gob_entry.C)
	return
}

func (gob_entry *GobEntry) Content() interface{} { return gob_entry.C }

// implementation of Entry interface with JSON
func (json_entry *JSONEntry) Marshal() (output_bytes []byte, err error) {
	json_string, err := json.Marshal(json_entry.C)
	if err != nil {
		return
	}
	output_bytes = []byte(json_string)
	return
}
func (json_entry *JSONEntry) Unmarshal(input_bytes []byte) (err error) {
	err = json.Unmarshal(input_bytes, &json_entry.C)
	return
}
func (json_entry *JSONEntry) Content() interface{} { return json_entry.C }
