package zstor

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// Checksum represents a checksum array.
type Checksum []byte

// Encryption represents the encryption details.
type Encryption struct {
	Aes string `json:"Aes"`
}

// CI represents the connection information for a shard.
type CI struct {
	Address   string `json:"address"`
	Namespace string `json:"namespace"`
	Password  string `json:"password"`
}

// Key represents a key with its version.
type Key struct {
	V2 int `json:"V2"`
}

// Shard represents a shard's metadata.
type Shard struct {
	Checksum  Checksum `json:"checksum"`
	CI        CI       `json:"ci"`
	Keys      []Key    `json:"keys"`
	ShardIdx  int      `json:"shard_idx"`
}

// Metadata represents the full metadata structure.
type Metadata struct {
	Checksum         Checksum    `json:"checksum"`
	Compression      string      `json:"compression"`
	DataShards       int         `json:"data_shards"`
	DisposableShards int         `json:"disposable_shards"`
	Encryption       Encryption  `json:"encryption"`
	Shards           []Shard     `json:"shards"`
}

// GetMetadata fetches and parses metadata for a given file using zstor-metadata-decoder.
func GetMetadata(configPath, filePath string) (*Metadata, error) {
	cmd := exec.Command("./zstor-metadata-decoder", "--config", configPath, "--file", filePath)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute zstor-metadata-decoder: %w", err)
	}

	var metadata Metadata
	if err := json.Unmarshal(output, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &metadata, nil
}