package internal

type KVEntry struct {
	Value string
	// logical clock, higher wins
	Version uint64
}

type KVWriteRequest struct {
	Key   string
	Value string
}

type KVReadResponse struct {
	Key     string
	Value   string
	Version uint64
}
