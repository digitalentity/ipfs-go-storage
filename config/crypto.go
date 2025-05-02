package config

import (
	"encoding/json"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// Wrapper for crypto.PrivKey to support JSON Marshall and Unmarshall transparently

type PrivKey struct {
	crypto.PrivKey
	json.Marshaler
	json.Unmarshaler
}

func (c *PrivKey) MarshalJSON() ([]byte, error) {
	if c.PrivKey == nil {
		return json.Marshal(nil)
	}

	b, err := crypto.MarshalPrivateKey(c.PrivKey)
	if err != nil {
		return nil, err
	}
	return json.Marshal(b)
}

func (c *PrivKey) UnmarshalJSON(data []byte) error {
	var b []byte
	if err := json.Unmarshal(data, &b); err != nil {
		return err
	}

	// Valid case: no key defined
	if len(b) == 0 {
		c.PrivKey = nil
		return nil
	}

	priv, err := crypto.UnmarshalPrivateKey(b)
	if err != nil {
		return err
	}

	c.PrivKey = priv
	return nil
}

func (c *PrivKey) Valid() bool {
	return c.PrivKey != nil
}

type PubKey struct {
	crypto.PubKey
	json.Marshaler
	json.Unmarshaler
}

func (c *PubKey) MarshalJSON() ([]byte, error) {
	if c.PubKey == nil {
		return json.Marshal(nil)
	}

	b, err := crypto.MarshalPublicKey(c.PubKey)
	if err != nil {
		return nil, err
	}
	return json.Marshal(b)
}

func (c *PubKey) UnmarshalJSON(data []byte) error {
	var b []byte
	if err := json.Unmarshal(data, &b); err != nil {
		return err
	}

	// Valid case: no key defined
	if len(b) == 0 {
		c.PubKey = nil
		return nil
	}

	pub, err := crypto.UnmarshalPublicKey(b)
	if err != nil {
		return err
	}

	c.PubKey = pub
	return nil
}

func (c *PubKey) Valid() bool {
	return c.PubKey != nil
}
