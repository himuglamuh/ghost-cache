package node

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

const (
	RadixChildren = 16
	LeafThreshold = 10
)

type ChildDigest struct {
	Count  uint16
	Digest uint64
}
type RadixSummary struct {
	Generation uint32
	Prefix     []byte
	Children   [RadixChildren]ChildDigest
}
type RadixLeaf struct {
	Generation uint32
	Prefix     []byte
	IDs        []publication.ID
}

func InventoryGeneration(ids []publication.ID) uint32 {
	sorted := sortedIDs(ids)
	hash := sha256.New()
	for _, id := range sorted {
		hash.Write(id[:])
	}
	sum := hash.Sum(nil)
	return binary.LittleEndian.Uint32(sum[:4])
}

func BuildSummary(ids []publication.ID, prefix []byte) RadixSummary {
	summary := RadixSummary{Generation: InventoryGeneration(ids), Prefix: append([]byte(nil), prefix...)}
	if len(prefix) >= 32 {
		return summary
	}
	for child := 0; child < RadixChildren; child++ {
		childPrefix := append(append([]byte(nil), prefix...), byte(child))
		bucket := FilterPrefix(ids, childPrefix)
		count := len(bucket)
		if count > 65535 {
			count = 65535
		}
		summary.Children[child].Count = uint16(count)
		summary.Children[child].Digest = digestIDs(bucket)
	}
	return summary
}

func BuildLeaf(ids []publication.ID, prefix []byte) RadixLeaf {
	return RadixLeaf{Generation: InventoryGeneration(ids), Prefix: append([]byte(nil), prefix...), IDs: FilterPrefix(ids, prefix)}
}

func FilterPrefix(ids []publication.ID, prefix []byte) []publication.ID {
	var out []publication.ID
	for _, id := range ids {
		match := true
		for depth, nibble := range prefix {
			if nibbleAt(id, depth) != nibble {
				match = false
				break
			}
		}
		if match {
			out = append(out, id)
		}
	}
	return sortedIDs(out)
}

func DifferingChildren(local RadixSummary, remote RadixSummary) [][]byte {
	if string(local.Prefix) != string(remote.Prefix) {
		return nil
	}
	var out [][]byte
	for child := 0; child < RadixChildren; child++ {
		if local.Children[child] != remote.Children[child] {
			out = append(out, append(append([]byte(nil), remote.Prefix...), byte(child)))
		}
	}
	return out
}

func MissingIDs(local []publication.ID, remote RadixLeaf) []publication.ID {
	have := make(map[publication.ID]struct{}, len(local))
	for _, id := range local {
		have[id] = struct{}{}
	}
	var missing []publication.ID
	for _, id := range remote.IDs {
		if _, ok := have[id]; !ok {
			missing = append(missing, id)
		}
	}
	return missing
}

func nibbleAt(id publication.ID, depth int) byte {
	b := id[depth/2]
	if depth%2 == 0 {
		return b >> 4
	}
	return b & 0x0f
}
func sortedIDs(ids []publication.ID) []publication.ID {
	out := append([]publication.ID(nil), ids...)
	sort.Slice(out, func(i, j int) bool { return string(out[i][:]) < string(out[j][:]) })
	return out
}
func digestIDs(ids []publication.ID) uint64 {
	hash := sha256.New()
	for _, id := range sortedIDs(ids) {
		hash.Write(id[:])
	}
	sum := hash.Sum(nil)
	return binary.LittleEndian.Uint64(sum[:8])
}
