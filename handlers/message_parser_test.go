package handlers

import (
	"strings"
	"testing"

	"github.com/tencent-connect/botgo/errs"
)

// TestSanitizeErrorMessage tests the sanitizeErrorMessage function
func TestSanitizeErrorMessage(t *testing.T) {
	tests := []struct {
		name             string
		err              error
		expected         string
		shouldNotContain []string
	}{
		{
			name:     "nil error should return empty string",
			err:      nil,
			expected: "",
		},
		{
			name:             "QQ API error should return sanitized message without sensitive info",
			err:              errs.New(400, `{"message":"富媒体文件下载失败","code":850026,"err_code":40034001,"trace_id":"test","url":"http://192.168.1.1/image.jpg"}`, "test-trace"),
			expected:         "code:850026, message:富媒体文件下载失败, err_code:40034001",
			shouldNotContain: []string{"192.168.1.1", "http://", "image.jpg", "trace_id"},
		},
		{
			name:             "non-JSON error should return generic message",
			err:              errs.New(500, "Internal Server Error from http://10.0.0.1:8080", "test-trace"),
			expected:         "code:500, 请求失败",
			shouldNotContain: []string{"10.0.0.1", "8080", "http://"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeErrorMessage(tt.err)
			if result != tt.expected {
				t.Errorf("sanitizeErrorMessage(%v) = %v, want %v", tt.err, result, tt.expected)
			}
			// Check that sensitive info is not included
			for _, sensitive := range tt.shouldNotContain {
				if strings.Contains(result, sensitive) {
					t.Errorf("sanitizeErrorMessage result should not contain sensitive info: %s, but got: %s", sensitive, result)
				}
			}
		})
	}
}

// TestIsRealFailure tests the isRealFailure function
func TestIsRealFailure(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error should return false",
			err:      nil,
			expected: false,
		},
		{
			name:     "rich media download failure (850026) should return true",
			err:      errs.New(400, `{"message":"富媒体文件下载失败","code":850026,"err_code":40034001,"trace_id":"test"}`, "test-trace"),
			expected: true,
		},
		{
			name:     "error code 40034001 should return true",
			err:      errs.New(400, `{"message":"error","code":40034001,"err_code":0,"trace_id":"test"}`, "test-trace"),
			expected: true,
		},
		{
			name:     "audit failure (304023) should return false",
			err:      errs.New(400, `{"message":"消息审核不通过","code":304023,"err_code":0,"trace_id":"test"}`, "test-trace"),
			expected: false,
		},
		{
			name:     "HTTP 500 error should return true",
			err:      errs.New(500, "Internal Server Error", "test-trace"),
			expected: true,
		},
		{
			name:     "HTTP 200 error should return false",
			err:      errs.New(200, "OK", "test-trace"),
			expected: false,
		},
		{
			name:     "non-JSON error with 400 status should return true",
			err:      errs.New(400, "Bad Request", "test-trace"),
			expected: true,
		},
		{
			name:     "unknown error code with HTTP 400 should return true",
			err:      errs.New(400, `{"message":"未知错误","code":999999,"err_code":0,"trace_id":"test"}`, "test-trace"),
			expected: true,
		},
		{
			name:     "unknown error code with HTTP 200 should return false",
			err:      errs.New(200, `{"message":"未知状态","code":888888,"err_code":0,"trace_id":"test"}`, "test-trace"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRealFailure(tt.err)
			if result != tt.expected {
				t.Errorf("isRealFailure(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
