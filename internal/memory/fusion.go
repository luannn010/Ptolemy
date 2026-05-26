package memory

type Fusion interface {
	Fuse(lists [][]RetrievedChunk, k int) []RetrievedChunk
}

type PassthroughFusion struct{}

func (PassthroughFusion) Fuse(lists [][]RetrievedChunk, k int) []RetrievedChunk {
	if len(lists) == 0 {
		return nil
	}
	first := lists[0]
	if k <= 0 || k >= len(first) {
		return first
	}
	return first[:k]
}
