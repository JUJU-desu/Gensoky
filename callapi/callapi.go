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
			a.Params.RequestID = a.RequestID
			return nil
		}

		// If request_id not set at top-level, try to find it inside params.
		if a.RequestID == nil {
			var rawMap map[string]json.RawMessage
			if err := json.Unmarshal(data, &rawMap); err == nil {
				if paramsRaw, ok := rawMap["params"]; ok {
					var paramsMap map[string]json.RawMessage
					if err := json.Unmarshal(paramsRaw, &paramsMap); err == nil {
						// Check for common key variants
						var reqRaw json.RawMessage
						if v, ok := paramsMap["request_id"]; ok {
							reqRaw = v
						} else if v, ok := paramsMap["requestID"]; ok {
							reqRaw = v
						}
						if reqRaw != nil {
							fmt.Printf("DEBUG Unmarshal: params.request_id raw: %s\n", string(reqRaw))
							// Try string first
							var s string
							if err := json.Unmarshal(reqRaw, &s); err == nil {
								a.RequestID = s
								a.Echo = s
								a.Params.RequestID = a.RequestID
								return nil
							}
							// Try numeric -> map to string
							var f float64
							if err := json.Unmarshal(reqRaw, &f); err == nil {
								a.RequestID = fmt.Sprintf("%.0f", f)
								a.Echo = a.RequestID
								a.Params.RequestID = a.RequestID
								return nil
							}
							// Try other types for compatibility
							var obj map[string]interface{}
							if err := json.Unmarshal(reqRaw, &obj); err == nil {
								a.RequestID = obj
								a.Echo = obj
								a.Params.RequestID = a.RequestID
								return nil
							}
							var arr []interface{}
							if err := json.Unmarshal(reqRaw, &arr); err == nil {
								a.RequestID = arr
								a.Echo = arr
								a.Params.RequestID = a.RequestID
								return nil
							}
						}
					}
				}
			}
		}

		var strValue string
		if lastErr = json.Unmarshal(rawRequestID, &strValue); lastErr == nil {
			a.RequestID = strValue
			a.Echo = strValue
			a.Params.RequestID = a.RequestID
			return nil
		}

		var arrValue []interface{}
		if lastErr = json.Unmarshal(rawRequestID, &arrValue); lastErr == nil {
			a.RequestID = arrValue
			a.Echo = arrValue
			a.Params.RequestID = a.RequestID
			return nil
		}

		var objValue map[string]interface{}
		if lastErr = json.Unmarshal(rawRequestID, &objValue); lastErr == nil {
			a.RequestID = objValue
			a.Echo = objValue
			a.Params.RequestID = a.RequestID
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
				a.Params.RequestID = a.RequestID
				return nil
			}
			var strValue string
			if lastErr = json.Unmarshal(rawRequestID, &strValue); lastErr == nil {
				a.RequestID = strValue
				a.Echo = strValue
				a.Params.RequestID = a.RequestID
				return nil
			}
			var arrValue []interface{}
			if lastErr = json.Unmarshal(rawRequestID, &arrValue); lastErr == nil {
				a.RequestID = arrValue
				a.Echo = arrValue
				a.Params.RequestID = a.RequestID
				return nil
			}
			var objValue map[string]interface{}
			if lastErr = json.Unmarshal(rawRequestID, &objValue); lastErr == nil {
				a.RequestID = objValue
				a.Echo = objValue
				a.Params.RequestID = a.RequestID
				return nil
			}
		}

		// If request_id not set at top-level, try to find it inside params.
		if a.RequestID == nil {
			var rawMap map[string]json.RawMessage
			if err := json.Unmarshal(data, &rawMap); err == nil {
				if paramsRaw, ok := rawMap["params"]; ok {
					var paramsMap map[string]json.RawMessage
					if err := json.Unmarshal(paramsRaw, &paramsMap); err == nil {
						// Check for common key variants
						var reqRaw json.RawMessage
						if v, ok := paramsMap["request_id"]; ok {
							reqRaw = v
						} else if v, ok := paramsMap["requestID"]; ok {
							reqRaw = v
						}
						if reqRaw != nil {
							// Try string first
							var s string
							if err := json.Unmarshal(reqRaw, &s); err == nil {
								a.RequestID = s
								a.Echo = s
								return nil
							}
							// Try numeric -> map to string
							var f float64
							if err := json.Unmarshal(reqRaw, &f); err == nil {
								a.RequestID = fmt.Sprintf("%.0f", f)
								a.Echo = a.RequestID
								return nil
							}
							// Try other types for compatibility
							var obj map[string]interface{}
							if err := json.Unmarshal(reqRaw, &obj); err == nil {
								a.RequestID = obj
								a.Echo = obj
								return nil
							}
							var arr []interface{}
							if err := json.Unmarshal(reqRaw, &arr); err == nil {
								a.RequestID = arr
								a.Echo = arr
								return nil
							}
						}
					}
				}
			}
		}

		var strValue string
		if lastErr = json.Unmarshal(rawEcho, &strValue); lastErr == nil {
			a.Echo = strValue
			if a.RequestID == nil {
				a.RequestID = strValue
				a.Params.RequestID = a.RequestID
			}
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

		// Fallback: if RequestID still nil, try pulling from params unconditionally
		// This handles cases where params.request_id exists but no top-level echo/request_id provided
		if a.RequestID == nil {
			var rawMap map[string]json.RawMessage
			if err := json.Unmarshal(data, &rawMap); err == nil {
				if paramsRaw, ok := rawMap["params"]; ok {
					var paramsMap map[string]json.RawMessage
					if err := json.Unmarshal(paramsRaw, &paramsMap); err == nil {
						var reqRaw json.RawMessage
						if v, ok := paramsMap["request_id"]; ok {
							reqRaw = v
						} else if v, ok := paramsMap["requestID"]; ok {
							reqRaw = v
						}
						if reqRaw != nil {
							// Try string
							var s string
							if err := json.Unmarshal(reqRaw, &s); err == nil {
								a.RequestID = s
								a.Echo = s
							}
							var f float64
							if err := json.Unmarshal(reqRaw, &f); err == nil {
								a.RequestID = fmt.Sprintf("%.0f", f)
								a.Echo = a.RequestID
							}
							var obj map[string]interface{}
							if err := json.Unmarshal(reqRaw, &obj); err == nil {
								a.RequestID = obj
								a.Echo = obj
							}
							var arr []interface{}
							if err := json.Unmarshal(reqRaw, &arr); err == nil {
								a.RequestID = arr
								a.Echo = arr
							}
						}
					}
				}
			}
		}

		// If RequestID found, ensure it also exists inside Params for compatibility
		if a.RequestID != nil {
			// If params struct exists, set its RequestID field
			a.Params.RequestID = a.RequestID
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
	RequestID interface{} `json:"request_id,omitempty"`
}

// 自定义一个ParamsContent的UnmarshalJSON 让GroupID同时兼容str和int
func (p *ParamsContent) UnmarshalJSON(data []byte) error {
	type Alias ParamsContent
	aux := &struct {
		GroupID interface{} `json:"group_id"`
		UserID  interface{} `json:"user_id"`
		RequestID interface{} `json:"request_id"`
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

	// pass-through request_id value if present in params
	p.RequestID = aux.RequestID

	return nil
}

// Message represents a standardized structure for the incoming messages.
type Message struct {
	Action    string                 `json:"action"`
	Params    map[string]interface{} `json:"params"`
	Echo      interface{}            `json:"echo,omitempty"`
	RequestID interface{}            `json:"request_id,omitempty"`
}

// GetActionEchoKey returns request_id if present, otherwise Echo.
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
