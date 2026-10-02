package main

import "testing"

func TestExternalURLWhitelist(t *testing.T) {
	for _, raw := range []string{"https://example.com/path?q=中文", "http://localhost:8080/", "mailto:test@example.com?subject=hello", "tel:+8613800000000", "HTTPS://example.com"} {
		if err := validateExternalURL(raw); err != nil {
			t.Errorf("应允许 %q：%v", raw, err)
		}
	}
	for _, raw := range []string{"", "file:///etc/passwd", "javascript:alert(1)", "data:text/html,test", "leafmark://open", "//example.com", "https:", "https:example.com", "http://", "https://user:password@example.com", "mailto:", "tel:", "tel://example.com", "https://example.com\n", "https://example.com/a b", "https://example.com/%zz"} {
		if err := validateExternalURL(raw); err == nil {
			t.Errorf("应拒绝 %q", raw)
		}
	}
}
