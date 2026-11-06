package series

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// paramsFields is Params without its JSON methods.
type paramsFields Params

// MarshalJSON writes a fixed series as it always has. A variable series
// names its hash under "variable_hash" and leaves "hash" out, so a poon
// from before D163, which ignores the keys it does not know, finds no hash
// and refuses the series (hash 0 is no hash) instead of reading it as a
// fixed series with another id (RED-071, D165).
func (p Params) MarshalJSON() ([]byte, error) {
	if p.SupplyType != Variable {
		return json.Marshal(paramsFields(p))
	}
	return json.Marshal(struct {
		VariableHash hash.ID  `json:"variable_hash"`
		Hash         *hash.ID `json:"hash,omitempty"` // left out: shadows the field below
		paramsFields
	}{VariableHash: p.HashID, paramsFields: paramsFields(p)})
}

// ErrUnknownKey is returned for series JSON with a key this poon does not
// know: a series made by a later poon, which must be unreadable here
// rather than read as another series (RED-071).
var ErrUnknownKey = errors.New("series: a key this poon does not know (a series from a later version?)")

// UnmarshalJSON reads both forms MarshalJSON writes, and a variable series
// written with "hash" (D163's first form). A key it does not know is an
// error, so a later extension is unreadable here, never misread.
func (p *Params) UnmarshalJSON(b []byte) error {
	var in struct {
		VariableHash *hash.ID `json:"variable_hash"`
		paramsFields
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		if bytes.HasPrefix([]byte(err.Error()), []byte("json: unknown field")) {
			return fmt.Errorf("%w: %v", ErrUnknownKey, err)
		}
		return err
	}
	if in.VariableHash != nil {
		if in.SupplyType != Variable {
			return fmt.Errorf("%w: variable_hash names the hash of a variable series only", ErrInvalid)
		}
		if in.HashID != 0 && in.HashID != *in.VariableHash {
			return fmt.Errorf("%w: hash and variable_hash differ", ErrInvalid)
		}
		in.HashID = *in.VariableHash
	}
	*p = Params(in.paramsFields)
	return nil
}
