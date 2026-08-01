package sys

// Discard any IP that may refer to a local network or a special purpose address (e.g. multicast)
func ValidIp4(ip []byte) bool {
	// local host / this network (RFC 1122, RFC 791)
	if ip[0] == 0 || ip[0] == 127 {
		return false
	}

	// RFC1918 private-use
	if ip[0] == 10 || ip[0] == 192 && ip[1] == 168 || ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31 {
		return false
	}

	// RFC3927 link-local
	if ip[0] == 169 && ip[1] == 254 {
		return false
	}

	// RFC6598 shared address space / CGNAT (100.64.0.0/10)
	if ip[0] == 100 && ip[1] >= 64 && ip[1] <= 127 {
		return false
	}

	// RFC6890 IETF protocol assignments (192.0.0.0/24)
	if ip[0] == 192 && ip[1] == 0 && ip[2] == 0 {
		return false
	}

	// RFC5737 documentation (TEST-NET-1/2/3)
	if ip[0] == 192 && ip[1] == 0 && ip[2] == 2 {
		return false
	}
	if ip[0] == 198 && ip[1] == 51 && ip[2] == 100 {
		return false
	}
	if ip[0] == 203 && ip[1] == 0 && ip[2] == 113 {
		return false
	}

	// RFC2544 benchmarking (198.18.0.0/15)
	if ip[0] == 198 && ip[1] >= 18 && ip[1] <= 19 {
		return false
	}

	// RFC3068 6to4 relay anycast (deprecated but still reserved)
	if ip[0] == 192 && ip[1] == 88 && ip[2] == 99 {
		return false
	}

	// RFC5771 multicast (224.0.0.0/4)
	if ip[0] >= 224 && ip[0] <= 239 {
		return false
	}

	// RFC1112/RFC919 reserved for future use + broadcast (240.0.0.0/4)
	if ip[0] >= 240 {
		return false
	}

	return true
}

func IsIPBlocked(ip4 []byte) bool {
	return false
}
