package zstor

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
)

// Checksum represents a checksum array.
type Checksum []byte

// UnmarshalJSON implements json.Unmarshaler interface for Checksum.
// It expects a hex string and converts it to a byte slice.
func (c *Checksum) UnmarshalJSON(data []byte) error {
	// Remove quotes from the JSON string
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		data = data[1 : len(data)-1]
	}

	// Convert hex string to bytes
	bytes, err := hex.DecodeString(string(data))
	if err != nil {
		return err
	}

	*c = bytes
	return nil
}

// MarshalJSON implements json.Marshaler interface for Checksum.
// It converts the byte slice to a hex string.
func (c Checksum) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(c))
}

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
	Checksum Checksum `json:"checksum"`
	CI       CI       `json:"ci"`
	Keys     []Key    `json:"keys"`
	ShardIdx int      `json:"shard_idx"`
}

// Metadata represents the full metadata structure.
type Metadata struct {
	Checksum         Checksum   `json:"checksum"`
	Compression      string     `json:"compression"`
	DataShards       int        `json:"data_shards"`
	DisposableShards int        `json:"disposable_shards"`
	Encryption       Encryption `json:"encryption"`
	Shards           []Shard    `json:"shards"`
}

// GetMetadata fetches and parses metadata for a given file using zstor-metadata-decoder.
func GetMetadata(configPath, filePath string) (*Metadata, error) {
	cmd := exec.Command("/usr/local/bin/zstor-metadata-decoder", "--config", configPath, "--file", filePath)
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

// GetAllMetadata fetches and parses metadata for all files using zstor-metadata-decoder.
func GetAllMetadata(configPath string) (map[string]Metadata, error) {
	cmd := exec.Command("/usr/local/bin/zstor-metadata-decoder", "--config", configPath, "--all")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute zstor-metadata-decoder: %w", err)
	}

	var allMetadata map[string]Metadata
	if err := json.Unmarshal(output, &allMetadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal all metadata: %w", err)
	}

	return allMetadata, nil
}
