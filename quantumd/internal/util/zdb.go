package util

import (
	"strconv"
	"strings"
)

func MapIPs(ips []string) map[string]string {
	mapped := make(map[string]string)
	for _, ip := range ips {
		parts := strings.Split(ip, ":")
		if len(parts) == 0 {
			continue
		}
		firstPart := parts[0]
		// Convert the first part to a hex number for range checking
		hexValue, err := strconv.ParseInt(firstPart, 16, 64)
		if err != nil {
			continue
		}
		// Check if it falls into specific ranges
		if 0x2000 <= hexValue && hexValue <= 0x3FFF {
			mapped["ipv6"] = ip
		} else if 0x200 <= hexValue && hexValue <= 0x3FF {
			mapped["ygg"] = ip
		} else if 0x400 <= hexValue && hexValue <= 0x5FF {
			mapped["mycelium"] = ip
		}
	}
	return mapped
}
