package vecid

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
)

// Hasher projects embedding vectors into compact locality-sensitive hashes.
type Hasher struct {
	dim    int
	bits   int
	planes [][]float32
}

// PlanesFile is the JSON shape used to persist a hasher's hyperplanes.
type PlanesFile struct {
	Dim    int         `json:"dim"`
	Bits   int         `json:"bits"`
	Seed   uint64      `json:"seed"`
	Planes [][]float32 `json:"planes"`
}

// NewHasher creates a random-hyperplane LSH hasher.
func NewHasher(dim, bits int, seed uint64) *Hasher {
	if bits <= 0 || bits%4 != 0 {
		panic("vecid: bits must be a positive multiple of 4")
	}
	if dim <= 0 {
		panic("vecid: dim must be positive")
	}

	rng := rand.New(rand.NewPCG(seed, seed^0xdeadbeef))
	planes := make([][]float32, bits)
	for i := range planes {
		plane := make([]float32, dim)
		var norm float64
		for j := range plane {
			v := float32(rng.NormFloat64())
			plane[j] = v
			norm += float64(v) * float64(v)
		}
		norm = math.Sqrt(norm)
		if norm > 0 {
			scale := float32(1.0 / norm)
			for j := range plane {
				plane[j] *= scale
			}
		}
		planes[i] = plane
	}
	return &Hasher{dim: dim, bits: bits, planes: planes}
}

// NewHasherFromPlanes creates a deterministic hasher from precomputed planes.
func NewHasherFromPlanes(planes [][]float32) *Hasher {
	bits := len(planes)
	if bits == 0 || bits%4 != 0 {
		panic("vecid: planes count must be a positive multiple of 4")
	}
	dim := len(planes[0])
	if dim <= 0 {
		panic("vecid: plane dimension must be positive")
	}
	for i, plane := range planes {
		if len(plane) != dim {
			panic(fmt.Sprintf("vecid: plane %d has dimension %d, expected %d", i, len(plane), dim))
		}
	}
	return &Hasher{dim: dim, bits: bits, planes: planes}
}

// NewHasherFromJSON loads a persisted planes file.
func NewHasherFromJSON(data []byte) (*Hasher, error) {
	var pf PlanesFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return nil, fmt.Errorf("vecid: parse planes JSON: %w", err)
	}
	if len(pf.Planes) == 0 {
		return nil, fmt.Errorf("vecid: empty planes in JSON")
	}
	return NewHasherFromPlanes(pf.Planes), nil
}

// Planes returns the internal hyperplane matrix.
func (h *Hasher) Planes() [][]float32 {
	return h.planes
}

// Hash computes the uppercase hex hash for one embedding.
func (h *Hasher) Hash(embedding []float32) string {
	if len(embedding) != h.dim {
		panic("vecid: embedding dimension mismatch")
	}

	nBytes := h.bits / 8
	if h.bits%8 != 0 {
		nBytes++
	}
	hashBytes := make([]byte, nBytes)

	for i, plane := range h.planes {
		if dot32(plane, embedding) > 0 {
			hashBytes[i/8] |= 1 << (7 - uint(i%8))
		}
	}

	full := hex.EncodeToString(hashBytes)
	nNibbles := h.bits / 4
	result := make([]byte, nNibbles)
	for i := range nNibbles {
		c := full[i]
		if c >= 'a' && c <= 'f' {
			c -= 'a' - 'A'
		}
		result[i] = c
	}
	return string(result)
}

// Bits returns the hash size in bits.
func (h *Hasher) Bits() int {
	return h.bits
}

// Dim returns the expected embedding dimension.
func (h *Hasher) Dim() int {
	return h.dim
}

func dot32(a, b []float32) float32 {
	var sum float32
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}
