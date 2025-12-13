package requestid

import (
	"fmt"
	"github.com/google/uuid"
	"time"
)

// NewRequestID 返回带时间戳前缀 + UUIDv4 的字符串, 例如: "1681234567890-xxxxxxxx-xxxx-4xxx-xxxx-xxxxxxxxxxxx"
func NewRequestID() string {
	ts := time.Now().UnixMilli()
	u := uuid.New()
	return fmt.Sprintf("%d-%s", ts, u.String())
}
