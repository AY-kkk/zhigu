package finance

import (
	"net"
	"net/url"
	"strings"
)

func ValidateUpstreamURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return NewError(400, "validation", "INVALID_URL", "无效的上游地址")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return NewError(400, "validation", "INVALID_URL", "仅允许 http/https")
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return NewError(400, "validation", "SSRF_BLOCKED", "禁止环回或内网地址")
	}
	if ip := net.ParseIP(host); ip != nil {
		if blockedIP(ip) {
			return NewError(400, "validation", "SSRF_BLOCKED", "禁止环回或内网地址")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return NewError(400, "validation", "SSRF_BLOCKED", "无法解析上游主机")
	}
	for _, ip := range ips {
		if blockedIP(ip) {
			return NewError(400, "validation", "SSRF_BLOCKED", "禁止解析到内网地址")
		}
	}
	return nil
}

func blockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.To4() != nil {
		v4 := ip.To4()
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return false
}

func InternalServiceAllowed(raw, allow string) bool {
	if allow == "" {
		return false
	}
	return strings.TrimRight(raw, "/") == strings.TrimRight(allow, "/") || strings.HasPrefix(raw, strings.TrimRight(allow, "/")+"/")
}
