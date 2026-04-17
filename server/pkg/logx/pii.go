package logx

const redacted = "[REDACTED]"

func MaskPII(s string) string {
	if s == "" {
		return s
	}
	return redacted
}

func MaskAPNsToken(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8] + "..."
}
