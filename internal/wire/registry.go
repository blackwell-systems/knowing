package wire

import (
	"fmt"
	"sort"
	"sync"
)

// Encoder serializes a Payload into a wire format string.
type Encoder func(p *Payload) (string, error)

// Decoder parses a wire format string back into a Payload.
type Decoder func(input string) (*Payload, error)

// Codec is a registered encoding scheme with encode/decode functions and metadata.
type Codec struct {
	Name        string
	Description string
	Encode      Encoder
	Decode      Decoder
}

var (
	mu     sync.RWMutex
	codecs = make(map[string]*Codec)
)

func init() {
	// Register built-in codecs.
	Register(&Codec{
		Name:        "kwf",
		Description: "Knowing Wire Format: graph-native, 75%+ token savings, text-only",
		Encode:      encodeKWF,
		Decode:      decodeKWF,
	})
	Register(&Codec{
		Name:        "json",
		Description: "Standard JSON: maximum compatibility, verbose",
		Encode:      encodeJSON,
		Decode:      decodeJSON,
	})
}

// Register adds a codec to the registry. Panics on duplicate name.
func Register(c *Codec) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := codecs[c.Name]; exists {
		panic(fmt.Sprintf("wire: codec %q already registered", c.Name))
	}
	codecs[c.Name] = c
}

// Get returns the codec for the given name, or an error if not found.
func Get(name string) (*Codec, error) {
	mu.RLock()
	defer mu.RUnlock()
	c, ok := codecs[name]
	if !ok {
		return nil, fmt.Errorf("wire: unknown codec %q (available: %s)", name, ListNames())
	}
	return c, nil
}

// ListNames returns all registered codec names in sorted order.
func ListNames() string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(codecs))
	for name := range codecs {
		names = append(names, name)
	}
	sort.Strings(names)
	result := ""
	for i, n := range names {
		if i > 0 {
			result += ", "
		}
		result += n
	}
	return result
}

// List returns all registered codecs.
func List() []*Codec {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]*Codec, 0, len(codecs))
	for _, c := range codecs {
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// EncodeWith encodes a payload using the named codec.
func EncodeWith(name string, p *Payload) (string, error) {
	c, err := Get(name)
	if err != nil {
		return "", err
	}
	return c.Encode(p)
}

// DecodeWith decodes input using the named codec.
func DecodeWith(name string, input string) (*Payload, error) {
	c, err := Get(name)
	if err != nil {
		return nil, err
	}
	return c.Decode(input)
}

// Adapter functions that wrap Encode/Decode to match the Encoder/Decoder signatures.

func encodeKWF(p *Payload) (string, error) {
	return Encode(p), nil
}

func decodeKWF(input string) (*Payload, error) {
	return Decode(input)
}
