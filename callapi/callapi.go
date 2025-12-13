package callapi

import (
	"encoding/json"
	"fmt"

	"github.com/hoshinonyaruko/gensokyo/mylog"
	"github.com/tencent-connect/botgo/openapi"
)

// onebot发来的action调用信息
type ActionMessage struct {
	Action    string        `json:"action"`
	Params    ParamsContent `json:"params"`
	Echo      interface{}   `json:"echo,omitempty"`
	RequestID interface{}   `json:"request_id,omitempty"`
}

func (a *ActionMessage) UnmarshalJSON(data []byte) error {
	type Alias ActionMessage

	var rawEcho json.RawMessage
	var rawRequestID json.RawMessage
	temp := &struct {
		*Alias
		Echo      *json.RawMessage `json:"echo,omitempty"`
		RequestID *json.RawMessage `json:"request_id,omitempty"`
	}{
		Alias:     (*Alias)(a),
		Echo:      &rawEcho,
		RequestID: &rawRequestID,
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Prefer parsing request_id if present even when echo is not set.
	if rawRequestID != nil {
		var lastErr error

		var intValue int
		if lastErr = json.Unmarshal(rawRequestID, &intValue); lastErr == nil {
			a.RequestID = intValue
			a.Echo = intValue
			return nil
		}

		var strValue string
		if lastErr = json.Unmarshal(rawRequestID, &strValue); lastErr == nil {
			a.RequestID = strValue
			a.Echo = strValue
			return nil
		}

		var arrValue []interface{}
		if lastErr = json.Unmarshal(rawRequestID, &arrValue); lastErr == nil {
			a.RequestID = arrValue
			a.Echo = arrValue
			return nil
		}

		var objValue map[string]interface{}
		if lastErr = json.Unmarshal(rawRequestID, &objValue); lastErr == nil {
			a.RequestID = objValue
			a.Echo = objValue
			return nil
		}
	}

	if rawEcho != nil {
		var lastErr error

		var intValue int
		if lastErr = json.Unmarshal(rawEcho, &intValue); lastErr == nil {
			a.Echo = intValue
			return nil
		}

		// If request_id exists, prefer it
		if rawRequestID != nil {
			// If request_id also present, prefer it but don't stop: we handled above so simply reuse
			var lastErr error
			var intValue int
			if lastErr = json.Unmarshal(rawRequestID, &intValue); lastErr == nil {
				a.RequestID = intValue
				a.Echo = intValue
				return nil
			}
			var strValue string
			if lastErr = json.Unmarshal(rawRequestID, &strValue); lastErr == nil {
				a.RequestID = strValue
				a.Echo = strValue
				return nil
			}
			var arrValue []interface{}
			if lastErr = json.Unmarshal(rawRequestID, &arrValue); lastErr == nil {
				a.RequestID = arrValue
				a.Echo = arrValue
				return nil
			}
			var objValue map[string]interface{}
			if lastErr = json.Unmarshal(rawRequestID, &objValue); lastErr == nil {
				a.RequestID = objValue
				a.Echo = objValue
				return nil
			}
		}

		var strValue string
		if lastErr = json.Unmarshal(rawEcho, &strValue); lastErr == nil {
			a.Echo = strValue
			return nil
		}

		var arrValue []interface{}
		if lastErr = json.Unmarshal(rawEcho, &arrValue); lastErr == nil {
			a.Echo = arrValue
			return nil
		}

		var objValue map[string]interface{}
		if lastErr = json.Unmarshal(rawEcho, &objValue); lastErr == nil {
			a.Echo = objValue
			return nil
		}

		return fmt.Errorf("unable to unmarshal echo: %v", lastErr)
	}

	return nil
}

// params类型
type ParamsContent struct {
	BotQQ     string      `json:"botqq"`
	ChannelID string      `json:"channel_id"`
	GuildID   string      `json:"guild_id"`
	GroupID   interface{} `json:"group_id"`           // 每一种onebotv11实现的字段类型都可能不同
	Message   interface{} `json:"message"`            // 这里使用interface{}因为它可能是多种类型
	UserID    interface{} `json:"user_id"`            // 这里使用interface{}因为它可能是多种类型
	Duration  int         `json:"duration,omitempty"` // 可选的整数
	Enable    bool        `json:"enable,omitempty"`   // 可选的布尔值
}

// 自定义一个ParamsContent的UnmarshalJSON 让GroupID同时兼容str和int
func (p *ParamsContent) UnmarshalJSON(data []byte) error {
	type Alias ParamsContent
	aux := &struct {
		GroupID interface{} `json:"group_id"`
		UserID  interface{} `json:"user_id"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	switch v := aux.GroupID.(type) {
	case nil: // 当GroupID不存在时
		p.GroupID = ""
	case float64: // JSON的数字默认被解码为float64
		p.GroupID = fmt.Sprintf("%.0f", v) // 将其转换为字符串，忽略小数点后的部分
	case string:
		p.GroupID = v
	default:
		return fmt.Errorf("GroupID has unsupported type")
	}

	switch v := aux.UserID.(type) {
	case nil: // 当UserID不存在时
		p.UserID = ""
	case float64: // JSON的数字默认被解码为float64
		p.UserID = fmt.Sprintf("%.0f", v) // 将其转换为字符串，忽略小数点后的部分
	case string:
		p.UserID = v
	default:
		return fmt.Errorf("UserID has unsupported type")
	}

	return nil
}

// Message represents a standardized structure for the incoming messages.
type Message struct {
	Action    string                 `json:"action"`
	Params    map[string]interface{} `json:"params"`
	Echo      interface{}            `json:"echo,omitempty"`
	RequestID interface{}            `json:"request_id,omitempty"`
}

// GetActionEchoKey returns RequestID if present, otherwise Echo.
func GetActionEchoKey(a ActionMessage) interface{} {
	if a.RequestID != nil {
		return a.RequestID
	}
	return a.Echo
}

// 这是一个接口,在wsclient传入client但不需要引用wsclient包,避免循环引用,复用wsserver和client逻辑
type Client interface {
	SendMessage(message map[string]interface{}) error
}

// 为了解决processor和server循环依赖设计的接口
type WebSocketServerClienter interface {
	SendMessage(message map[string]interface{}) error
	Close() error
}

// 根据action订阅handler处理api
type HandlerFunc func(client Client, api openapi.OpenAPI, apiv2 openapi.OpenAPI, messgae ActionMessage)

var handlers = make(map[string]HandlerFunc)

// RegisterHandler registers a new handler for a specific action.
func RegisterHandler(action string, handler HandlerFunc) {
	handlers[action] = handler
}

// CallAPIFromDict 处理信息 by calling the 对应的 handler.
func CallAPIFromDict(client Client, api openapi.OpenAPI, apiv2 openapi.OpenAPI, message ActionMessage) {
	handler, ok := handlers[message.Action]
	if !ok {
		mylog.Println("Unsupported action:", message.Action)
		return
	}
	handler(client, api, apiv2, message)
}
